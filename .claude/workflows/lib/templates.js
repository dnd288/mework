#!/usr/bin/env node
'use strict';
/*
 * templates — discover + manage optional, skill-like task-planning templates.
 *
 * A *template* works like a skill: a folder under `templatesDir` (default
 * `openspec/templates/`) holding a `TEMPLATE.md` with `name` + `description`
 * frontmatter and a body that is the planning guide (the "readme") — HOW to break
 * this kind of work into tasks for THIS project's landscape.
 *
 *   openspec/templates/<name>/TEMPLATE.md
 *   ---
 *   name: <name>
 *   description: <one-line — when this template applies>
 *   ---
 *   # <Title>
 *   ...planning guide...
 *
 * Templates are OPTIONAL. During planning the agent lists them and uses one only if
 * it matches the change; if there is no template (the common case), planning just
 * proceeds normally. This module is the deterministic surface the template.js
 * workflow + /opsx:template-* commands call (the same way ship-code.js calls the
 * gate-resolver CLI):
 *
 *   node .claude/workflows/lib/templates.js list [--json] [--dir <d>]
 *   node .claude/workflows/lib/templates.js show <name> [--json] [--dir <d>]
 *   node .claude/workflows/lib/templates.js path <name> [--dir <d>]
 *   printf '%s' "<body>" | node .../templates.js create <name> --description "<d>" [--force]
 *   node .claude/workflows/lib/templates.js remove <name> [--dir <d>]
 *
 * Exports list/get/create/remove(dir, ...) for unit tests (dir is explicit there).
 */

const fs = require('fs');
const path = require('path');

const TEMPLATE_FILE = 'TEMPLATE.md';
const DEFAULT_DIR = 'openspec/templates';

// Resolve the templates directory: explicit --dir > config.templatesDir > default,
// relative to cwd (opsx commands run from the repo root).
function resolveDir(explicitDir) {
  if (explicitDir) return path.resolve(explicitDir);
  let configured = DEFAULT_DIR;
  try {
    // Walk up for mzspec.config.json without requiring a valid toolchains block —
    // discovery must work even on a barely-configured repo.
    let dir = process.cwd();
    for (;;) {
      const c = path.join(dir, 'mzspec.config.json');
      if (fs.existsSync(c)) {
        const cfg = JSON.parse(fs.readFileSync(c, 'utf8'));
        if (cfg && typeof cfg.templatesDir === 'string') configured = cfg.templatesDir;
        break;
      }
      const parent = path.dirname(dir);
      if (parent === dir) break;
      dir = parent;
    }
  } catch {
    /* fall back to default */
  }
  return path.resolve(configured);
}

// Tiny frontmatter reader — just the leading --- ... --- block, `key: value` lines.
function parseFrontmatter(text) {
  const fm = {};
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/.exec(text);
  if (!m) return { fm, body: text };
  for (const line of m[1].split(/\r?\n/)) {
    const i = line.indexOf(':');
    if (i === -1) continue;
    const k = line.slice(0, i).trim();
    let v = line.slice(i + 1).trim();
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) v = v.slice(1, -1);
    if (k) fm[k] = v;
  }
  return { fm, body: m[2] };
}

function fileFor(dir, name) {
  return path.join(dir, name, TEMPLATE_FILE);
}

function get(dir, name) {
  const file = fileFor(dir, name);
  if (!fs.existsSync(file)) return null;
  const text = fs.readFileSync(file, 'utf8');
  const { fm, body } = parseFrontmatter(text);
  return { name: fm.name || name, description: fm.description || '', file, body };
}

function list(dir) {
  if (!fs.existsSync(dir)) return [];
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const t = get(dir, entry.name);
    if (t) out.push({ name: t.name, description: t.description, file: t.file });
  }
  return out.sort((a, b) => a.name.localeCompare(b.name));
}

function create(dir, name, description, body, force) {
  if (!/^[a-z0-9][a-z0-9-]*$/.test(name)) {
    throw new Error(`template name must be kebab-case ([a-z0-9-]): "${name}"`);
  }
  const file = fileFor(dir, name);
  if (fs.existsSync(file) && !force) {
    throw new Error(`template "${name}" already exists at ${file} (use --force / template-update)`);
  }
  const content =
    `---\nname: ${name}\ndescription: ${description || ''}\n---\n` +
    (body && body.trim() ? body.replace(/\s*$/, '') + '\n' : `# ${name}\n\n<planning guide for this kind of work>\n`);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, content);
  return { name, description: description || '', file };
}

function remove(dir, name) {
  const folder = path.join(dir, name);
  const file = fileFor(dir, name);
  if (!fs.existsSync(file)) throw new Error(`template "${name}" not found at ${file}`);
  fs.rmSync(folder, { recursive: true, force: true });
  return { name, removed: folder };
}

// ---- CLI ----------------------------------------------------------------------

function flag(args, name) {
  const i = args.indexOf(name);
  return i !== -1 && i + 1 < args.length ? args[i + 1] : undefined;
}

function readStdin() {
  try {
    return fs.readFileSync(0, 'utf8');
  } catch {
    return '';
  }
}

function main(argv) {
  const args = argv.slice(2);
  const cmd = args[0];
  const json = args.includes('--json');
  const dir = resolveDir(flag(args, '--dir'));
  const positional = args.slice(1).filter((a) => !a.startsWith('--'));
  const out = (obj, line) => process.stdout.write(json ? JSON.stringify(obj, null, 2) + '\n' : line + '\n');

  switch (cmd) {
    case 'list': {
      const items = list(dir);
      return out(items, items.map((t) => `${t.name}\t${t.description}`).join('\n'));
    }
    case 'show': {
      const t = get(dir, positional[0]);
      if (!t) throw new Error(`template "${positional[0]}" not found`);
      return out(t, `# ${t.name}\n${t.description}\n\n${t.body}`);
    }
    case 'path': {
      const t = get(dir, positional[0]);
      if (!t) throw new Error(`template "${positional[0]}" not found`);
      return process.stdout.write(t.file + '\n');
    }
    case 'create': {
      const body = args.includes('--body-stdin') || !process.stdin.isTTY ? readStdin() : '';
      const t = create(dir, positional[0], flag(args, '--description'), body, args.includes('--force'));
      return out(t, `created ${t.file}`);
    }
    case 'remove': {
      const r = remove(dir, positional[0]);
      return out(r, `removed ${r.removed}`);
    }
    default:
      process.stderr.write('usage: templates.js list|show|path|create|remove [name] [--description d] [--dir d] [--json]\n');
      process.exit(2);
  }
}

if (require.main === module) {
  try {
    main(process.argv);
  } catch (e) {
    process.stderr.write(`templates: ${e.message}\n`);
    process.exit(1);
  }
}

module.exports = { list, get, create, remove, resolveDir, parseFrontmatter, DEFAULT_DIR, TEMPLATE_FILE };
