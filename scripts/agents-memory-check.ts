#!/usr/bin/env bun

/// <reference types="bun" />

/**
 * Validates the shared agent memory store (.agents/memory/): frontmatter,
 * sections, anchor resolution and the user/environment-agnostic guard.
 * Spec: .agents/README.md. Run: bun run memory:check (root package.json).
 * Flags: --no-anchors skips anchor resolution. Exit 1 on errors; warnings
 * never fail the run. There is no index: search and read entries with
 * memory:find and memory:read (agents-find.ts).
 *
 * Managed by shared-agent-memory (.agents/shared-memory.lock.json): change it
 * upstream and run its setup; a local edit blocks the next upgrade of this file.
 */
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

import {
  checkAnchors,
  checkKeywords,
  CONFIG,
  type Config,
  escapeRegExp,
  isConverted,
  listRepoFiles,
  MEMORY_DIR,
  parseUnit,
  REPO_ROOT,
  type RepoFiles,
  UNIT_SECTIONS,
  VERIFIED,
} from './agents-anchors';

const DEFAULT_DIR = join(REPO_ROOT, MEMORY_DIR);
const TYPES = new Set(['project', 'reference']);
const TOP_KEYS = new Set(['name', 'title', 'description', 'keywords', 'verified', 'metadata']);
const META_KEYS = new Set(['type']);
const MAX_HOOK = 160;
const MAX_SUMMARY = 500;

/**
 * User/environment-agnostic guard. Literal <name> placeholders and the homes the
 * config allows (a durable contract, such as a fleet's standard home) pass.
 */
export function personalMarkers(agnostic: Config['agnostic']): [RegExp, string][] {
  const homes = ['<name>', ...agnostic.allowedHomes].map(escapeRegExp).join('|');
  const markers: [RegExp, string][] = [
    [/\/Users\/(?!<name>)[^/\s]+/, 'absolute /Users/ path'],
    [new RegExp(`/home/(?!(?:${homes})(?:/|$))[^/\\s]+`), 'absolute /home/ path'],
    [/[A-Za-z]:\\Users\\(?!<name>)[^\\\s]+/i, 'absolute Windows user-profile path'],
  ];
  if (agnostic.names.length) {
    markers.push([
      new RegExp(`\\b(?:${agnostic.names.map(escapeRegExp).join('|')})\\b`, 'i'),
      'personal name',
    ]);
  }
  return markers;
}

export type MemoryValidationResult = {
  entryCount: number;
  errors: string[];
  /** Entries still in prose, without a unit section. */
  legacy: string[];
  warnings: string[];
};

function checkPersonalMarkers(
  raw: string,
  prefix: string,
  markers: [RegExp, string][],
  errors: string[],
) {
  for (const [pattern, label] of markers) {
    const hit = raw.match(pattern);
    if (hit) {
      errors.push(`${prefix}: ${label} ("${hit[0]}") — entries must be user/environment-agnostic`);
    }
  }
}

export function validateMemoryStore(
  dir = DEFAULT_DIR,
  options: { config?: Config; repo?: RepoFiles } = {},
): MemoryValidationResult {
  const errors: string[] = [];
  const warnings: string[] = [];
  const legacy: string[] = [];
  const config = options.config ?? CONFIG;
  const markers = personalMarkers(config.agnostic);

  if (!existsSync(dir)) {
    return {
      entryCount: 0,
      errors: [`memory dir not found: ${dir}`],
      legacy,
      warnings,
    };
  }

  const entryFiles = readdirSync(dir)
    .filter((file) => file.endsWith('.md') && !/^(HANDOFF|PLAN)-/.test(file))
    .sort();
  const namesByFile = new Map<string, string>();
  const baseNames = new Set(entryFiles.map((file) => file.replace(/\.md$/, '')));

  for (const file of entryFiles) {
    const raw = readFileSync(join(dir, file), 'utf8');
    const err = (message: string) => errors.push(`${file}: ${message}`);
    const unit = parseUnit(raw);
    if (!unit.hasFrontmatter) {
      err('missing frontmatter block (--- ... ---)');
      continue;
    }

    const name = unit.fields.get('name');
    const description = unit.fields.get('description');
    const type = unit.metadata.get('type');
    const verified = unit.fields.get('verified');
    const expected = file.replace(/\.md$/, '').replaceAll('_', '-');

    if (!name) {
      err('missing name');
    } else if (name !== expected) {
      err(`name "${name}" must be "${expected}" (kebab of filename)`);
    }
    if (name) {
      const duplicate = [...namesByFile.entries()].find(([, candidate]) => candidate === name);
      if (duplicate) {
        err(`duplicate name "${name}" (also in ${duplicate[0]})`);
      }
      namesByFile.set(file, name);
    }
    if (!description) {
      err('missing/empty description');
    } else if (description.length > MAX_HOOK) {
      warnings.push(
        `${file}: description is ${description.length} chars (keep ≤${MAX_HOOK}; it is the hook memory:find prints)`,
      );
    }
    if (!type) {
      err('missing metadata.type');
    } else if (!TYPES.has(type)) {
      err(`metadata.type "${type}" must be project|reference`);
    }
    for (const key of unit.fields.keys()) {
      if (!TOP_KEYS.has(key)) {
        err(`unknown frontmatter key "${key}" (allowed: ${[...TOP_KEYS].join(', ')})`);
      }
    }
    for (const key of unit.metadata.keys()) {
      if (!META_KEYS.has(key)) {
        err(`unknown metadata key "${key}" (allowed: type)`);
      }
    }
    for (const line of unit.badFrontmatter) {
      err(`unparseable frontmatter line "${line}"`);
    }
    if (verified !== undefined && !VERIFIED.test(verified)) {
      err('verified must be "<YYYY-MM-DD> <commit>"');
    }
    if (!unit.body.trim()) {
      err('empty body');
    }
    checkPersonalMarkers(raw, file, markers, errors);

    if (!isConverted(unit)) {
      legacy.push(file);
      warnings.push(
        `${file}: legacy prose entry (no ## Anchors or ## Not in code) — convert it with prompts/agentic-format-conversion.md`,
      );
      if (unit.fields.has('keywords')) {
        checkKeywords(file, unit.fields.get('keywords'), errors, warnings);
      }
      continue;
    }

    if (!unit.fields.get('title')) {
      err('missing title');
    }
    checkKeywords(file, unit.fields.get('keywords'), errors, warnings);
    if (unit.summary.length > MAX_SUMMARY) {
      warnings.push(`${file}: summary is ${unit.summary.length} chars (keep ≤${MAX_SUMMARY})`);
    }
    for (const heading of unit.headings) {
      if (!UNIT_SECTIONS.has(heading.text)) {
        err(
          `line ${heading.line}: unexpected section "${heading.text}" (allowed: ${[...UNIT_SECTIONS].join(', ')})`,
        );
      }
    }
    checkAnchors(file, unit, options.repo, errors, config);
  }

  // Dangling wiki links are warnings: they can identify memory worth writing.
  const knownNames = new Set([...namesByFile.values(), ...baseNames]);
  for (const file of entryFiles) {
    const body = readFileSync(join(dir, file), 'utf8');
    for (const match of body.matchAll(/\[\[([\w-]+)\]\]/g)) {
      if (!knownNames.has(match[1])) {
        warnings.push(`${file}: dangling link [[${match[1]}]]`);
      }
    }
  }

  return { entryCount: entryFiles.length, errors, legacy, warnings };
}

if (import.meta.main) {
  const flags = new Set(process.argv.slice(2));
  const result = validateMemoryStore(DEFAULT_DIR, {
    repo: flags.has('--no-anchors') ? undefined : listRepoFiles(),
  });
  for (const warning of result.warnings) {
    console.warn(`WARN  ${warning}`);
  }
  for (const error of result.errors) {
    console.error(`ERROR ${error}`);
  }
  console.log(
    `${result.entryCount} entries checked — ${result.errors.length} error(s), ${result.warnings.length} warning(s), ${result.legacy.length} in legacy prose`,
  );
  process.exitCode = result.errors.length ? 1 : 0;
}
