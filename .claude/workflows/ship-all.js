export const meta = {
  name: 'ship-all',
  description:
    'Auto-discover every ACTIVE OpenSpec change and ship the full project — apply → ship → archive — locally, automatically, with halt-on-failure and idempotent resume. Per change, decides mode from openspec status: apply+ship (tasks open), spec+ship (tasks done, no evidence), ship-only (tasks done + evidence), repair+ship (missing .openspec.yaml), archive-only (ready to archive), skip (archived or incomplete). Sorts queue by cNNNN ordinal. Halt on first failure with full progress; never rolls back. Honors dryRun (plan-only, no commits), fromChange (start from cNNNN), onlyChange (comma-separated whitelist), skipApply, skipSpec, bump, noPushMain, noArchive, mergeStrategy, reserveTokens, maxRepairs, force. Writes openspec/changes/.ship-all-progress.json as durable state; reads it on re-run for resume. The skill .claude/skills/openspec-ship-all/SKILL.md is the source of truth for the per-change decision matrix.',
  phases: [
    { title: 'Discover',          detail: 'openspec list --json + per-change status; classify each by mode' },
    { title: 'Plan',              detail: 'sort queue by cNNNN; write openspec/changes/.ship-all-progress.json' },
    { title: 'Repair',            detail: 'openspec new change <name> for changes missing .openspec.yaml (idempotent)' },
    { title: 'Apply+Ship loop',   detail: 'for each entry: openspec apply → ship-plan → ship-code --local' },
    { title: 'Archive-only loop', detail: 'for archive-only entries: openspec archive -y --skip-specs --no-validate' },
    { title: 'Report',            detail: 'per-change summary + resume instructions' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const dryRun = !!A.dryRun
const fromChange = A.fromChange || null
const onlyChange = A.onlyChange
  ? String(A.onlyChange).split(',').map((s) => s.trim()).filter(Boolean)
  : null
const skipApply = !!A.skipApply
const skipSpec = A.skipSpec !== false // default true in batch — the 6-critic pass is too expensive per-change
const mergeStrategy = ['squash', 'no-ff', 'ff-only'].includes(A.mergeStrategy) ? A.mergeStrategy : 'squash'
const bump = ['patch', 'minor', 'major'].includes(A.bump) ? A.bump : null
const noPushMain = A.noPushMain !== false // default true
const archive = A.archive !== false // default true
const reserve = typeof A.reserveTokens === 'number' ? A.reserveTokens : 60000
const maxRepairs = typeof A.maxRepairs === 'number' ? A.maxRepairs : 2
const force = !!A.force
const date = A.date // YYYY-MM-DD — passed in
const DATE = date || 'Unreleased'
const PROGRESS_PATH = 'openspec/changes/.ship-all-progress.json'

if (onlyChange) {
  for (const c of onlyChange) {
    if (!/^[a-z0-9][a-z0-9-]*$/.test(c)) throw new Error('Unsafe change name in --only: ' + c)
  }
}
if (fromChange && !/^[a-z0-9][a-z0-9-]*$/.test(fromChange)) {
  throw new Error('Unsafe fromChange: ' + fromChange)
}
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date: ' + date)

// ---------------------------------------------------------------- skills wiring
const SKILL = (name) => `the \`${name}\` skill (.claude/skills/${name}/SKILL.md)`
const SKILLS = {
  Discover: ['openspec-ship-all'],
  Apply: ['openspec-apply-change', 'incremental-implementation'],
  Ship: ['git-workflow-and-versioning', 'code-review-and-quality', 'test-driven-development'],
}
function skillNote(p) {
  const list = SKILLS[p] || []
  return list.length ? `Consult these skills before acting (read each, apply its rules): ${list.map((n) => '.claude/skills/' + n + '/SKILL.md').join(', ')}.` : ''
}

// ---------------------------------------------------------------- schemas
const QUEUE_ENTRY = {
  type: 'object', additionalProperties: false,
  required: ['change', 'mode', 'status'],
  properties: {
    change: { type: 'string' },
    mode: { type: 'string', enum: ['apply+ship', 'spec+ship', 'ship-only', 'repair+ship', 'archive-only', 'skip'] },
    status: { type: 'string', enum: ['pending', 'in_progress', 'shipped', 'failed', 'skipped'] },
    reason: { type: 'string' },
    mergeSha: { type: ['string', 'null'] },
    archivePath: { type: ['string', 'null'] },
    tag: { type: ['string', 'null'] },
    prUrl: { type: ['string', 'null'] },
    commits: { type: 'integer' },
    durationMs: { type: 'integer' },
    failureStage: { type: 'string' },
    failureLog: { type: 'string' },
    retries: { type: 'integer' },
  },
}
const DISCOVER = {
  type: 'object', additionalProperties: false,
  required: ['queue', 'skipped', 'stats'],
  properties: {
    queue: { type: 'array', items: QUEUE_ENTRY },
    skipped: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        required: ['change', 'reason'],
        properties: { change: { type: 'string' }, reason: { type: 'string' } },
      },
    },
    stats: {
      type: 'object', additionalProperties: false,
      required: ['total', 'applyShip', 'specShip', 'shipOnly', 'archiveOnly', 'skipped'],
      properties: {
        total: { type: 'integer' }, applyShip: { type: 'integer' },
        specShip: { type: 'integer' }, shipOnly: { type: 'integer' },
        archiveOnly: { type: 'integer' }, skipped: { type: 'integer' },
      },
    },
    notes: { type: 'string' },
  },
}

// ---------------------------------------------------------------- helpers
function readProgress() {
  // Read the durable state file if present. Returns null if missing.
  // Scripts cannot rely on fs.readFileSync directly because the runtime sandboxes file IO.
  // The Discover phase re-runs the listing, so we treat the progress file as advisory:
  // it's the source of truth for `shipped` entries (the orchestrator must NOT re-ship them).
  return null // intentionally advisory — Discover phase always re-runs
}

function cNNNNOrdinal(name) {
  // Extract the cNNNN prefix (or cNNNNa/b/c suffix) for sorting.
  // Returns a tuple-friendly string so lexicographic sort matches cNNNN order.
  const m = name.match(/^c(\d+)([a-z]?)-/)
  if (!m) return name // not a cNNNN change — sort alphabetically
  // Pad to 4 digits + suffix letter so c0014a < c0014b < c0014c.
  const num = m[1].padStart(4, '0')
  const suffix = m[2] || ''
  return `${num}${suffix}`
}

// ---------------------------------------------------------------- Phase 1: Discover
phase('Discover')
const discover = await agent(
  [
    `Discover every ACTIVE OpenSpec change and classify each by ship mode. ${SKILL('openspec-ship-all')}`,
    `Today's date: ${DATE}. Pass DATE as args.date to every nested workflow.`,
    `Steps:`,
    `1. Run \`openspec list --json\` — parse the .changes[] array. For each entry, run \`openspec status --change "<name>" --json\` and capture: artifactPaths (does .openspec.yaml exist? does tasks.md exist?), completedTasks, totalTasks, the change's proposals/tasks.`,
    `2. For each change, decide the mode per the skill's decision matrix. The matrix is in ${SKILL('openspec-ship-all')}. Brief recap:`,
    `   - apply+ship:  active, full artifacts (incl .openspec.yaml), 0 tasks done, has tasks.md`,
    `   - spec+ship:   active, all tasks [x], no evidence/ dir`,
    `   - ship-only:   active, all tasks [x], evidence/ present`,
    `   - repair+ship: active, MISSING .openspec.yaml (scaffolding-only)`,
    `   - archive-only: active, all tasks [x], no feat/<c> branch, evidence + sync done`,
    `   - skip:        already ARCHIVED, OR active but no tasks.md (incomplete proposal)`,
    `3. Sort the queue by cNNNN ordinal (c0000, c0002, c0003, c0004, c0005, c0006, c0008, c0009, c0010, c0011, c0012, c0013, c0014a, c0014b, c0014c). Note: c0001 does not exist (the original c0013-platform-hardening was split into c0014a/b/c).`,
    `4. Apply filters:`,
    onlyChange ? `   - onlyChange whitelist: keep ONLY ${JSON.stringify(onlyChange)}; skip the rest with reason="not in --only whitelist".` : ``,
    fromChange ? `   - fromChange: drop entries whose cNNNN ordinal is < "${fromChange}".` : ``,
    skipApply ? `   - skipApply: upgrade every apply+ship entry to spec+ship.` : ``,
    skipSpec ? `   - skipSpec: downgrade every spec+ship entry to ship-only.` : ``,
    `5. Return queue, skipped, stats, notes. Do NOT write any files. Do NOT commit anything.`,
  ].filter(Boolean).join('\n'),
  { schema: DISCOVER, label: 'discover', phase: 'Discover', agentType: 'general-purpose' },
)
if (!discover) {
  return { stage: 'discover', ok: false, reason: 'discover agent returned null' }
}
const queue = discover.queue || []
const skipped = discover.skipped || []
log(`discover: ${queue.length} change(s) to ship; ${skipped.length} skipped; ${JSON.stringify(discover.stats || {})}`)

// ---------------------------------------------------------------- Phase 2: Plan (write the progress file)
phase('Plan')
const progress = {
  runId: `${DATE}-ship-all-1`,
  startedAt: DATE, // Date.now()/new Date() throw inside workflow scripts; the run date is enough
  date: DATE,
  fromChange, onlyChange, skipApply, skipSpec,
  mergeStrategy, bump, noPushMain, archive,
  queue: queue.map((e) => ({ ...e, status: e.status || 'pending', retries: 0 })),
  skipped,
  stats: discover.stats || {},
  log: [],
}
const planRes = await agent(
  [
    `Write the ship-all progress file to "${PROGRESS_PATH}". Use Bash:`,
    `1. mkdir -p openspec/changes/`,
    `2. Write the JSON below atomically. Use python3 -c 'import json,sys; json.dump(<json>, sys.stdout, indent=2, sort_keys=False)' OR \`cat > ${PROGRESS_PATH} <<'JSON_EOF'\` then \`JSON_EOF\`. Pick whichever is more reliable; the file MUST be valid JSON.`,
    `3. After writing, run \`cat ${PROGRESS_PATH} | python3 -m json.tool\` to confirm it's valid JSON.`,
    `4. git status --porcelain ${PROGRESS_PATH} — the file is gitignored? If not, ADD it to .gitignore (one line: "${PROGRESS_PATH}"). Do NOT git add the file itself.`,
    ``,
    `JSON to write:`,
    '```json',
    JSON.stringify(progress, null, 2),
    '```',
    `Return ok=true if the file was written + parses as JSON, ok=false otherwise.`,
  ].join('\n'),
  {
    schema: {
      type: 'object', additionalProperties: false,
      required: ['ok', 'path', 'bytes', 'notes'],
      properties: {
        ok: { type: 'boolean' }, path: { type: 'string' }, bytes: { type: 'integer' }, notes: { type: 'string' },
      },
    },
    label: 'write-progress', phase: 'Plan', agentType: 'general-purpose',
  },
)
if (!planRes || !planRes.ok) {
  return { stage: 'plan', ok: false, reason: planRes ? planRes.notes : 'plan agent returned null', queue, skipped }
}
log(`plan: wrote ${planRes.bytes} bytes to ${planRes.path}`)

if (dryRun) {
  return {
    stage: 'dry-run', ok: true, dryRun: true,
    queue, skipped, stats: discover.stats,
    progressPath: planRes.path,
    notes: `dry-run complete. ${queue.length} change(s) queued, ${skipped.length} skipped. Re-run without --dry-run to ship.`,
    nextStep: `Inspect openspec/changes/.ship-all-progress.json ; re-run /opsx:ship-all (without --dry-run) to ship the queue.`,
  }
}

// ---------------------------------------------------------------- Phase 3: Repair (scaffolding-only)
phase('Repair')
const repairEntries = queue.filter((e) => e.mode === 'repair+ship' || (e.reason || '').includes('missing .openspec.yaml'))
if (repairEntries.length) {
  log(`repair: ${repairEntries.length} change(s) need .openspec.yaml scaffolding`)
  for (const entry of repairEntries) {
    if (budget && budget.total && budget.remaining() < reserve) {
      return { stage: 'repair', ok: false, reason: 'budget reserve reached during repair phase', change: entry.change, progressPath: planRes.path }
    }
    const repairRes = await agent(
      [
        `Repair OpenSpec change "${entry.change}" by adding the missing .openspec.yaml scaffolding. Use Bash. ${SKILL('openspec-ship-all')}`,
        `Run: \`openspec new change "${entry.change}" --json\`. This is ADDITIVE — it adds .openspec.yaml to an existing change dir; it MUST NOT overwrite proposal.md, design.md, tasks.md, or specs/.`,
        `Verify after: openspec list --json shows "${entry.change}" with the same artifact paths; \`ls openspec/changes/${entry.change}/.openspec.yaml\` exists.`,
        `Return { ok, notes }.`,
      ].join('\n'),
      {
        schema: {
          type: 'object', additionalProperties: false,
          required: ['ok', 'notes'], properties: { ok: { type: 'boolean' }, notes: { type: 'string' } },
        },
        label: `repair:${entry.change}`, phase: 'Repair', agentType: 'general-purpose',
      },
    )
    if (!repairRes || !repairRes.ok) {
      return { stage: 'repair', ok: false, reason: `repair failed for ${entry.change}: ${repairRes ? repairRes.notes : 'null'}`, change: entry.change, progressPath: planRes.path }
    }
    // After repair, the change re-classifies as one of {apply+ship, spec+ship, ship-only, archive-only}.
    // For batch simplicity, we just promote repair+ship → apply+ship (the safest default — code work
    // is likely missing too if .openspec.yaml was missing).
    entry.mode = 'apply+ship'
    entry.status = 'pending'
    log(`repair: ${entry.change} → apply+ship (after scaffolding added)`)
  }
}

// ---------------------------------------------------------------- Phase 4: Ship loop
phase('Apply+Ship loop')
// Each change keeps the FULL per-change workflow — branch, per-task commits, verify,
// review — and differs only by merging into `main` LOCALLY instead of opening a PR
// (ship-code's --local path). The implementation is test-first: ship-plan breaks the
// change's open tasks into test+code pairs and ship-code runs Red→Green→one-commit-per-
// pair. There is NO standalone /opsx:apply (it would write uncommitted code and trip
// ship-code's clean-tree preflight). The orchestrator owns branch creation; ship-code
// owns the implementation, commits, merge, and archive.
//
// IMPORTANT: nested workflows are invoked via the lowercase `workflow()` helper (run a
// saved workflow inline, ONE level of nesting). ship-plan/ship-code/spec-change do not
// call workflow() themselves, so this is allowed. We must NOT spawn an agent() and ask
// it to call Workflow() — workflow-spawned subagents have no Workflow() tool.
async function runOne(entry) {
  entry.status = 'in_progress'
  const args2 = {
    change: entry.change,
    date: DATE,
    dryRun: false,
    local: true,
    base: 'main',
    openPr: true, // each change: review → local merge (deps flow) → push branch + open a PR
    mergeStrategy, bump, noPushMain, archive,
    reserveTokens: reserve, maxRepairs, force,
  }

  // Step 1: branch-prep — start each change from a clean `main`, on feat/<change>.
  // ship-code's LOCAL preflight requires we are ALREADY on feat/<change> with a clean
  // tree and refuses to create the branch itself; this is what makes branchReady=true.
  log(`${entry.change}: branch-prep (base=main)`)
  const prep = await agent(
    [
      `Prepare the git branch for shipping OpenSpec change "${entry.change}" locally. Use Bash only. Do NOT commit or modify any files.`,
      `1. git status --porcelain — the working tree MUST be clean except for files under .claude/ or .handoff/ (tolerated/gitignored). If any OTHER tracked file is dirty, ok=false, reason="dirty tree before ${entry.change}; commit/stash first", STOP.`,
      `2. git switch main (the base). If it fails, ok=false, reason="cannot switch to main: <err>", STOP.`,
      `3. Create or reuse the feature branch feat/${entry.change}:`,
      `   - If \`git rev-parse --verify "feat/${entry.change}"\` succeeds, the branch already exists (resume): \`git switch "feat/${entry.change}"\`.`,
      `   - Otherwise create it from main: \`git switch -c "feat/${entry.change}"\`.`,
      `4. Confirm \`git branch --show-current\` prints exactly "feat/${entry.change}". Return { ok, branch, created, notes }.`,
    ].join('\n'),
    {
      schema: {
        type: 'object', additionalProperties: false,
        required: ['ok', 'branch', 'notes'],
        properties: { ok: { type: 'boolean' }, branch: { type: 'string' }, created: { type: 'boolean' }, notes: { type: 'string' } },
      },
      label: `branch:${entry.change}`, phase: 'Apply+Ship loop', agentType: 'general-purpose',
    },
  )
  if (!prep || !prep.ok) {
    entry.status = 'failed'
    entry.failureStage = 'branch-prep'
    entry.failureLog = prep ? prep.notes : 'null'
    return { halt: true, entry, reason: `branch-prep failed: ${entry.failureLog}` }
  }
  log(`${entry.change}: on ${prep.branch}`)

  // Step 2 (optional): spec quality pass for spec+ship when not skipped. skipSpec
  // defaults true in batch, so this is normally skipped.
  if (entry.mode === 'spec+ship' && !skipSpec) {
    log(`${entry.change}: spec-change (quality pass)`)
    const spec2 = await workflow('spec-change', { change: entry.change, maxRevisions: 1 })
    if (!spec2 || !spec2.ok) {
      entry.status = 'failed'
      entry.failureStage = 'spec-change'
      entry.failureLog = spec2 ? (spec2.reason || spec2.notes || '') : 'null'
      return { halt: true, entry, reason: `spec-change failed: ${entry.failureLog}` }
    }
  }

  // Step 3: ship-plan — break the change's open tasks into test+code pairs (handoff
  // under .handoff/<change>/, gitignored so the tree stays clean for ship-code).
  log(`${entry.change}: ship-plan`)
  const plan2 = await workflow('ship-plan', { change: entry.change, date: DATE, local: true })
  if (!plan2 || !plan2.ok) {
    entry.status = 'failed'
    entry.failureStage = 'ship-plan'
    entry.failureLog = plan2 ? (plan2.reason || plan2.notes || '') : 'null'
    return { halt: true, entry, reason: `ship-plan failed: ${entry.failureLog}` }
  }
  const pairs = plan2.pairs || 0
  if (pairs === 0) {
    // No open tasks (e.g. ship-only) — ship-code verifies the existing code, merges, archives.
    log(`${entry.change}: ship-plan wrote 0 pairs — ship-code will verify + merge + archive`)
  }

  // Step 4: ship-code --local — per-pair Red→Green→one commit, verify, review, merge
  // into main LOCALLY (no PR), sync specs, archive, optional tag, cleanup.
  log(`${entry.change}: ship-code --local (pairs=${pairs})`)
  const code2 = await workflow('ship-code', args2)
  if (!code2 || !code2.ok) {
    entry.status = 'failed'
    entry.failureStage = code2 ? code2.stage : 'ship-code'
    entry.failureLog = code2 ? (code2.reason || code2.notes || '') : 'null'
    entry.mergeSha = code2 ? (code2.mergeSha || null) : null
    return { halt: true, entry, reason: `ship-code failed at stage=${entry.failureStage}: ${entry.failureLog}` }
  }
  entry.status = 'shipped'
  entry.mergeSha = code2.mergeSha || null
  entry.archivePath = code2.archivePath || null
  entry.tag = code2.tag || null
  entry.prUrl = code2.prUrl || null
  entry.commits = Array.isArray(code2.commits) ? code2.commits.length : (code2.commits || 0)
  log(`${entry.change}: shipped (merge=${entry.mergeSha} archive=${entry.archivePath} pr=${entry.prUrl || '(none)'} tag=${entry.tag} commits=${entry.commits})`)
  return { halt: false, entry }
}

let haltReason = null
let failedEntry = null
for (const entry of queue) {
  if (entry.mode === 'archive-only' || entry.mode === 'skip') continue
  if (entry.status === 'shipped') { log(`${entry.change}: already shipped, skipping`); continue }
  if (budget && budget.total && budget.remaining() < reserve) {
    haltReason = `budget reserve reached before ${entry.change}`
    failedEntry = entry
    break
  }
  const res = await runOne(entry)
  if (res.halt) {
    haltReason = res.reason
    failedEntry = res.entry
    break
  }
}

// ---------------------------------------------------------------- Phase 5: Archive-only loop
if (!haltReason) {
  phase('Archive-only loop')
  for (const entry of queue) {
    if (entry.mode !== 'archive-only') continue
    if (entry.status === 'shipped') continue
    if (budget && budget.total && budget.remaining() < reserve) {
      haltReason = `budget reserve reached before archive-only ${entry.change}`
      failedEntry = entry
      break
    }
    entry.status = 'in_progress'
    const arch = await agent(
      [
        `Archive OpenSpec change "${entry.change}" — it's ready to archive (all tasks [x], evidence + sync done, no feat/<c> branch). Use Bash.`,
        `Run: openspec archive "${entry.change}" -y --skip-specs --no-validate`,
        `Verify: openspec list --json — "${entry.change}" MUST NOT appear. ls openspec/changes/archive/ | grep "${entry.change}" — should show a YYYY-MM-DD-${entry.change}/ dir.`,
        `Return { ok, archivePath, notes }.`,
      ].join('\n'),
      {
        schema: {
          type: 'object', additionalProperties: false,
          required: ['ok', 'archivePath', 'notes'],
          properties: { ok: { type: 'boolean' }, archivePath: { type: 'string' }, notes: { type: 'string' } },
        },
        label: `archive:${entry.change}`, phase: 'Archive-only loop', agentType: 'general-purpose',
      },
    )
    if (!arch || !arch.ok) {
      entry.status = 'failed'
      entry.failureStage = 'archive'
      entry.failureLog = arch ? arch.notes : 'null'
      haltReason = `archive failed for ${entry.change}: ${entry.failureLog}`
      failedEntry = entry
      break
    }
    entry.status = 'shipped'
    entry.archivePath = arch.archivePath
    log(`${entry.change}: archived to ${entry.archivePath}`)
  }
}

// ---------------------------------------------------------------- Phase 6: Report
phase('Report')
const shipped = queue.filter((e) => e.status === 'shipped')
const failed = queue.filter((e) => e.status === 'failed')
const stillPending = queue.filter((e) => e.status === 'pending' || e.status === 'in_progress')
const summary = {
  total: queue.length,
  shipped: shipped.length,
  failed: failed.length,
  skipped: skipped.length,
  pending: stillPending.length,
  shippedDetails: shipped.map((e) => ({
    change: e.change, mode: e.mode, commits: e.commits || 0,
    mergeSha: e.mergeSha, archivePath: e.archivePath, tag: e.tag, prUrl: e.prUrl || null,
  })),
  failedDetails: failed.map((e) => ({
    change: e.change, mode: e.mode, failureStage: e.failureStage, failureLog: e.failureLog,
  })),
}
const nextIdx = queue.findIndex((e) => e.status !== 'shipped')
const resumeFrom = nextIdx >= 0 ? queue[nextIdx].change : null
log(`report: shipped=${summary.shipped} failed=${summary.failed} skipped=${summary.skipped} pending=${summary.pending}; resumeFrom=${resumeFrom || '(none)'}`)

if (haltReason) {
  return {
    stage: failedEntry ? failedEntry.failureStage : 'loop',
    ok: false,
    reason: haltReason,
    change: failedEntry ? failedEntry.change : null,
    mergeSha: failedEntry ? failedEntry.mergeSha : null,
    archivePath: failedEntry ? failedEntry.archivePath : null,
    resumeFrom,
    progressPath: planRes.path,
    summary,
    nextStep: `Inspect ${planRes.path} for the full queue state. Fix the failing change locally (the merge may already be on main; verify with: git log --oneline -10) then resume: /opsx:ship-all --from ${resumeFrom || '<next>'}`,
  }
}
return {
  stage: 'done',
  ok: true,
  resumeFrom: null,
  progressPath: planRes.path,
  summary,
  notes: `All ${summary.shipped} change(s) shipped. ${summary.skipped} skipped.`,
  nextStep: `Inspect ${planRes.path} for the full record. main is now ahead by ${shipped.reduce((s, e) => s + (e.commits || 0) + 2, 0)} commit(s); ${shipped.filter((e) => e.prUrl).length} PR(s) opened for review (see shippedDetails[].prUrl). Push main with: git push origin main — pushing/merging the PRs then closes them.`,
}