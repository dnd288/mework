#!/usr/bin/env node
'use strict';
/*
 * gate-resolver — map a change's touched files to the per-toolchain quality gates
 * that must run in the OpenSpec Verify step.
 *
 * mzspec layers a gated delivery pipeline on top of OpenSpec. A project can be a
 * polyglot monorepo (e.g. a Python uv workspace + standalone Go modules + a pnpm
 * web app), so Verify cannot run one fixed toolchain. This module resolves
 * `git diff --name-only` output into the exact, deduped set of gate commands.
 *
 * Resolution chain (no centralized config required) — see docs/hooks.md:
 *   1. openspec/hooks/resolve-gates (executable)  → run it; its stdout JSON IS the
 *      plan. The universal, fully-customizable override for ANY framework/language.
 *   2. mzspec.config.json present                 → config-driven (back-compat).
 *   3. otherwise                                  → zero-config auto-discovery
 *      (lib/discover.js synthesizes the inventory from the repo's own manifests).
 *
 * Usage (runtime, from the repo root, called by the ship-code Verify agent):
 *   git diff --name-only <base>...HEAD | node .claude/workflows/lib/gate-resolver.js --stdin --json
 *   node .claude/workflows/lib/gate-resolver.js packages/rag-core/x.py apps/portal/y.ts
 *
 * Output (--json): { toolchains, units:[{toolchain,unitDir,gates:[{name,cmd}]}],
 *                    flags:{touchesMigrations,touchesBench,touchesSubmoduleOnly},
 *                    always:[...], custom:[{name,cmd}], source:'hook'|'config'|'discover' }
 * Default output: a human-readable, copy-pasteable gate plan.
 *
 * Pure `resolve(files, config?)` is exported for unit tests; when `config` is
 * omitted it comes from getConfig() (config file if present, else discovery).
 * `resolvePlan(files)` adds the hook override on top of resolve().
 */

const fs = require('fs');
const path = require('path');
const { loadConfig, findConfigPath } = require('./load-config.js');
const { discover } = require('./discover.js');
const { runHook } = require('./run-hook.js');

// Config source: an explicit mzspec.config.json wins (back-compat); otherwise the
// repo is described entirely by its own manifests via discovery (zero config).
function getConfig(startDir) {
  return findConfigPath(startDir) ? loadConfig(startDir) : discover(startDir);
}

// ---- helpers -------------------------------------------------------------------

function longestPrefixMatch(file, dirs) {
  let best = null;
  for (const d of dirs) {
    if (file === d || file.startsWith(d + '/')) {
      if (!best || d.length > best.length) best = d;
    }
  }
  return best;
}

function subst(cmd, vars) {
  return cmd.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? vars[k] : m));
}

function benchCfg(config) {
  return config.bench || { prefix: 'benchmarks/', dir: 'benchmarks', gates: [], alsoWhenToolchains: [] };
}

// ---- classification ------------------------------------------------------------

function classify(file, config = getConfig()) {
  // Returns { toolchain, unitDir } or { toolchain: 'meta' }.
  // Iterate toolchains in declared order so that, on an equal-length prefix tie,
  // the first-declared toolchain wins (e.g. declare `go` before `py` to make the
  // Go module list authoritative for a name that also looks like a Python member).
  const candidates = [];
  let order = 0;
  for (const [name, tc] of Object.entries(config.toolchains)) {
    const hit = longestPrefixMatch(file, tc.dirs || []);
    if (hit) candidates.push({ toolchain: name, unitDir: hit, order: order });
    order += 1;
  }
  if (candidates.length) {
    candidates.sort((a, b) => b.unitDir.length - a.unitDir.length || a.order - b.order);
    return { toolchain: candidates[0].toolchain, unitDir: candidates[0].unitDir };
  }
  const bench = benchCfg(config);
  if (bench.prefix && file.startsWith(bench.prefix)) {
    return { toolchain: 'bench', unitDir: bench.dir || 'benchmarks' };
  }
  for (const p of config.metaPrefixes || []) if (file.startsWith(p)) return { toolchain: 'meta' };
  if (!file.includes('/') && file.endsWith('.md')) return { toolchain: 'meta' };
  // Unknown path (e.g. repo-root config, scripts/) → treat as meta (validate only).
  return { toolchain: 'meta' };
}

// ---- resolution ----------------------------------------------------------------

function gatesFor(c, config) {
  if (c.toolchain === 'bench') {
    return (benchCfg(config).gates || []).map((g) => ({ name: g.name, cmd: subst(g.cmd, { dir: c.unitDir }) }));
  }
  const tc = config.toolchains[c.toolchain];
  if (!tc) return [];
  // Per-dir gate overrides (e.g. discovery's per-package ts script subset) win
  // over the uniform templated list.
  const list = (tc.gatesByDir && tc.gatesByDir[c.unitDir]) || tc.gates || [];
  return list.map((g) => ({ name: g.name, cmd: subst(g.cmd, { dir: c.unitDir }) }));
}

function resolve(files, config = getConfig()) {
  const bench = benchCfg(config);
  const migration = config.migration || {};
  const migrationRe = migration.pattern ? new RegExp(migration.pattern) : null;

  const flags = { touchesMigrations: false, touchesBench: false, touchesSubmoduleOnly: true };
  const seen = new Map(); // key `${toolchain}:${unitDir}` -> {toolchain, unitDir}
  for (const raw of files) {
    const file = String(raw).trim();
    if (!file) continue;
    if (migrationRe && migrationRe.test(file)) flags.touchesMigrations = true;
    if (bench.prefix && file.startsWith(bench.prefix)) flags.touchesBench = true;
    const c = classify(file, config);
    if (c.toolchain !== 'meta') flags.touchesSubmoduleOnly = false;
    if (c.toolchain === 'meta') continue;
    const key = `${c.toolchain}:${c.unitDir || ''}`;
    if (!seen.has(key)) seen.set(key, c);
  }

  const units = [...seen.values()].map((c) => ({
    toolchain: c.toolchain,
    unitDir: c.unitDir,
    gates: gatesFor(c, config),
  }));
  const toolchains = [...new Set(units.map((u) => u.toolchain))];

  // The free bench ladder runs whenever a configured trigger toolchain (default
  // none beyond what the project sets) is touched, OR any bench path is touched.
  const trigger = bench.alsoWhenToolchains || [];
  if ((bench.gates && bench.gates.length) && (trigger.some((t) => toolchains.includes(t)) || flags.touchesBench)) {
    if (!units.some((u) => u.toolchain === 'bench')) {
      units.push({ toolchain: 'bench', unitDir: bench.dir || 'benchmarks', gates: gatesFor({ toolchain: 'bench', unitDir: bench.dir || 'benchmarks' }, config) });
      if (!toolchains.includes('bench')) toolchains.push('bench');
    }
  }

  // Always-gates (e.g. openspec validate), plus the migration gate when relevant.
  const always = (config.always || []).map((g) => ({ name: g.name, cmd: g.cmd }));
  if (flags.touchesMigrations && migration.gate) {
    always.push({ name: migration.gate.name, cmd: migration.gate.cmd });
  }

  // Custom project gates — the plugin hook. Each entry: { name, cmd, when }.
  // `when` may specify { touchesMigrations|touchesBench: true } and/or
  // { touches: '<prefix>' } / { toolchains: ['py', ...] }. Empty `when` = always.
  const custom = [];
  for (const g of config.customGates || []) {
    const w = g.when || {};
    let match = true;
    if (w.touchesMigrations && !flags.touchesMigrations) match = false;
    if (w.touchesBench && !flags.touchesBench) match = false;
    if (w.touches && !files.some((f) => String(f).startsWith(w.touches))) match = false;
    if (Array.isArray(w.toolchains) && !w.toolchains.some((t) => toolchains.includes(t))) match = false;
    if (match) custom.push({ name: g.name, cmd: g.cmd });
  }

  return { toolchains, units, flags, always, custom, source: config._discovered ? 'discover' : 'config' };
}

// ---- hook override -------------------------------------------------------------

// openspec/hooks/resolve-gates — the universal, per-project customization point.
// If present and executable, it fully owns gate resolution: changed files arrive
// on stdin (one per line), and its stdout JSON IS the returned plan. The executable
// contract lives in lib/run-hook.js (shared with the task-lifecycle on-<event> hooks).
function resolvePlan(files, startDir) {
  const hooked = runHook('resolve-gates', files.join('\n') + '\n', startDir);
  if (hooked) { hooked.source = 'hook'; return hooked; }
  return resolve(files, getConfig(startDir));
}

// ---- CLI -----------------------------------------------------------------------

function render(plan) {
  const lines = [];
  lines.push(`# Gate plan — toolchains: ${plan.toolchains.join(', ') || '(none / meta-only)'}`);
  lines.push(`# flags: ${JSON.stringify(plan.flags)}`);
  for (const u of plan.units) {
    lines.push(`\n## ${u.toolchain}  ${u.unitDir}`);
    for (const g of u.gates) lines.push(`${g.cmd}`);
  }
  if (plan.custom && plan.custom.length) {
    lines.push(`\n## custom`);
    for (const g of plan.custom) lines.push(`${g.cmd}`);
  }
  lines.push(`\n## always`);
  for (const g of plan.always) lines.push(`${g.cmd}`);
  return lines.join('\n');
}

function main(argv) {
  const args = argv.slice(2);
  const json = args.includes('--json');
  const useStdin = args.includes('--stdin');
  const fileArgs = args.filter((a) => !a.startsWith('--'));
  const run = (files) => {
    const plan = resolvePlan(files);
    process.stdout.write(json ? JSON.stringify(plan, null, 2) + '\n' : render(plan) + '\n');
  };
  if (useStdin) {
    let buf = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (d) => (buf += d));
    process.stdin.on('end', () => run(buf.split('\n').filter(Boolean)));
  } else {
    run(fileArgs);
  }
}

if (require.main === module) main(process.argv);
module.exports = { resolve, resolvePlan, classify, getConfig };
