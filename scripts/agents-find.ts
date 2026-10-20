#!/usr/bin/env bun

/// <reference types="bun" />

/**
 * Finder, reader and drift report for agentic units (.agents/memory entries,
 * specs and docs). There is no index: every command reads the units.
 * Format: .agents/README.md.
 *
 *   bun run memory:find <term>... [--at <path>] [--memory|--specs|--docs] [--type project|reference] [--limit N] [--all]
 *   bun run memory:read <unit>... [--hits N] [--near N]
 *   bun run memory:drift [--memory|--specs|--docs] [--near N] [--all]
 *
 * find ranks units by words (title, keywords and anchor patterns weigh 3, hook
 * and anchor globs 2, body and path 1; `a|b` matches either) or lists the units
 * bound to a file or directory through an anchor hit (--at). read prints a unit
 * with every anchor resolved to path:line, then the commits that touched the
 * anchored files since the unit was verified. drift lists the units whose
 * anchored lines changed since their baseline.
 *
 * Managed by shared-agent-memory (.agents/shared-memory.lock.json): change it
 * upstream and run its setup; a local edit blocks the next upgrade of this file.
 */
import { existsSync, readFileSync, realpathSync } from 'node:fs';
import { basename, dirname, isAbsolute, join } from 'node:path';

import {
  type Anchor,
  CONFIG,
  type Config,
  filesMatchingGlob,
  type Hit,
  inSpecs,
  isUnitPath,
  listRepoFiles,
  loadConfig,
  MEMORY_DIR,
  parseList,
  parseUnit,
  REPO_ROOT,
  type RepoFiles,
  resolveAnchor,
  type Unit,
} from './agents-anchors';

export type Source = 'docs' | 'memory' | 'specs';

export type LoadedUnit = {
  hook: string;
  id?: string;
  keywords: string[];
  /** Filename without `.md`; memory entries link each other by it. */
  name: string;
  path: string;
  raw: string;
  source: Source;
  title: string;
  unit: Unit;
};

export type Query = {
  /** Repo file or directory the units must be bound to through an anchor hit. */
  at?: string;
  sources?: Set<Source>;
  terms?: string[];
  types?: Set<string>;
};

/** One anchor of a unit that hits the queried path; `breadth` is how many repository files its glob matches. */
export type Binding = {
  anchor: Anchor;
  breadth: number;
  hits: Hit[];
};

export type Match = {
  /** Anchor hits inside `at`, when the query has one. */
  bindings: Hit[];
  /** The tightest binding to `at`: smallest glob breadth, then most hits. */
  bound?: Binding;
  /** Fields where a term matched. */
  fields: string[];
  matched: number;
  score: number;
  unit: LoadedUnit;
};

/** A queried path as a repository file or directory prefix. */
export type Target = { isFile: boolean; path: string };

export type Drift = {
  base?: string;
  baseSource?: 'unit' | 'verified';
  commits: string[];
  error?: string;
  /** Files the unit's anchors hit. */
  files: string[];
  /** `path:line` of diff hunks within `window` lines of an anchor hit. */
  near: string[];
  window: number;
};

const DEFAULT_WINDOW = 20;

export function sourceOf(path: string, config: Config = CONFIG): Source {
  if (path.startsWith(`${MEMORY_DIR}/`)) {
    return 'memory';
  }
  return inSpecs(path, config) ? 'specs' : 'docs';
}

export function loadUnits(
  root = REPO_ROOT,
  repo = listRepoFiles(root),
  config = loadConfig(root),
): LoadedUnit[] {
  return (
    repo.paths
      // A tracked unit deleted in the working tree is still listed by git ls-files.
      .filter((path) => isUnitPath(path, config) && existsSync(join(root, path)))
      .sort()
      .map((path) => {
        const raw = readFileSync(join(root, path), 'utf8');
        const unit = parseUnit(raw);
        const name = basename(path, '.md');
        return {
          hook: unit.fields.get('description') ?? '',
          id: unit.fields.get('id'),
          keywords: parseList(unit.fields.get('keywords')) ?? [],
          name,
          path,
          raw,
          source: sourceOf(path, config),
          title: unit.title ?? name,
          unit,
        };
      })
  );
}

/** Searchable fields, best weight first; a term scores the first field it matches. */
function columnsOf(loaded: LoadedUnit): [string, string, number][] {
  const anchors = loaded.unit.anchors;
  return [
    ['title', loaded.title.toLowerCase(), 3],
    ['kw', loaded.keywords.join('\n').toLowerCase(), 3],
    [
      'anchor',
      anchors
        .map((anchor) => anchor.pattern)
        .join('\n')
        .toLowerCase(),
      3,
    ],
    ['hook', loaded.hook.toLowerCase(), 2],
    [
      'glob',
      anchors
        .map((anchor) => anchor.glob)
        .join('\n')
        .toLowerCase(),
      2,
    ],
    ['body', loaded.unit.body.toLowerCase(), 1],
    ['path', loaded.path.toLowerCase(), 1],
  ];
}

/**
 * Whitespace separates terms (a quoted phrase is several terms); `a|b` lists
 * alternatives. Every matching alternative adds its field weight, so a wide OR
 * ranks the units that cover most of it first.
 */
export function splitTerms(words: string[]) {
  return words
    .flatMap((word) => word.split(/\s+/))
    .map((term) => term.toLowerCase().split('|').filter(Boolean))
    .filter((options) => options.length);
}

/**
 * A queried path as the repository knows it: absolute paths under the root are
 * made relative (through symlinks), `./` and trailing slashes are dropped.
 * Undefined when nothing in the repository is at that path.
 */
export function resolveTarget(at: string, repo: RepoFiles, root = REPO_ROOT): Target | undefined {
  let path = at.trim().replaceAll('\\', '/');
  if (isAbsolute(path)) {
    const real = existsSync(path) ? realpathSync(path) : path;
    const rootReal = realpathSync(root);
    if (real === rootReal) {
      path = '';
    } else if (real.startsWith(`${rootReal}/`)) {
      path = real.slice(rootReal.length + 1);
    } else {
      return undefined;
    }
  }
  path = path.replace(/^(\.\/)+/, '').replace(/\/+$/, '');
  if (!path) {
    return undefined;
  }
  const paths = new Set(repo.paths);
  if (paths.has(path)) {
    return { isFile: true, path };
  }
  if (repo.paths.some((candidate) => candidate.startsWith(`${path}/`))) {
    return { isFile: false, path };
  }
  return undefined;
}

/**
 * Every anchor of the unit whose glob matches a file at `target` and whose
 * regex hits it, with the glob's repository-wide breadth for ranking.
 */
export function bindingsOf(
  loaded: LoadedUnit,
  target: Target,
  repo: RepoFiles,
  view: RepoFiles,
): Binding[] {
  const bindings: Binding[] = [];
  for (const anchor of loaded.unit.anchors) {
    const { hits } = resolveAnchor(anchor, view, 10);
    if (!hits.length) {
      continue;
    }
    bindings.push({
      anchor,
      breadth: filesMatchingGlob(repo, anchor.glob).length,
      hits,
    });
  }
  return bindings.sort((a, b) => a.breadth - b.breadth || b.hits.length - a.hits.length);
}

export function findUnits(units: LoadedUnit[], query: Query, repo?: RepoFiles): Match[] {
  const terms = splitTerms(query.terms ?? []);
  let target: Target | undefined;
  let view: RepoFiles | undefined;
  if (query.at) {
    if (!repo) {
      throw new Error('a bound-unit query (at) needs the repository files');
    }
    target = resolveTarget(query.at, repo);
    if (!target) {
      throw new Error(`no repository file or directory at ${query.at}`);
    }
    const prefix = `${target.path}/`;
    const targetPath = target.path;
    view = {
      lines: repo.lines,
      paths: repo.paths.filter((path) => path === targetPath || path.startsWith(prefix)),
    };
  }
  const matches: Match[] = [];
  for (const unit of units) {
    if (query.sources?.size && !query.sources.has(unit.source)) {
      continue;
    }
    if (query.types?.size && !query.types.has(unit.unit.metadata.get('type') ?? '')) {
      continue;
    }
    const bindings: Hit[] = [];
    let bound: Binding | undefined;
    if (target && view && repo) {
      const all = bindingsOf(unit, target, repo, view);
      if (!all.length) {
        continue;
      }
      bound = all[0];
      bindings.push(...all.flatMap((binding) => binding.hits));
    }
    const columns = columnsOf(unit);
    const fields = new Set<string>();
    let matched = 0;
    let score = 0;
    for (const options of terms) {
      let any = false;
      for (const option of options) {
        const best = columns.find(([, text]) => text.includes(option));
        if (!best) {
          continue;
        }
        any = true;
        score += best[2];
        fields.add(best[0]);
      }
      if (any) {
        matched += 1;
      }
    }
    if (terms.length && !matched) {
      continue;
    }
    matches.push({
      bindings,
      bound,
      fields: [...fields],
      matched,
      score,
      unit,
    });
  }
  // Words first; among equals the unit whose anchor targets the path most tightly
  // (fewest files behind its glob, then most hits) comes first.
  return matches.sort(
    (a, b) =>
      b.score - a.score ||
      b.matched - a.matched ||
      (a.bound && b.bound
        ? a.bound.breadth - b.bound.breadth || b.bound.hits.length - a.bound.hits.length
        : 0) ||
      (a.unit.path < b.unit.path ? -1 : 1),
  );
}

/** The unit's text with each anchor row followed by its hits as `path:line: text`. */
export function renderUnit(loaded: LoadedUnit, repo: RepoFiles, options: { hits?: number } = {}) {
  const limit = options.hits ?? 5;
  const byLine = new Map(loaded.unit.anchors.map((anchor) => [anchor.line, anchor]));
  const out: string[] = [];
  loaded.raw.split('\n').forEach((line, index) => {
    out.push(line);
    const anchor = byLine.get(index + 1);
    if (!anchor) {
      return;
    }
    const { error, hits } = resolveAnchor(anchor, repo, 500);
    if (error) {
      out.push(`    DEAD ${error}`);
    }
    for (const hit of hits.slice(0, limit)) {
      out.push(`    ${hit.path}:${hit.line}: ${hit.text.slice(0, 120)}`);
    }
    if (hits.length > limit) {
      out.push(`    … ${hits.length - limit} more hits`);
    }
  });
  return out.join('\n');
}

function git(root: string, args: string[]) {
  const result = Bun.spawnSync(['git', ...args], { cwd: root });
  return {
    code: result.exitCode,
    err: result.stderr.toString().trim(),
    out: result.stdout.toString(),
  };
}

/**
 * Commits that touched the unit's anchored files since its baseline: the
 * `verified` commit when the unit has one, else the unit's last commit. `near`
 * keeps the diff hunks within `window` lines of an anchor hit, which is where
 * a bound fact is most likely to have gone stale.
 */
export function driftOf(
  loaded: LoadedUnit,
  repo: RepoFiles,
  options: { root?: string; window?: number } = {},
): Drift {
  const root = options.root ?? REPO_ROOT;
  const window = options.window ?? DEFAULT_WINDOW;
  const drift: Drift = { commits: [], files: [], near: [], window };
  const verified = loaded.unit.fields.get('verified')?.split(/\s+/)[1];
  if (verified) {
    drift.base = verified;
    drift.baseSource = 'verified';
  } else {
    const last = git(root, ['log', '-1', '--format=%h', '--', loaded.path]).out.trim();
    if (last) {
      drift.base = last;
      drift.baseSource = 'unit';
    }
  }
  const regions: { from: number; path: string; to: number }[] = [];
  for (const anchor of loaded.unit.anchors) {
    for (const hit of resolveAnchor(anchor, repo, 50).hits) {
      regions.push({
        from: hit.line - window,
        path: hit.path,
        to: hit.line + window,
      });
    }
  }
  drift.files = [...new Set(regions.map((region) => region.path))];
  if (!drift.base || !drift.files.length) {
    return drift;
  }

  const range = `${drift.base}..HEAD`;
  const log = git(root, [
    'log',
    '--no-merges',
    '--format=%h %ad %s',
    '--date=short',
    range,
    '--',
    ...drift.files,
  ]);
  if (log.code !== 0) {
    drift.error = log.err || `git log ${range} failed`;
    return drift;
  }
  drift.commits = log.out.split('\n').filter(Boolean);
  if (!drift.commits.length) {
    return drift;
  }

  const diff = git(root, ['diff', '-U0', range, '--', ...drift.files]);
  const near = new Set<string>();
  let file = '';
  for (const line of diff.out.split('\n')) {
    const header = line.match(/^\+\+\+ (?:b\/(.*)|\/dev\/null)$/);
    if (header) {
      file = header[1] ?? '';
      continue;
    }
    const hunk = line.match(/^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@/);
    if (!hunk || !file) {
      continue;
    }
    const from = Number(hunk[1]);
    const to = from + Math.max(1, Number(hunk[2] ?? 1)) - 1;
    if (regions.some((region) => region.path === file && from <= region.to && to >= region.from)) {
      near.add(`${file}:${from}`);
    }
  }
  drift.near = [...near];
  return drift;
}

export function renderDrift(drift: Drift) {
  if (!drift.files.length) {
    return 'Drift: no anchor hits to watch';
  }
  if (!drift.base) {
    return 'Drift: no baseline (unit not committed yet)';
  }
  const since = `since ${drift.base} (${drift.baseSource === 'verified' ? 'verified' : 'last commit of this unit'})`;
  if (drift.error) {
    return `Drift ${since}: ${drift.error}`;
  }
  if (!drift.commits.length) {
    return `Drift ${since}: none`;
  }
  const lines = [
    `Drift ${since}: ${drift.commits.length} commit(s) touched anchored files, ${drift.near.length} hunk(s) within ${drift.window} lines of an anchor hit`,
  ];
  if (drift.near.length) {
    lines.push(
      `  near: ${drift.near.slice(0, 6).join(', ')}${drift.near.length > 6 ? ` (+${drift.near.length - 6})` : ''}`,
    );
  }
  for (const commit of drift.commits.slice(0, 8)) {
    lines.push(`  - ${commit}`);
  }
  if (drift.commits.length > 8) {
    lines.push(`  … ${drift.commits.length - 8} more commits`);
  }
  return lines.join('\n');
}

function usage(message?: string): never {
  if (message) {
    console.error(message);
  }
  console.error(
    [
      'usage: bun run memory:find <term>... [--at <path>] [--memory|--specs|--docs] [--type project|reference] [--limit N] [--all]',
      '       bun run memory:read <unit>... [--hits N] [--near N]',
      '       bun run memory:drift [--memory|--specs|--docs] [--near N] [--all]',
    ].join('\n'),
  );
  process.exit(2);
}

if (import.meta.main) {
  const [command, ...rest] = process.argv.slice(2);
  const flags = {
    all: false,
    at: undefined as string | undefined,
    hits: 5,
    limit: 10,
    sources: new Set<Source>(),
    types: new Set<string>(),
    window: DEFAULT_WINDOW,
  };
  const words: string[] = [];
  for (let index = 0; index < rest.length; index += 1) {
    const arg = rest[index];
    if (arg === '--all') {
      flags.all = true;
    } else if (arg === '--at') {
      index += 1;
      flags.at = rest[index] ?? usage('--at needs a path');
    } else if (arg === '--hits') {
      index += 1;
      flags.hits = Number(rest[index]) || flags.hits;
    } else if (arg === '--limit') {
      index += 1;
      flags.limit = Number(rest[index]) || flags.limit;
    } else if (arg === '--near') {
      index += 1;
      flags.window = Number(rest[index]) || flags.window;
    } else if (arg === '--type') {
      index += 1;
      flags.types.add(rest[index] ?? usage('--type needs a value'));
    } else if (/^--(memory|specs|docs)$/.test(arg)) {
      flags.sources.add(arg.slice(2) as Source);
    } else if (arg.startsWith('--')) {
      usage(`unknown flag ${arg}`);
    } else {
      words.push(arg);
    }
  }
  const repo = listRepoFiles();
  const units = loadUnits(REPO_ROOT, repo, CONFIG);

  if (command === 'find') {
    if (!words.length && !flags.at && !flags.all) {
      usage('find needs terms, --at <path> or --all');
    }
    const target = flags.at ? resolveTarget(flags.at, repo) : undefined;
    if (flags.at && !target) {
      console.error(
        `no repository file or directory at ${flags.at} (paths are relative to the repo root; absolute paths under it are accepted)`,
      );
      process.exit(2);
    }
    const matches = findUnits(
      units,
      {
        at: target?.path,
        sources: flags.sources,
        terms: words,
        types: flags.types,
      },
      repo,
    );
    const shown = flags.all ? matches : matches.slice(0, flags.limit);
    for (const match of shown) {
      const parts = [match.unit.path];
      if (words.length) {
        parts.push(`[${match.matched}/${splitTerms(words).length} ${match.fields.join(',')}]`);
      }
      if (match.bound) {
        const [first] = match.bound.hits;
        const more = match.bindings.length > 1 ? ` +${match.bindings.length - 1}` : '';
        parts.push(`@${first.path}:${first.line}${more} via ${match.bound.anchor.glob}`);
      }
      parts.push(`${match.unit.title}${match.unit.hook ? ` — ${match.unit.hook}` : ''}`);
      console.log(parts.join('  '));
    }
    if (matches.length > shown.length) {
      console.log(`… ${matches.length - shown.length} more (--limit N or --all)`);
    }
    if (!matches.length && target?.isFile && !words.length) {
      // A file with no binding: say what the directory holds instead of a bare "no match".
      const parent = dirname(target.path);
      const near = parent === '.' ? [] : findUnits(units, { at: parent }, repo);
      console.log(
        near.length
          ? `no unit is bound to ${target.path}; ${near.length} unit(s) are bound under ${parent} (run --at ${parent})`
          : `no unit is bound to ${target.path} or under ${parent}`,
      );
    } else if (!matches.length) {
      console.log('no match — try synonyms, an identifier, or --at <path>');
    }
  } else if (command === 'read') {
    if (!words.length) {
      usage('read needs a unit path, name or spec ID');
    }
    let failed = 0;
    for (const word of words) {
      const wanted = word.replace(/^\.\//, '');
      const found = units.filter(
        (unit) =>
          unit.path === wanted ||
          unit.name === wanted ||
          basename(unit.path) === wanted ||
          unit.id === wanted,
      );
      if (!found.length) {
        failed += 1;
        console.error(`unknown unit: ${word}`);
        continue;
      }
      for (const loaded of found) {
        console.log(`# ${loaded.path}`);
        console.log(renderUnit(loaded, repo, { hits: flags.hits }));
        console.log(renderDrift(driftOf(loaded, repo, { window: flags.window })));
        console.log('');
      }
    }
    process.exitCode = failed ? 2 : 0;
  } else if (command === 'drift') {
    const selected = units.filter(
      (unit) => (!flags.sources.size || flags.sources.has(unit.source)) && unit.unit.anchors.length,
    );
    const rows: { drift: Drift; unit: LoadedUnit }[] = [];
    let fileOnly = 0;
    let clean = 0;
    let unknown = 0;
    for (const unit of selected) {
      const drift = driftOf(unit, repo, { window: flags.window });
      if (drift.error) {
        unknown += 1;
        console.error(`${unit.path}: ${drift.error}`);
      } else if (!drift.base || !drift.files.length) {
        unknown += 1;
      } else if (drift.near.length) {
        rows.push({ drift, unit });
      } else if (drift.commits.length) {
        fileOnly += 1;
        if (flags.all) {
          rows.push({ drift, unit });
        }
      } else {
        clean += 1;
      }
    }
    rows.sort(
      (a, b) => b.drift.near.length - a.drift.near.length || (a.unit.path < b.unit.path ? -1 : 1),
    );
    for (const { drift, unit } of rows) {
      console.log(
        `${unit.path}  since ${drift.base}  ${drift.commits.length} commit(s), ${drift.near.length} hunk(s) near anchors${drift.near.length ? `: ${drift.near.slice(0, 3).join(', ')}` : ''} | ${drift.commits[0]}`,
      );
    }
    console.log(
      `${selected.length} units with anchors — ${rows.length} listed, ${fileOnly} with commits on anchored files only, ${clean} clean, ${unknown} without a baseline`,
    );
  } else {
    usage();
  }
}
