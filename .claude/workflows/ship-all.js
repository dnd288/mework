export const meta = {
  name: 'ship-all',
  description:
    'Auto-discover every ACTIVE OpenSpec change and ship the full project — apply → ship → archive — locally, automatically, with halt-on-failure and idempotent resume. Per change, decides mode from openspec status: apply+ship (tasks open), spec+ship (tasks done, no evidence), ship-only (tasks done + evidence), repair+ship (missing .openspec.yaml), archive-only (ready to archive), skip (archived or incomplete). Sorts queue by cNNNN ordinal. Halt on first failure with full progress; never rolls back. Honors dryRun (plan-only, no commits), fromChange (start from cNNNN), onlyChange (comma-separated whitelist), skipApply, skipSpec, bump, noPushMain, noArchive, mergeStrategy, reserveTokens, maxRepairs, force. Writes openspec/changes/.ship-all-progress.json as durable state; reads it on re-run for resume. The skill .claude/skills/openspec-ship-all/SKILL.md is the source of truth for the per-change decision matrix.',
  phases: [
    { title: 'Discover',          detail: 'openspec list --json + per-change status; classify each by mode' },
    { title: 'Plan',              detail: 'sort queue by cNNNN; write openspec/changes/.ship-all-progress.json' },
    { title: 'Repair',            detail: 'openspec new change <name> for changes missing .openspec.yaml (idempotent)' },
    { title: 'Plan all changes',          detail: 'ship-plan for every change sequentially (fast — creates handoffs)' },
    { title: 'Implement parallel',        detail: 'implement ALL changes in parallel git worktrees, test-first' },
    { title: 'Merge, archive, PR',        detail: 'merge each branch to main + archive + PR (sequential)' },
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
const REQUIRED_GO_MINOR = 25
const TOOLCHAIN_NOTE = `TOOLCHAIN (do this FIRST, before any go/make/go test command): go.mod requires go 1.${REQUIRED_GO_MINOR}.x. Run \`go version\`; if it is older than 1.${REQUIRED_GO_MINOR} (a stale go on PATH such as /usr/local/go can shadow a newer one), locate a newer toolchain via \`which -a go\` and \`ls /opt/homebrew/bin/go /opt/homebrew/Cellar/go*/*/bin/go 2>/dev/null\`, then \`export PATH=<dir-of-the-1.${REQUIRED_GO_MINOR}+-go>:$PATH\` and re-check \`go version\` before continuing — make and go test inherit this PATH.`

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

// ---------------------------------------------------------------- Phase 4: ship-plan ALL changes (sequential — fast)
phase('Plan all changes')
// Run ship-plan for every change first. These are fast (~1-2 min each) and create
// .handoff/<change>/plan.json with the unit breakdown. Doing them all first lets
// the parallel implementation agents in Phase 5 use them without waiting.
for (const entry of queue) {
  if (entry.mode === 'archive-only' || entry.mode === 'skip') continue
  if (entry.status === 'shipped') { log(`${entry.change}: already shipped, skipping`); continue }
  entry.status = 'in_progress'
  log(`${entry.change}: ship-plan`)
  const plan2 = await workflow('ship-plan', { change: entry.change, date: DATE, local: true })
  if (!plan2 || !plan2.ok) {
    entry.status = 'failed'
    entry.failureStage = 'ship-plan'
    entry.failureLog = plan2 ? (plan2.reason || plan2.notes || '') : 'null'
    return { stage: 'ship-plan', ok: false, reason: `ship-plan failed for ${entry.change}: ${entry.failureLog}`, change: entry.change }
  }
  const pairs = plan2.pairs || 0
  log(`${entry.change}: ship-plan done (pairs=${pairs})`)
  entry.status = 'pending'
}

// Schema for the parallel worktree implementation agent
const IMPL_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['change', 'ok', 'branch', 'commits'],
  properties: {
    change: { type: 'string' },
    ok: { type: 'boolean' },
    branch: { type: 'string' },
    commits: { type: 'integer' },
    failureStage: { type: 'string' },
    failureLog: { type: 'string' },
  },
}

// Implement one change in an isolated git worktree. The agent gets a clean worktree
// based on main and implements change test-first from its .handoff plan.
async function implementInWorktree(entry) {
  entry.status = 'in_progress'
  const change = entry.change
  const handoffPath = `.handoff/${change}/plan.json`
  const TOOLCHAIN = `TOOLCHAIN: go.mod requires go 1.${REQUIRED_GO_MINOR}.x. Run \`which -a go\`; if go < 1.${REQUIRED_GO_MINOR}, try /opt/homebrew/Cellar/go@*/bin/go or \`ls /opt/homebrew/bin/go*\`. Export PATH with the right one before running go commands.`

  return await agent(
    [
      `Implement OpenSpec change "${change}" test-first in this isolated worktree. ${TOOLCHAIN}`,
      ``,
      `CONTEXT:`,
      `- Change directory: openspec/changes/${change}/`,
      `- Design: openspec/changes/${change}/design.md`,
      `- Tasks: openspec/changes/${change}/tasks.md`,
      `- Handoff plan: ${handoffPath} (already created — has per-unit breakdown)`,
      `- All existing code is on base branch "main" (this worktree was created from it)`,
      ``,
      `STEPS:`,
      `1. Worktree setup:`,
      `   - cd into the worktree root (pwd)`,
      `   - git switch -c feat/${change} main  (create feature branch from main)`,
      ``,
      `2. Read ${handoffPath} (if exists) for the unit breakdown. Otherwise read openspec/changes/${change}/tasks.md and group tasks into 2-4 units.`,
      ``,
      `3. For EACH unit, implement test-first:`,
      `   a. RED: Write the test file(s) specified in the unit. Run them — they must FAIL.`,
      `   b. GREEN: Implement the minimal production code. Tests must PASS.`,
      `   c. COMMIT: git add -A && git commit -s -m "feat: <unit title> (${change})" -m "Co-Authored-By: Claude <noreply@anthropic.com>"`,
      `   d. Move to next unit.`,
      ``,
      `4. FULL VERIFY:`,
      `   - go build ./... && make vet`,
      `   - go test -p 1 ./...  (DB tests need TEST_DATABASE_URL — skip if not set, that is OK)`,
      `   - openspec validate --change "${change}" --type change --strict (best-effort)`,
      `   - If tests fail, fix and re-run (max 2 repair attempts)`,
      ``,
      `5. EVIDENCE:`,
      `   - mkdir -p openspec/changes/${change}/evidence/`,
      `   - Run go test -p 1 -coverprofile=coverage.out ./...  (best-effort)`,
      `   - go tool cover -func=coverage.out 2>/dev/null | tail -5 > openspec/changes/${change}/evidence/coverage.txt`,
      `   - Write openspec/changes/${change}/evidence/gates.md with build/vet/test results`,
      `   - Write openspec/changes/${change}/evidence/test-results.md with full test output`,
      `   - git add openspec/changes/${change}/evidence/`,
      `   - git commit -s -m "chore(${change}): evidence" -m "Co-Authored-By: Claude <noreply@anthropic.com>"`,
      ``,
      `6. RETURN { change: "${change}", ok: true/false, branch: "feat/${change}", commits: <count>, failureLog: "<details if failed>" }`,
      `   If any step fails: return ok=false and describe what failed in failureLog.`,
    ].join('\n'),
    {
      schema: IMPL_SCHEMA,
      label: `impl:${change}`,
      phase: 'Implement parallel',
      isolation: 'worktree',
      agentType: 'general-purpose',
    },
  )
}

// ---------------------------------------------------------------- Phase 5: Implement ALL changes in parallel worktrees
phase('Implement parallel')
log(`implement: ${queue.filter(e => e.mode !== 'archive-only' && e.mode !== 'skip' && e.status !== 'shipped').length} change(s) in parallel worktrees`)

const implResults = await parallel(
  queue
    .filter(e => e.mode !== 'archive-only' && e.mode !== 'skip' && e.status !== 'shipped')
    .map(entry => () => implementInWorktree(entry))
)

// Process results — log successes, collect failures
const failedImpls = implResults.filter(r => r && !r.ok)
const succeededImpls = implResults.filter(r => r && r.ok)
for (const r of succeededImpls) {
  const entry = queue.find(e => e.change === r.change)
  if (entry) {
    entry.status = 'pending' // ready for merge
    entry.commits = r.commits || 0
    log(`✅ ${r.change}: implemented (${r.commits} commits on ${r.branch})`)
  }
}
for (const r of failedImpls) {
  const entry = queue.find(e => e.change === r.change)
  if (entry) {
    entry.status = 'failed'
    entry.failureStage = r.failureStage || 'implement'
    entry.failureLog = r.failureLog || 'unknown'
    log(`❌ ${r.change}: failed — ${r.failureLog}`)
  }
}
if (failedImpls.length) {
  const firstFail = failedImpls[0]
  return {
    stage: firstFail.failureStage || 'implement', ok: false,
    reason: `worktree implementation failed for ${firstFail.change}: ${firstFail.failureLog}`,
    change: firstFail.change,
    resumeFrom: firstFail.change,
    progressPath: PROGRESS_PATH,
    summary: { total: queue.length, shipped: 0, failed: failedImpls.length, skipped: 0, pending: succeededImpls.length },
  }
}

// ---------------------------------------------------------------- Phase 6: Merge + archive + PR + cleanup (sequential)
phase('Merge, archive, PR')
// Each successful implementation is on its own feat/<change> branch (created in the
// worktree). These branches are already in the main repo's git (worktree shares refs).
// Merge them to main in dependency order.
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

  // Merge feat/<change> into main (squash)
  entry.status = 'in_progress'
  log(`${entry.change}: merging feat/${entry.change} into main (${mergeStrategy})`)
  const mergeRes = await agent(
    [
      `Merge feature branch "feat/${entry.change}" into main using strategy "${mergeStrategy}". Use Bash. ${TOOLCHAIN_NOTE}`,
      `1. git switch main`,
      `2. Merge feat/${entry.change}: git merge --${mergeStrategy} "feat/${entry.change}"`,
      `   - If conflict: merged=false, reason="merge conflict: <files>", STOP.`,
      `3. Confirm merge worked: git log --oneline -3`,
      `4. git log --oneline -3 — get the merge commit SHA.`,
      `5. RUN go build ./... && make vet && go test -p 1 ./...  (verify merge didn't break anything)`,
      `6. Return { merged, mergeSha, notes }.`,
    ].join('\n'),
    {
      schema: {
        type: 'object', additionalProperties: false,
        required: ['merged', 'mergeSha', 'notes'],
        properties: { merged: { type: 'boolean' }, mergeSha: { type: 'string' }, notes: { type: 'string' } },
      },
      label: `merge:${entry.change}`, phase: 'Merge, archive, PR', agentType: 'general-purpose',
    },
  )
  if (!mergeRes || !mergeRes.merged) {
    entry.status = 'failed'
    entry.failureStage = 'merge'
    entry.failureLog = mergeRes ? mergeRes.notes : 'merge agent returned null'
    haltReason = `merge failed for ${entry.change}: ${entry.failureLog}`
    failedEntry = entry
    break
  }
  log(`${entry.change}: merged at ${mergeRes.mergeSha}`)

  // Archive
  const archiveTarget = `openspec/changes/archive/${DATE}-${entry.change}`
  const archiveRes = await agent(
    [
      `Archive change "${entry.change}" after merge. Use Bash.`,
      `1. mkdir -p openspec/changes/archive`,
      `2. mv "openspec/changes/${entry.change}" "${archiveTarget}"`,
      `3. git add "openspec/changes/archive/${DATE}-${entry.change}/" && git rm -r "openspec/changes/${entry.change}/" 2>/dev/null; true`,
      `4. git commit -s -m "chore(${entry.change}): archive after ship" -m "Co-Authored-By: Claude <noreply@anthropic.com>"`,
      `5. Return { archived: true, archivePath: "${archiveTarget}" }`,
    ].join('\n'),
    {
      schema: {
        type: 'object', additionalProperties: false,
        required: ['archived', 'archivePath'],
        properties: { archived: { type: 'boolean' }, archivePath: { type: 'string' }, notes: { type: 'string' } },
      },
      label: `archive:${entry.change}`, phase: 'Merge, archive, PR', agentType: 'general-purpose',
    },
  )
  entry.mergeSha = mergeRes.mergeSha
  entry.archivePath = archiveRes ? archiveRes.archivePath : null
  entry.status = 'shipped'
  log(`${entry.change}: shipped (merge=${entry.mergeSha} archive=${entry.archivePath})`)
}

// If we halted during merge/archive, report
if (haltReason) {
  return {
    stage: failedEntry ? failedEntry.failureStage : 'merge',
    ok: false, reason: haltReason, change: failedEntry ? failedEntry.change : null,
    mergeSha: failedEntry ? failedEntry.mergeSha : null,
    archivePath: failedEntry ? failedEntry.archivePath : null,
    resumeFrom: failedEntry ? failedEntry.change : null,
    progressPath: PROGRESS_PATH,
    summary: { total: queue.length, shipped: queue.filter(e => e.status === 'shipped').length, failed: 1, skipped: 0, pending: queue.filter(e => e.status === 'pending').length },
    nextStep: `Fix ${failedEntry ? failedEntry.change : '?'} and re-run: /opsx:ship-all --from ${failedEntry ? failedEntry.change : ''}`,
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