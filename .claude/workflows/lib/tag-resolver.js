#!/usr/bin/env node
'use strict';
/*
 * tag-resolver — deterministic tag-driven skill and hook resolution.
 *
 * Replaces static conditional text in prompt hooks with pure tag matching:
 * tasks/units carry tags (ui, backend, db, api, config, ...), skills and hooks
 * declare which tags they apply to in YAML frontmatter, and this module resolves
 * the intersection at runtime.
 *
 * Tag matching rules:
 *   - A skill/hook with NO tags field → UNIVERSAL (always returned)
 *   - A skill/hook WITH tags → ALL its tags must be present on the unit (AND logic)
 *   - Multiple skills/hooks matching → ALL are returned (OR across results)
 *   - classifyTags() infers tags from file paths via path-prefix mappings
 */

const fs = require('fs');
const path = require('path');

// Core tag taxonomy. Projects can extend this via mzspec.config.json → tags.
const CORE_TAGS = [
  'ui',        // User interface components, screens, forms
  'frontend',  // Frontend code (JS/TS/CSS)
  'backend',   // Backend/server-side logic
  'api',       // API endpoints and contracts
  'db',        // Database schemas, migrations, queries
  'data',      // Data processing, transforms, ETL
  'auth',      // Authentication and authorization
  'security',  // Security concerns, hardening
  'config',    // Configuration, environment setup
  'docs',      // Documentation, ADRs
  'infra',     // Infrastructure, CI/CD, git
  'test',      // Testing infrastructure, fixtures
  'migration', // Data/schema migrations
  'cli',       // CLI tools and scripts
  'design',    // UX/visual design artifacts
];

/**
 * Parse YAML frontmatter from a markdown file.
 * Returns { tags, ...otherFields } or null if no valid frontmatter.
 */
function parseFrontmatter(filePath) {
  try {
    const content = fs.readFileSync(filePath, 'utf8');
    if (!content.startsWith('---')) return null;

    const endIdx = content.indexOf('---', 3);
    if (endIdx === -1) return null;

    const raw = content.slice(3, endIdx).trim();
    const lines = raw.split('\n');
    const result = {};

    for (const line of lines) {
      const colonIdx = line.indexOf(':');
      if (colonIdx === -1) continue;
      const key = line.slice(0, colonIdx).trim();
      const val = line.slice(colonIdx + 1).trim();

      // Parse array values like [ui, frontend]
      if (val.startsWith('[') && val.endsWith(']')) {
        result[key] = val.slice(1, -1).split(',').map(s => s.trim()).filter(Boolean);
      } else {
        result[key] = val;
      }
    }

    return result;
  } catch {
    return null;
  }
}

/**
 * Check if a set of unitTags satisfies a skill/hook's required tags.
 * - No required tags → universal (always matches)
 * - Required tags present → ALL must be in unitTags (AND)
 */
function tagsMatch(unitTags, requiredTags) {
  if (!requiredTags || requiredTags.length === 0) return true; // universal
  if (!unitTags || unitTags.length === 0) return false; // unit has no tags, hook/skill requires some
  return requiredTags.every(t => unitTags.includes(t));
}

/**
 * Recursively find all SKILL.md files under given directories.
 */
function findSkillFiles(dirs) {
  const files = [];
  for (const dir of dirs) {
    if (!fs.existsSync(dir)) continue;
    try {
      const entries = fs.readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        const fullPath = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          const skillPath = path.join(fullPath, 'SKILL.md');
          if (fs.existsSync(skillPath)) files.push(skillPath);
          // Recurse one level deeper for nested structures
          files.push(...findSkillFiles([fullPath]));
        }
      }
    } catch { /* skip unreadable */ }
  }
  return files;
}

/**
 * Find all .prompt.md hook files under given directories matching event name.
 */
function findHookFiles(event, dirs) {
  const files = [];
  const target = `on-${event}.prompt.md`;
  for (const dir of dirs) {
    if (!fs.existsSync(dir)) continue;
    try {
      const entries = fs.readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        if (entry.name === target) {
          files.push(path.join(dir, entry.name));
        }
        if (entry.isDirectory()) {
          // Also search subdirectories (e.g., extensions/*/Hooks/)
          files.push(...findHookFiles(event, [path.join(dir, entry.name)]));
        }
      }
    } catch { /* skip unreadable */ }
  }
  return files;
}

/**
 * Resolve skills matching the given tags.
 *
 * @param {string[]} tags - tags on the unit/task
 * @param {string[]} skillDirs - directories to search for SKILL.md files
 * @returns {{ name: string, path: string, tags: string[] }[]}
 */
function resolveSkills(tags, skillDirs) {
  const skillFiles = findSkillFiles(skillDirs);
  const results = [];

  for (const file of skillFiles) {
    const fm = parseFrontmatter(file);
    if (!fm) continue; // no frontmatter → can't determine tags, skip

    const skillTags = fm.tags || [];
    if (tagsMatch(tags, skillTags)) {
      results.push({
        name: fm.name || path.basename(path.dirname(file)),
        path: file,
        tags: skillTags,
      });
    }
  }

  return results;
}

/**
 * Resolve hooks matching the given tags for a specific pipeline event.
 *
 * @param {string} event - pipeline event name (e.g., "implement", "test", "plan")
 * @param {string[]} tags - tags on the unit/task
 * @param {string[]} hookDirs - directories to search for .prompt.md files
 * @returns {{ file: string, tags: string[], content: string }[]}
 */
function resolveHooks(event, tags, hookDirs) {
  const hookFiles = findHookFiles(event, hookDirs);
  const results = [];

  for (const file of hookFiles) {
    const fm = parseFrontmatter(file);
    const hookTags = (fm && fm.tags) || [];

    if (tagsMatch(tags, hookTags)) {
      // Read full content (skip frontmatter for the content)
      let content = fs.readFileSync(file, 'utf8');
      if (content.startsWith('---')) {
        const endIdx = content.indexOf('---', 3);
        if (endIdx !== -1) {
          content = content.slice(endIdx + 3).trim();
        }
      }
      results.push({ file, tags: hookTags, content });
    }
  }

  return results;
}

/**
 * Infer tags from file paths using path-prefix mappings.
 *
 * @param {string[]} filePaths - list of changed file paths
 * @param {object} tagMappings - { tagName: { paths: [prefix, ...] } }
 * @returns {string[]} inferred tags
 */
function classifyTags(filePaths, tagMappings) {
  if (!tagMappings || !filePaths || !filePaths.length) return [];
  const tags = new Set();

  for (const fp of filePaths) {
    for (const [tag, mapping] of Object.entries(tagMappings)) {
      if (!mapping.paths) continue;
      for (const prefix of mapping.paths) {
        if (fp.startsWith(prefix)) {
          tags.add(tag);
        }
      }
    }
  }

  return Array.from(tags);
}

module.exports = {
  resolveSkills,
  resolveHooks,
  classifyTags,
  parseFrontmatter,
  tagsMatch,
  CORE_TAGS,
};
