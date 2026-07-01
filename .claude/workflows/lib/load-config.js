#!/usr/bin/env node
'use strict';
/*
 * load-config — locate and load a project's mzspec.config.json.
 *
 * The config is the single source of truth for the per-toolchain package
 * inventory + gate commands that gate-resolver.js consumes. It is searched for
 * by walking up from `startDir` (default: cwd) to the filesystem root, so the
 * resolver works regardless of which subdirectory it is invoked from.
 *
 * Exported:
 *   findConfigPath(startDir?)  -> absolute path | null
 *   loadConfig(startDir?)      -> validated config object (throws if missing/invalid)
 */

const fs = require('fs');
const path = require('path');

const CONFIG_NAME = 'mzspec.config.json';

function findConfigPath(startDir) {
  let dir = path.resolve(startDir || process.cwd());
  // Walk up to the root looking for the config file.
  for (;;) {
    const candidate = path.join(dir, CONFIG_NAME);
    if (fs.existsSync(candidate)) return candidate;
    const parent = path.dirname(dir);
    if (parent === dir) return null; // reached filesystem root
    dir = parent;
  }
}

// Minimal structural validation — enough to fail fast with a clear message
// rather than throwing deep inside the resolver.
function validate(config, sourcePath) {
  const where = sourcePath ? ` (${sourcePath})` : '';
  if (!config || typeof config !== 'object') {
    throw new Error(`mzspec config${where} is not an object`);
  }
  if (!config.toolchains || typeof config.toolchains !== 'object') {
    throw new Error(`mzspec config${where} is missing a "toolchains" object`);
  }
  for (const [name, tc] of Object.entries(config.toolchains)) {
    if (!Array.isArray(tc.dirs)) {
      throw new Error(`mzspec config${where}: toolchains.${name}.dirs must be an array`);
    }
    if (!Array.isArray(tc.gates)) {
      throw new Error(`mzspec config${where}: toolchains.${name}.gates must be an array`);
    }
  }
  if (!Array.isArray(config.always)) {
    throw new Error(`mzspec config${where} is missing an "always" array`);
  }
  if (config.taskSources !== undefined) {
    if (!Array.isArray(config.taskSources)) {
      throw new Error(`mzspec config${where}: "taskSources" must be an array`);
    }
    for (const s of config.taskSources) {
      if (!s || typeof s !== 'object' || !s.name || !s.type) {
        throw new Error(`mzspec config${where}: each taskSources entry needs { name, type, enabled?, config? }`);
      }
    }
  }
  return config;
}

function loadConfig(startDir) {
  const p = findConfigPath(startDir);
  if (!p) {
    throw new Error(
      `Could not find ${CONFIG_NAME} walking up from ${path.resolve(startDir || process.cwd())}. ` +
        `Run the mzspec installer or copy templates/mzspec.config.template.json to your repo root.`
    );
  }
  let raw;
  try {
    raw = fs.readFileSync(p, 'utf8');
  } catch (e) {
    throw new Error(`Could not read ${p}: ${e.message}`);
  }
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (e) {
    throw new Error(`Could not parse ${p} as JSON: ${e.message}`);
  }
  return validate(parsed, p);
}

module.exports = { findConfigPath, loadConfig, validate, CONFIG_NAME };
