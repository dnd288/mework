#!/usr/bin/env node
// openspec.js — native OpenSpec CLI replacement for mzspec
// Reimplements the subset of openspec commands that mzspec workflows use.
// No external dependencies — pure Node.js.
//
// Usage:
//   node openspec.js init
//   node openspec.js new change <name>
//   node openspec.js validate <change> [--strict]
//   node openspec.js status --change <name> --json
//   node openspec.js list --json
//   node openspec.js archive <change> [-y]
//
// All commands operate on the openspec/ directory in the current working directory.

const fs = require('fs')
const path = require('path')

const CWD = process.cwd()
const OPENSPEC_DIR = path.join(CWD, 'openspec')
const CHANGES_DIR = path.join(OPENSPEC_DIR, 'changes')
const ARCHIVE_DIR = path.join(OPENSPEC_DIR, 'changes', 'archive')
const SPECS_DIR = path.join(OPENSPEC_DIR, 'specs')

// ─── Helpers ──────────────────────────────────────────────────────────

function die(msg, code = 1) {
  process.stderr.write(`error: ${msg}\n`)
  process.exit(code)
}

function readJSON(p) {
  try { return JSON.parse(fs.readFileSync(p, 'utf8')) } catch { return null }
}

function writeJSON(p, obj) {
  fs.mkdirSync(path.dirname(p), { recursive: true })
  fs.writeFileSync(p, JSON.stringify(obj, null, 2) + '\n')
}

function changeDir(name) {
  return path.join(CHANGES_DIR, name)
}

function changeExists(name) {
  return fs.existsSync(changeDir(name))
}

function findChanges() {
  if (!fs.existsSync(CHANGES_DIR)) return []
  return fs.readdirSync(CHANGES_DIR).filter(d =>
    fs.statSync(path.join(CHANGES_DIR, d)).isDirectory() && d !== 'archive'
  )
}

function findArchive() {
  if (!fs.existsSync(ARCHIVE_DIR)) return []
  return fs.readdirSync(ARCHIVE_DIR).filter(d =>
    fs.statSync(path.join(ARCHIVE_DIR, d)).isDirectory()
  )
}

function findSpecFiles(dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...findSpecFiles(fullPath))
    } else if (entry.name.endsWith('.md')) {
      files.push(fullPath)
    }
  }
  return files
}

function changeInfo(name) {
  const dir = changeDir(name)
  if (!fs.existsSync(dir)) return null
  const proposalPath = path.join(dir, 'proposal.md')
  const designPath = path.join(dir, 'design.md')
  const uiPath = path.join(dir, 'ui.md')
  const tasksPath = path.join(dir, 'tasks.md')
  const specsDir = path.join(dir, 'specs')
  const specFiles = fs.existsSync(specsDir)
    ? findSpecFiles(specsDir).map(f => path.relative(dir, f))
    : []
  let title = name
  if (fs.existsSync(proposalPath)) {
    const content = fs.readFileSync(proposalPath, 'utf8')
    const lines = content.split('\n')
    let startIdx = 0
    if (lines[0] && lines[0].trim() === '---') {
      const endIdx = lines.findIndex((l, i) => i > 0 && l.trim() === '---')
      startIdx = endIdx !== -1 ? endIdx + 1 : 0
    }
    for (let i = startIdx; i < lines.length; i++) {
      const line = lines[i].trim()
      if (line.startsWith('# ')) { title = line.replace(/^#\s*/, '').trim(); break }
    }
  }
  return {
    changeRoot: dir,
    proposalPath: fs.existsSync(proposalPath) ? proposalPath : null,
    designPath: fs.existsSync(designPath) ? designPath : null,
    tasksPath: fs.existsSync(tasksPath) ? tasksPath : null,
    uiPath: fs.existsSync(uiPath) ? uiPath : null,
    specPaths: specFiles,
    title,
    name,
  }
}

// ─── Validate ──────────────────────────────────────────────────────────

function validateChange(name, strict) {
  const info = changeInfo(name)
  if (!info) die(`change "${name}" not found`)
  const errors = []
  const warnings = []

  // Check required files
  if (!info.proposalPath) errors.push('missing proposal.md')
  if (!info.designPath) warnings.push('missing design.md (optional but recommended)')
  if (!info.uiPath) warnings.push('missing ui.md (optional — only needed for UI changes)')
  if (!info.tasksPath) errors.push('missing tasks.md')

  // Check proposal.md has frontmatter
  if (info.proposalPath) {
    const content = fs.readFileSync(info.proposalPath, 'utf8')
    if (!content.startsWith('---\n')) errors.push('proposal.md must start with --- frontmatter')
    else {
      const endIdx = content.indexOf('---\n', 4)
      if (endIdx === -1) errors.push('proposal.md frontmatter not closed (no second ---)')
    }
  }

  // Check delta specs for ADDED/MODIFIED/REMOVED/RENAMED format
  for (const sp of info.specPaths) {
    const fullPath = path.join(info.changeRoot, sp)
    if (!fs.existsSync(fullPath)) continue
    const content = fs.readFileSync(fullPath, 'utf8')
    const firstLine = content.trim().split('\n')[0] || ''
    if (strict) {
      const validPrefixes = ['# ADDED', '# MODIFIED', '# REMOVED', '# RENAMED']
      const ok = validPrefixes.some(p => firstLine.startsWith(p))
      if (!ok) errors.push(`${sp}: first line must start with one of: ${validPrefixes.join(', ')}`)
    }
  }

  // Check canonical specs exist for MODIFIED/REMOVED refs
  for (const sp of info.specPaths) {
    const fullPath = path.join(info.changeRoot, sp)
    if (!fs.existsSync(fullPath)) continue
    const content = fs.readFileSync(fullPath, 'utf8')
    for (const line of content.split('\n')) {
      const m = line.match(/^\*\*`([^`]+)`\*\*/)
      if (m) {
        const canonicalPath = path.join(SPECS_DIR, m[1])
        if (!fs.existsSync(canonicalPath)) {
          warnings.push(`${sp}: references "${m[1]}" but canonical spec not found at ${canonicalPath}`)
        }
      }
    }
  }

  // Output results
  if (errors.length > 0) {
    for (const e of errors) process.stderr.write(`FAIL: ${e}\n`)
    for (const w of warnings) process.stderr.write(`WARN: ${w}\n`)
    die(`validate: ${errors.length} error(s), ${warnings.length} warning(s)`)
  }
  for (const w of warnings) process.stderr.write(`WARN: ${w}\n`)
  process.stdout.write(`validate: ok (${info.specPaths.length} spec(s), ${warnings.length} warning(s))\n`)
}

// ─── Status ────────────────────────────────────────────────────────────

function statusChange(name) {
  const info = changeInfo(name)
  if (!info) {
    process.stdout.write(JSON.stringify({ error: 'not found', change: name }) + '\n')
    return
  }
  process.stdout.write(JSON.stringify(info) + '\n')
}

// ─── List ──────────────────────────────────────────────────────────────

function listChanges() {
  const active = findChanges()
  const archived = findArchive()
  const result = { active, archived }
  process.stdout.write(JSON.stringify(result) + '\n')
}

// ─── New Change ────────────────────────────────────────────────────────

function newChange(name) {
  if (!name) die('change name required')
  if (changeExists(name)) die(`change "${name}" already exists`)
  const dir = changeDir(name)
  fs.mkdirSync(path.join(dir, 'specs'), { recursive: true })
  fs.mkdirSync(path.join(dir, 'review'), { recursive: true })
  const proposal = `---\nname: "${name}"\n---\n\n# ${name}\n\n`
  fs.writeFileSync(path.join(dir, 'proposal.md'), proposal)
  fs.writeFileSync(path.join(dir, 'design.md'), `# Design — ${name}\n\n`)
  fs.writeFileSync(path.join(dir, 'tasks.md'), `# Tasks — ${name}\n\n`)
  process.stdout.write(`created change: ${name}\n`)
}

// ─── Archive ────────────────────────────────────────────────────────────

function archiveChange(name) {
  if (!changeExists(name)) die(`change "${name}" not found`)
  const src = changeDir(name)
  const date = new Date().toISOString().slice(0, 10)
  const dest = path.join(ARCHIVE_DIR, `${date}-${name}`)
  if (fs.existsSync(dest)) die(`archive target already exists: ${dest}`)
  fs.mkdirSync(ARCHIVE_DIR, { recursive: true })
  fs.renameSync(src, dest)
  process.stdout.write(`archived: ${name} -> ${dest}\n`)
}

// ─── Init ──────────────────────────────────────────────────────────────

function initProject() {
  const created = []
  for (const dir of ['specs', 'changes', 'changes/archive', 'hooks', 'templates']) {
    const p = path.join(OPENSPEC_DIR, dir)
    if (!fs.existsSync(p)) {
      fs.mkdirSync(p, { recursive: true })
      created.push(`openspec/${dir}/`)
    }
  }
  const marker = path.join(OPENSPEC_DIR, '.openspec.json')
  if (!fs.existsSync(marker)) {
    writeJSON(marker, { version: 1 })
    created.push('openspec/.openspec.json')
  }
  if (created.length > 0) {
    process.stdout.write(`initialized openspec/ (${created.join(', ')})\n`)
  } else {
    process.stdout.write('openspec/ already initialized\n')
  }
}

// ─── Instructions (lightweight: returns artifact paths) ────────────────

function instructions(artifact, changeName) {
  const dir = changeDir(changeName)
  const paths = {
    proposal: path.join(dir, 'proposal.md'),
    design: path.join(dir, 'design.md'),
    ui: path.join(dir, 'ui.md'),
    tasks: path.join(dir, 'tasks.md'),
  }
  const result = { change: changeName, changeRoot: dir }
  if (artifact === 'proposal') result.path = paths.proposal
  else if (artifact === 'design') result.path = paths.design
  else if (artifact === 'ui') result.path = paths.ui
  else if (artifact === 'tasks') result.path = paths.tasks
  else result.available = Object.keys(paths)
  process.stdout.write(JSON.stringify(result) + '\n')
}

// ─── Main ──────────────────────────────────────────────────────────────

const args = process.argv.slice(2)
if (args.length === 0) {
  console.error('Usage: node openspec.js <command> [options]')
  process.exit(1)
}

const cmd = args[0]
switch (cmd) {
  case 'init':
    initProject()
    break

  case 'new':
    if (args[1] !== 'change') die('usage: openspec new change <name>')
    newChange(args[2])
    break

  case 'validate':
    {
      const changeIdx = args[1] ? (args[2] === '--strict' || args[2] === '--no-interactive' ? 1 : 2) : -1
      const changeName = args[1] && !args[1].startsWith('--') ? args[1] : args[2]
      const strict = args.includes('--strict')
      if (!changeName) die('usage: openspec validate <change> [--strict]')
      validateChange(changeName, strict)
    }
    break

  case 'status':
    {
      const idx = args.indexOf('--change')
      const changeName = idx !== -1 ? args[idx + 1] : null
      if (!changeName) die('usage: openspec status --change <name>')
      statusChange(changeName)
    }
    break

  case 'list':
    listChanges()
    break

  case 'archive':
    {
      const changeName = args[1]
      if (!changeName) die('usage: openspec archive <change> [-y]')
      archiveChange(changeName)
    }
    break

  case 'instructions':
    {
      const artifact = args[1]
      const idx = args.indexOf('--change')
      const changeName = idx !== -1 ? args[idx + 1] : null
      if (!artifact || !changeName) die('usage: openspec instructions <artifact> --change <name>')
      instructions(artifact, changeName)
    }
    break

  default:
    die(`unknown command: ${cmd}`)
}
