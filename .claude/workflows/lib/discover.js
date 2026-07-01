#!/usr/bin/env node
'use strict';
/*
 * discover — synthesize a project's toolchain/gate inventory from its own
 * language manifests, so no `mzspec.config.json` is needed.
 *
 * This is the zero-config default behind gate-resolver.js. It returns an object
 * with the SAME shape the resolver already consumes from a config file:
 *   { toolchains:{<tc>:{dirs:[],gates:[{name,cmd}]}}, bench, metaPrefixes,
 *     always, migration, customGates, taskSources }
 *
 * Detection is convention-based and self-healing — a package is gated the moment
 * it carries the language's manifest, with no list to maintain:
 *   - py  : root pyproject.toml `[tool.uv.workspace].members` (else: dirs with a
 *           pyproject.toml). Gates: uv ruff/pyright/pytest.
 *   - go  : every dir with a go.mod. Gates: go build/vet/test -race.
 *   - ts  : pnpm-workspace.yaml packages that define a `lint` script (the marker
 *           of a CI-gated web package). Gates: pnpm typecheck/lint/test (subset
 *           that the package actually defines).
 *   - bench    : `benchmarks/ci-free-gates.sh` if present (runs when py is touched).
 *   - migration: the dir holding `alembic.ini`, gated on a migrations/alembic diff.
 *   - taskSources: a gh-issues source inferred from `git remote get-url origin`.
 *
 * Anything exotic (other languages/frameworks, custom gates, exclusions) is the
 * job of the `openspec/hooks/resolve-gates` override, not this file. See
 * docs/hooks.md and gate-resolver.js for the resolution chain.
 */

const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');

// Dirs we never descend into when globbing manifests.
const PRUNE = new Set(['node_modules', '.venv', '.git', 'vendor', 'dist', 'build', '.next']);
// Path fragments that mark a transient copy (agent worktrees) — never gated.
const PRUNE_FRAGMENTS = ['/.claude/worktrees/'];

// ---- repo root -----------------------------------------------------------------

function findRepoRoot(startDir) {
  let dir = path.resolve(startDir || process.cwd());
  for (;;) {
    if (fs.existsSync(path.join(dir, '.git'))) return dir;
    const parent = path.dirname(dir);
    if (parent === dir) return path.resolve(startDir || process.cwd());
    dir = parent;
  }
}

// ---- generic manifest globber --------------------------------------------------

// Return repo-relative dirs that directly contain `filename`.
function manifestDirs(root, filename) {
  const out = [];
  const walk = (abs) => {
    let entries;
    try { entries = fs.readdirSync(abs, { withFileTypes: true }); }
    catch { return; }
    if (entries.some((e) => e.isFile() && e.name === filename)) {
      const rel = path.relative(root, abs);
      if (rel) out.push(rel.split(path.sep).join('/'));
    }
    for (const e of entries) {
      if (!e.isDirectory()) continue;
      if (PRUNE.has(e.name)) continue;
      const childRel = '/' + path.relative(root, path.join(abs, e.name)).split(path.sep).join('/') + '/';
      if (PRUNE_FRAGMENTS.some((f) => childRel.includes(f))) continue;
      walk(path.join(abs, e.name));
    }
  };
  walk(root);
  return out.sort();
}

function readFileSafe(p) {
  try { return fs.readFileSync(p, 'utf8'); } catch { return null; }
}

// ---- gate command builders (conventional defaults per toolchain) ---------------

function pyGates(dir) {
  return [
    { name: 'lint', cmd: `uv --directory ${dir} run ruff check .` },
    { name: 'format', cmd: `uv --directory ${dir} run ruff format --check .` },
    { name: 'typecheck', cmd: `uv --directory ${dir} run pyright` },
    { name: 'test', cmd: `uv --directory ${dir} run python -m pytest -q` },
    { name: 'coverage', cmd: `uv --directory ${dir} run python -m pytest -q --cov --cov-report=term-missing` },
  ];
}
function goGates(dir) {
  return [
    { name: 'build', cmd: `(cd ${dir} && go build ./...)` },
    { name: 'vet', cmd: `(cd ${dir} && go vet ./...)` },
    { name: 'test', cmd: `(cd ${dir} && go test -race ./...)` },
    { name: 'coverage', cmd: `(cd ${dir} && go test -race -coverprofile=cover.out ./... && go tool cover -func=cover.out | tail -1)` },
  ];
}
// ts gates are per-package: only the subset of {typecheck,lint,test} the package defines.
function tsGates(dir, scripts) {
  const want = ['typecheck', 'lint', 'test'];
  return want.filter((s) => scripts[s]).map((s) => ({ name: s, cmd: `(cd ${dir} && pnpm ${s})` }));
}

// ---- per-toolchain detectors ---------------------------------------------------

function parseUvMembers(text) {
  const i = text.indexOf('[tool.uv.workspace]');
  if (i < 0) return null;
  const m = text.slice(i).match(/members\s*=\s*\[([\s\S]*?)\]/);
  if (!m) return [];
  const body = m[1].split('\n').map((l) => l.replace(/#.*$/, '')).join('\n');
  const found = body.match(/["']([^"']+)["']/g) || [];
  return found.map((s) => s.slice(1, -1));
}

function discoverPy(root) {
  const pyproject = readFileSafe(path.join(root, 'pyproject.toml'));
  let dirs = pyproject ? parseUvMembers(pyproject) : null;
  if (!dirs) {
    // No uv workspace — every dir with a pyproject.toml (except the root) is a unit.
    dirs = manifestDirs(root, 'pyproject.toml').filter((d) => d !== '');
  }
  return dirs.filter(Boolean);
}

function discoverGo(root) {
  return manifestDirs(root, 'go.mod');
}

function parsePnpmPackages(text) {
  const out = [];
  let inPkgs = false;
  for (const line of text.split('\n')) {
    if (/^packages:\s*$/.test(line)) { inPkgs = true; continue; }
    if (!inPkgs) continue;
    const m = line.match(/^\s*-\s*['"]?([^'"#\s]+)['"]?/);
    if (m) { out.push(m[1]); continue; }
    if (/^\S/.test(line)) break; // next top-level key
  }
  return out;
}

function expandGlob(root, pattern) {
  // Support exact paths and a single trailing "/*".
  if (pattern.endsWith('/*')) {
    const base = pattern.slice(0, -2);
    const absBase = path.join(root, base);
    let entries;
    try { entries = fs.readdirSync(absBase, { withFileTypes: true }); } catch { return []; }
    return entries.filter((e) => e.isDirectory() && !PRUNE.has(e.name)).map((e) => `${base}/${e.name}`);
  }
  return [pattern];
}

function discoverTs(root) {
  const ws = readFileSafe(path.join(root, 'pnpm-workspace.yaml'));
  let candidates;
  if (ws) {
    candidates = parsePnpmPackages(ws).flatMap((p) => expandGlob(root, p));
  } else {
    // No pnpm workspace — every dir with a package.json (except root) is a candidate.
    candidates = manifestDirs(root, 'package.json').filter((d) => d !== '');
  }
  const units = [];
  for (const dir of candidates) {
    const pkg = readFileSafe(path.join(root, dir, 'package.json'));
    if (!pkg) continue;
    let scripts = {};
    try { scripts = JSON.parse(pkg).scripts || {}; } catch { continue; }
    // The `lint` script is the marker that a JS package opts into CI gates
    // (keeps e2e/test-only and asset-only packages out of the per-change loop).
    if (!scripts.lint) continue;
    units.push({ dir, scripts });
  }
  return units;
}

// ---- task source inference -----------------------------------------------------

function inferGithubRepo(root) {
  let url = '';
  try {
    url = execFileSync('git', ['-C', root, 'remote', 'get-url', 'origin'], { encoding: 'utf8' }).trim();
  } catch { return null; }
  // git@github.com:owner/repo.git  |  https://github.com/owner/repo(.git)
  const m = url.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?$/);
  return m ? m[1] : null;
}

function discoverTaskSources(startDir) {
  const repo = inferGithubRepo(findRepoRoot(startDir));
  if (!repo) return [];
  return [{ name: 'github', type: 'gh-issues', enabled: true, config: { repo, label: '' } }];
}

// ---- assemble the config-shaped object -----------------------------------------

function discover(startDir) {
  const root = findRepoRoot(startDir);
  const toolchains = {};

  // Declare `go` before `py` so that, on an equal-length prefix tie (a dir that is
  // both a go module and a uv member, e.g. a dual-language package), Go wins — the
  // resolver breaks ties by declaration order.
  const go = discoverGo(root);
  if (go.length) toolchains.go = { dirs: go, gates: [] /* per-dir, built below */ };
  const py = discoverPy(root);
  if (py.length) toolchains.py = { dirs: py, gates: [] };
  const ts = discoverTs(root);
  if (ts.length) toolchains.ts = { dirs: ts.map((u) => u.dir), gates: [] };

  // The resolver applies `tc.gates` (with {dir} substitution) to every matched dir.
  // py/go gates are uniform, so a single templated list works. ts gates differ per
  // package (subset of scripts), so we attach them via a per-dir map the resolver
  // reads through `tc.gatesByDir` (falls back to `tc.gates`).
  if (toolchains.go) toolchains.go.gates = goGates('{dir}');
  if (toolchains.py) toolchains.py.gates = pyGates('{dir}');
  if (toolchains.ts) {
    toolchains.ts.gates = tsGates('{dir}', { typecheck: 1, lint: 1, test: 1 }); // default superset
    toolchains.ts.gatesByDir = {};
    for (const u of ts) toolchains.ts.gatesByDir[u.dir] = tsGates('{dir}', u.scripts);
  }

  // bench ladder — only if the conventional free-gate script exists.
  const bench = { prefix: 'benchmarks/', dir: 'benchmarks', alsoWhenToolchains: ['py'], gates: [] };
  if (fs.existsSync(path.join(root, 'benchmarks', 'ci-free-gates.sh'))) {
    bench.gates = [{ name: 'free-ladder', cmd: 'bash benchmarks/ci-free-gates.sh' }];
  }

  // migration gate — the dir that owns alembic.ini, run only on a migrations diff.
  const migration = { pattern: '(^|/)(migrations|alembic)/' };
  const alembicDirs = manifestDirs(root, 'alembic.ini');
  if (alembicDirs.length) {
    const d = alembicDirs[0];
    migration.gate = { name: 'migration', cmd: `uv --directory ${d} run alembic upgrade head  # needs DATABASE_URL` };
  }

  return {
    toolchains,
    bench,
    metaPrefixes: ['openspec/', 'docs/', '.claude/', '.github/'],
    always: [{ name: 'openspec-validate', cmd: 'node .claude/workflows/lib/openspec.js validate "<change>" --strict' }],
    migration,
    customGates: [],
    taskSources: discoverTaskSources(root),
    _discovered: true,
  };
}

module.exports = {
  discover,
  findRepoRoot,
  manifestDirs,
  parseUvMembers,
  parsePnpmPackages,
  inferGithubRepo,
  discoverTaskSources,
  pyGates,
  goGates,
  tsGates,
};

if (require.main === module) {
  process.stdout.write(JSON.stringify(discover(), null, 2) + '\n');
}
