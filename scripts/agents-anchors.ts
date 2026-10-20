#!/usr/bin/env bun

/// <reference types="bun" />

/**
 * Shared parser for agentic units (.agents/memory entries, specs and docs):
 * line-based frontmatter, `## Anchors` rows, anchor resolution against the
 * repository, and the per-repo configuration (.agents/shared-memory.json).
 * Format: .agents/README.md.
 * Run: bun run anchors <file.md>... — prints every anchor hit as path:line.
 *
 * Managed by shared-agent-memory (.agents/shared-memory.lock.json): change it
 * upstream and run its setup; a local edit blocks the next upgrade of this file.
 */
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

export const REPO_ROOT = join(import.meta.dir, '..');
export const CONFIG_PATH = '.agents/shared-memory.json';
export const MEMORY_DIR = '.agents/memory';
export const ANCHORS = '## Anchors';
export const NOT_IN_CODE = '## Not in code';
export const UNIT_SECTIONS = new Set([ANCHORS, NOT_IN_CODE]);
/** `verified: <YYYY-MM-DD> <commit>`: when a unit's claims were last checked against the code. */
export const VERIFIED = /^\d{4}-\d{2}-\d{2} [0-9a-f]{7,40}$/;

/** Per-repo settings, read from .agents/shared-memory.json; every key is optional. */
export type Config = {
  agnostic: {
    /** `/home/<x>` homes that are a durable contract, not a personal machine path. */
    allowedHomes: string[];
    /** Personal names the agnostic guard rejects, case-insensitive whole words. */
    names: string[];
  };
  /** Docs layer root (repo-relative); undefined when the repo has none. */
  docs?: string;
  /** Directory names never listed as repository files, at any depth. */
  excludedDirs: string[];
  /**
   * Docs indexed but never converted, path → provenance: generator-owned output
   * outside the generated directories, and evidence whose provenance pins its bytes.
   */
  frozenDocs: Record<string, string>;
  /** Generator-owned directories inside the docs layer, never converted. */
  generatedDocs: string[];
  /** Feature specs directory (repo-relative); undefined when the repo has none. */
  specs?: string;
};

export const DEFAULT_CONFIG: Config = {
  agnostic: { allowedHomes: [], names: [] },
  excludedDirs: ['node_modules', 'dist', 'build'],
  frozenDocs: {},
  generatedDocs: [],
};

/** The repo's configuration over the defaults; a missing file means the defaults. */
export function loadConfig(root = REPO_ROOT): Config {
  let raw: Partial<Config> = {};
  try {
    raw = JSON.parse(readFileSync(join(root, CONFIG_PATH), 'utf8'));
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'ENOENT') {
      throw new Error(`${CONFIG_PATH}: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  return {
    ...DEFAULT_CONFIG,
    ...raw,
    agnostic: { ...DEFAULT_CONFIG.agnostic, ...raw.agnostic },
  };
}

export const CONFIG = loadConfig();

const ANCHOR_ROW = /^- `([^`]+)` in `([^`]+)`(?: — (.+))?$/;
// JS-only syntax that ripgrep's default regex engine rejects.
const RG_INCOMPATIBLE = /\(\?<?[=!]|\\[1-9]|\\k</;

export function escapeRegExp(text: string) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function excludedPath(config: Config) {
  if (!config.excludedDirs.length) {
    return undefined;
  }
  return new RegExp(`(^|/)(${config.excludedDirs.map(escapeRegExp).join('|')})/`);
}

export type Anchor = {
  glob: string;
  line: number;
  note?: string;
  pattern: string;
};

export type Unit = {
  anchors: Anchor[];
  /** Frontmatter lines that are neither `key: value` nor nested metadata. */
  badFrontmatter: string[];
  body: string;
  fields: Map<string, string>;
  /** Absolute line numbers of `##` headings outside code fences. */
  headings: { line: number; text: string }[];
  hasFrontmatter: boolean;
  malformedAnchors: { line: number; text: string }[];
  metadata: Map<string, string>;
  summary: string;
  title?: string;
};

export type Hit = { line: number; path: string; text: string };

export type RepoFiles = {
  lines(path: string): string[] | undefined;
  paths: string[];
};

function unquote(value: string) {
  return value.trim().replace(/^(?:"(.*)"|'(.*)')$/, '$1$2');
}

/** `[a, b, c]` → ['a', 'b', 'c']; undefined when the value is not a flow list. */
export function parseList(value: string | undefined) {
  const match = value?.match(/^\[(.*)\]$/);
  if (!match) {
    return undefined;
  }
  return match[1]
    .split(',')
    .map((item) => unquote(item))
    .filter(Boolean);
}

export function parseUnit(raw: string): Unit {
  const lines = raw.split('\n');
  const fields = new Map<string, string>();
  const metadata = new Map<string, string>();
  const badFrontmatter: string[] = [];
  let bodyStart = 0;
  let hasFrontmatter = false;

  if (lines[0] === '---') {
    const end = lines.indexOf('---', 1);
    if (end > 0) {
      hasFrontmatter = true;
      bodyStart = end + 1;
      let inMetadata = false;
      for (const line of lines.slice(1, end)) {
        if (!line.trim()) {
          continue;
        }
        const nested = line.match(/^ {2}([A-Za-z][\w-]*): *(.*)$/);
        if (nested && inMetadata) {
          metadata.set(nested[1], unquote(nested[2]));
          continue;
        }
        const top = line.match(/^([A-Za-z][\w-]*): *(.*)$/);
        if (top) {
          inMetadata = top[1] === 'metadata' && !top[2].trim();
          fields.set(top[1], unquote(top[2]));
          continue;
        }
        badFrontmatter.push(line);
      }
    }
  }

  const anchors: Anchor[] = [];
  const headings: Unit['headings'] = [];
  const malformedAnchors: Unit['malformedAnchors'] = [];
  const summary: string[] = [];
  let section: string | undefined;
  let fence: string | undefined;
  let title: string | undefined;

  for (let index = bodyStart; index < lines.length; index += 1) {
    const line = lines[index];
    const lineNumber = index + 1;
    const fenceMatch = line.match(/^ {0,3}(`{3,}|~{3,})/);
    if (fenceMatch) {
      if (!fence) {
        fence = fenceMatch[1];
      } else if (fenceMatch[1].startsWith(fence)) {
        fence = undefined;
      }
    }
    if (!fence && !fenceMatch) {
      if (!title && /^# \S/.test(line)) {
        title = line.slice(2).trim();
        continue;
      }
      if (/^## /.test(line)) {
        section = line.trim();
        headings.push({ line: lineNumber, text: section });
        continue;
      }
    }
    if (section === undefined) {
      summary.push(line);
      continue;
    }
    if (section !== ANCHORS || !line.trim()) {
      continue;
    }
    const match = line.match(ANCHOR_ROW);
    if (match) {
      anchors.push({
        glob: match[2],
        line: lineNumber,
        note: match[3],
        pattern: match[1],
      });
    } else {
      malformedAnchors.push({ line: lineNumber, text: line });
    }
  }

  return {
    anchors,
    badFrontmatter,
    body: lines.slice(bodyStart).join('\n'),
    fields,
    hasFrontmatter,
    headings,
    malformedAnchors,
    metadata,
    summary: summary.join('\n').trim(),
    title: fields.get('title') || title,
  };
}

/** A unit uses the agentic format once it has one of the two unit sections. */
export function isConverted(unit: Unit) {
  return unit.headings.some((heading) => UNIT_SECTIONS.has(heading.text));
}

/** Tracked plus untracked-but-not-ignored files, without the excluded directories. */
export function listRepoFiles(root = REPO_ROOT, config = loadConfig(root)): RepoFiles {
  const output = Bun.spawnSync(
    ['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'],
    { cwd: root },
  );
  const excluded = excludedPath(config);
  const paths = [
    ...new Set(
      output.stdout
        .toString()
        .split('\0')
        .filter((path) => path && !excluded?.test(path)),
    ),
  ];
  const cache = new Map<string, string[] | undefined>();
  return {
    lines(path) {
      if (!cache.has(path)) {
        try {
          cache.set(path, readFileSync(join(root, path), 'utf8').split('\n'));
        } catch {
          cache.set(path, undefined);
        }
      }
      return cache.get(path);
    },
    paths,
  };
}

/** In-memory repository for tests: path → file content. */
export function repoFromFiles(files: Record<string, string>): RepoFiles {
  return {
    lines: (path) => files[path]?.split('\n'),
    paths: Object.keys(files),
  };
}

const globCache = new WeakMap<RepoFiles, Map<string, string[]>>();

function matchGlob(repo: RepoFiles, pattern: string) {
  let byGlob = globCache.get(repo);
  if (!byGlob) {
    byGlob = new Map();
    globCache.set(repo, byGlob);
  }
  const cached = byGlob.get(pattern);
  if (cached) {
    return cached;
  }
  const firstMeta = pattern.search(/[*?[{]/);
  const literal = firstMeta === -1 ? pattern : pattern.slice(0, firstMeta);
  const prefix = literal.slice(0, literal.lastIndexOf('/') + 1);
  const glob = new Bun.Glob(pattern);
  const matches = repo.paths.filter((path) => path.startsWith(prefix) && glob.match(path));
  byGlob.set(pattern, matches);
  return matches;
}

/** Repository files a glob matches; cached per repository object. */
export const filesMatchingGlob = matchGlob;

export function resolveAnchor(
  anchor: Anchor,
  repo: RepoFiles,
  limit = Number.POSITIVE_INFINITY,
): { error?: string; hits: Hit[] } {
  if (RG_INCOMPATIBLE.test(anchor.pattern)) {
    return {
      error: `\`${anchor.pattern}\` uses lookaround or backreferences (not ripgrep-compatible)`,
      hits: [],
    };
  }
  let regex: RegExp;
  try {
    regex = new RegExp(anchor.pattern);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { error: `invalid regex \`${anchor.pattern}\` (${message})`, hits: [] };
  }
  const files = matchGlob(repo, anchor.glob);
  if (!files.length) {
    return { error: `no repository file matches \`${anchor.glob}\``, hits: [] };
  }

  const hits: Hit[] = [];
  for (const path of files) {
    const lines = repo.lines(path);
    if (!lines) {
      continue;
    }
    for (let index = 0; index < lines.length; index += 1) {
      if (!regex.test(lines[index])) {
        continue;
      }
      hits.push({ line: index + 1, path, text: lines[index].trim() });
      if (hits.length >= limit) {
        return { hits };
      }
    }
  }
  if (!hits.length) {
    return {
      error: `\`${anchor.pattern}\` not found in \`${anchor.glob}\``,
      hits,
    };
  }
  return { hits };
}

function under(path: string, dir: string | undefined) {
  return dir !== undefined && path.startsWith(`${dir}/`);
}

/** Whether a repo path lies in the specs directory (any depth). */
export function inSpecs(path: string, config = CONFIG) {
  return under(path, config.specs);
}

/**
 * Repo path of a unit (memory entry, spec or converted doc). Units are rewritten
 * freely, so other units link them instead of anchoring their text.
 */
export function isUnitPath(path: string, config = CONFIG) {
  if (path.startsWith(`${MEMORY_DIR}/`)) {
    return path.endsWith('.md') && !/\/(HANDOFF|PLAN)-[^/]*$/.test(path);
  }
  if (!path.endsWith('.md') || !(under(path, config.docs) || under(path, config.specs))) {
    return false;
  }
  if (path === `${config.docs}/README.md` || path === `${config.specs}/README.md`) {
    return false;
  }
  if (config.generatedDocs.some((dir) => under(path, dir))) {
    return false;
  }
  return !Object.hasOwn(config.frozenDocs, path);
}

/** Structural anchor errors plus resolution failures, prefixed `file:line`. */
export function checkAnchors(
  label: string,
  unit: Unit,
  repo: RepoFiles | undefined,
  errors: string[],
  config = CONFIG,
) {
  for (const row of unit.malformedAnchors) {
    errors.push(
      `${label}:${row.line}: malformed anchor row (want: - \`regex\` in \`glob\` — note)`,
    );
  }
  if (!repo) {
    return;
  }
  for (const anchor of unit.anchors) {
    const unitFile = matchGlob(repo, anchor.glob).find((path) => isUnitPath(path, config));
    if (unitFile) {
      errors.push(`${label}:${anchor.line}: anchor into unit \`${unitFile}\` — link it instead`);
      continue;
    }
    const { error } = resolveAnchor(anchor, repo, 1);
    if (error) {
      errors.push(`${label}:${anchor.line}: dead anchor — ${error}`);
    }
  }
}

/** Keyword list shape shared by every unit type. */
export function checkKeywords(
  label: string,
  value: string | undefined,
  errors: string[],
  warnings: string[],
) {
  const keywords = parseList(value);
  if (!value) {
    errors.push(`${label}: missing keywords`);
  } else if (!keywords) {
    errors.push(`${label}: keywords must be a [a, b, c] list`);
  } else if (keywords.length < 2 || keywords.length > 8) {
    warnings.push(`${label}: ${keywords.length} keywords (keep 2–8)`);
  }
  return keywords ?? [];
}

if (import.meta.main) {
  const files = process.argv.slice(2);
  if (!files.length) {
    console.error('usage: bun run anchors <unit.md>...');
    process.exit(2);
  }
  const repo = listRepoFiles();
  let failed = 0;
  for (const file of files) {
    const unit = parseUnit(readFileSync(file, 'utf8'));
    console.log(`# ${file}`);
    for (const row of unit.malformedAnchors) {
      failed += 1;
      console.log(`  MALFORMED ${row.line}: ${row.text}`);
    }
    for (const anchor of unit.anchors) {
      const { error, hits } = resolveAnchor(anchor, repo, 20);
      console.log(`- \`${anchor.pattern}\` in \`${anchor.glob}\``);
      if (error) {
        failed += 1;
        console.log(`  DEAD ${error}`);
      }
      for (const hit of hits) {
        console.log(`  ${hit.path}:${hit.line}: ${hit.text.slice(0, 160)}`);
      }
    }
  }
  process.exitCode = failed ? 1 : 0;
}
