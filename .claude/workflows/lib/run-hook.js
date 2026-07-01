#!/usr/bin/env node
'use strict';
/*
 * run-hook — the shared executable-hook contract for mzspec.
 *
 * A hook is an executable file at `<repoRoot>/openspec/hooks/<name>`. mzspec
 * discovers it, pipes a context payload to its stdin, and reads JSON from its
 * stdout. Any language works (the file just needs a shebang + the executable bit).
 *
 * This is the single place that owns the contract; both the gate resolver
 * (`resolve-gates`) and the task-lifecycle dispatcher (`on-<event>`) use it.
 *
 *   - hookPath(name, startDir)  -> absolute path of the hook
 *   - isExecutable(p)           -> true only for an executable regular file
 *   - runHook(name, stdin, dir) -> null when the hook is absent/not executable;
 *                                  otherwise the parsed JSON stdout ({} when the
 *                                  hook prints nothing). Throws on non-zero exit or
 *                                  invalid JSON — the CALLER decides whether that's
 *                                  fatal (gates: fall back to discovery) or
 *                                  ignorable (lifecycle: log + continue, best-effort).
 */

const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
const { findRepoRoot } = require('./discover.js');

function hookPath(name, startDir) {
  return path.join(findRepoRoot(startDir), 'openspec', 'hooks', name);
}

function isExecutable(p) {
  try { fs.accessSync(p, fs.constants.X_OK); return fs.statSync(p).isFile(); }
  catch { return false; }
}

function runHook(name, stdinStr, startDir) {
  const p = hookPath(name, startDir);
  if (!isExecutable(p)) return null;
  const out = execFileSync(p, [], { input: stdinStr == null ? '' : String(stdinStr), encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 });
  const trimmed = (out || '').trim();
  return trimmed ? JSON.parse(trimmed) : {};
}

module.exports = { hookPath, isExecutable, runHook, findRepoRoot };
