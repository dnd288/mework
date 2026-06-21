export const meta = {
  name: 'ship-code',
  description:
    'Execute a ship-plan handoff for an APPROVED OpenSpec change, unit-by-unit, test-first. Preflight (tools+toolchain check, validate, clean tree, branch, load .handoff/<change>/plan.json — a FEW test-first units) → for EACH unit run Red (write the unit\'s failing test(s), confirm they fail) then Green (implement the minimal code across the unit\'s files, confirm they pass, tick every tasks.md item the unit covers) and make ONE commit for the unit → full Verify (make vet/test + coverage + openspec validate, repair loop; every gate re-activates a go >=1.25 toolchain) → Evidence (openspec/changes/<name>/evidence/). Then it forks based on args.local: the REMOTE path syncs delta specs → prepends a CHANGELOG entry → a final chore commit → push + open/update a PR (stops at PR opened, no auto-merge). The LOCAL path (args.local=true) instead runs Local review (code-review-and-quality + security-and-hardening audit of the diff vs base) → merges feat/<change> into <base> locally (squash/no-ff/ff-only) → re-runs verify on base → sync delta specs → archives the change to openspec/changes/archive/YYYY-MM-DD-<change>/ → optional semver tag → (with args.openPr) push feat/<change> + open a PR for review → chore commit + local branch delete + optional git push origin <base> (noPushMain=true by default). When all units are already done on the local path it skips Implement and still verifies/merges/archives (clean resume). Honors dryRun, only:<unit>, retryBlocked, a token budget reserve, mergeStrategy, bump, noPushMain, archive, skipReview, openPr, and base.',
  phases: [
    { title: 'Preflight',           detail: 'tools+toolchain, validate, branch, load handoff (--local checks base + branch slug match)' },
    { title: 'Implement',           detail: 'per unit: Red → Green → one commit (a FEW units, not one per task)' },
    { title: 'Verify',              detail: 'make vet/test + coverage + openspec validate, repair loop' },
    { title: 'Local review',        detail: '(--local) code-review-and-quality + security-and-hardening audit of diff vs base' },
    { title: 'Evidence',            detail: 'write test results, coverage, gates to evidence/' },
    { title: 'Merge',               detail: '(--local) git switch <base> && git merge --{squash,no-ff,ff-only} feat/<change>' },
    { title: 'Post-merge verify',   detail: '(--local) re-run gates on <base> post-merge; halt on failure' },
    { title: 'Sync',                detail: 'merge delta specs into openspec/specs/' },
    { title: 'Archive',             detail: '(--local) mv openspec/changes/<c>/ → archive/YYYY-MM-DD-<c>/' },
    { title: 'Tag',                 detail: '(--local, --bump) optional git tag -a vX.Y.Z on main' },
    { title: 'Open PR',             detail: '(--local, --openPr) push feat/<change> + gh pr create for review' },
    { title: 'Cleanup',             detail: '(--local) chore commit + branch -D + optional push main + post-merge.md' },
    { title: 'Changelog',           detail: 'prepend a Keep a Changelog entry' },
    { title: 'Finalize',            detail: 'remote: chore commit + push + gh pr create (skipped on dryRun); --local: ship report' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const date = A.date
const dryRun = !!A.dryRun
const onlyPair = A.only ? String(A.only) : null
const retryBlocked = !!A.retryBlocked
const reserve = A.reserveTokens || 60000
const maxRepairs = typeof A.maxRepairs === 'number' ? A.maxRepairs : 2
const REQUIRED_GO_MINOR = 25
// Toolchain note injected into EVERY phase that runs go/make (Red, Green, Verify).
// Each agent runs in a fresh shell, so the PATH that preflight resolved does NOT
// carry over — every gate must re-activate a go >= 1.25 itself or it will hit a
// stale go (e.g. /usr/local/go) and fail with "invalid go version".
const TOOLCHAIN_NOTE = `TOOLCHAIN (do this FIRST, before any go/make/go test command): go.mod requires go 1.${REQUIRED_GO_MINOR}.x. Run \`go version\`; if it is older than 1.${REQUIRED_GO_MINOR} (a stale go on PATH such as /usr/local/go can shadow a newer one), locate a newer toolchain via \`which -a go\` and \`ls /opt/homebrew/bin/go /opt/homebrew/Cellar/go*/*/bin/go 2>/dev/null\`, then \`export PATH=<dir-of-the-1.${REQUIRED_GO_MINOR}+-go>:$PATH\` and re-check \`go version\` before continuing — make and go test inherit this PATH.`
// --local: fully-local ship path (no gh, no remote push unless --push-main)
const local = A.local === true
const base = A.base || 'main'
const mergeStrategy = ['squash', 'no-ff', 'ff-only'].includes(A.mergeStrategy) ? A.mergeStrategy : 'squash'
const bump = ['patch', 'minor', 'major'].includes(A.bump) ? A.bump : null
const noPushMain = A.noPushMain !== false // default true
const archive = A.archive !== false // default true
const skipReview = !!A.skipReview
const keepBranch = !!A.keepBranch
// --openPr (LOCAL path): after the local merge, also push the feature branch and open a
// PR for the record/human review. Forces the local branch to be kept on origin.
const openPr = A.openPr === true

if (!change || typeof change !== 'string') {
  throw new Error('ship-code requires args { change, date, dryRun?, only?, retryBlocked?, reserveTokens?, local?, base?, mergeStrategy?, bump?, noPushMain?, archive?, skipReview?, keepBranch?, openPr? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z0-9][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (expected kebab-case slug): ' + change)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)
if (onlyPair && !/^[0-9]{1,3}$/.test(onlyPair)) throw new Error('Unsafe only (expected a pair ordinal like "02"): ' + onlyPair)
if (local && !/^[A-Za-z0-9._/-]+$/.test(base)) throw new Error('Unsafe base branch: ' + base)
const DATE = date || 'Unreleased'
const branch = `feat/${change}`
const handoffDir = `.handoff/${change}`

// ---------------------------------------------------------------- skills wiring
const SKILL_DIR = '.claude/skills'
const SKILLS = {
  Test: ['test-driven-development'],
  Implement: ['incremental-implementation', 'code-simplification'],
  Verify: ['debugging-and-error-recovery', 'code-review-and-quality', 'security-and-hardening'],
  Review: ['code-review-and-quality', 'security-and-hardening'],
  PR: ['git-workflow-and-versioning', 'documentation-and-adrs'],
  LocalMerge: ['git-workflow-and-versioning', 'code-review-and-quality'],
  Archive: ['git-workflow-and-versioning', 'documentation-and-adrs'],
}
function skillNote(p) {
  const list = SKILLS[p] || []
  return list.length ? `Consult these skills before acting (read each, apply its rules): ${list.map((n) => `${SKILL_DIR}/${n}/SKILL.md`).join(', ')}.` : ''
}
const allSkills = Array.from(new Set(Object.values(SKILLS).flat()))

// ---------------------------------------------------------------- schemas
const TASKREF = {
  type: 'object', additionalProperties: false, required: ['id', 'role', 'status', 'file', 'deliverables', 'verify'],
  properties: {
    id: { type: 'string' }, role: { type: 'string', enum: ['test', 'code'] },
    status: { type: 'string' }, file: { type: 'string', description: 'handoff unit-file path' },
    deliverables: { type: 'array', items: { type: 'string' }, description: 'repo-relative files this side of the unit writes (may be several)' },
    verify: { type: 'string' },
    skipRed: { type: 'boolean' },
  },
}
const PREFLIGHT = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'toolchainOk', 'branchReady', 'changeRoot', 'proposalPath', 'tasksPath', 'specPaths', 'pairs'],
  properties: {
    ok: { type: 'boolean' }, reason: { type: 'string' },
    toolchainOk: { type: 'boolean' }, branchReady: { type: 'boolean' },
    changeRoot: { type: 'string' }, proposalPath: { type: ['string', 'null'] },
    tasksPath: { type: 'string' }, specPaths: { type: 'array', items: { type: 'string' } },
    title: { type: 'string' },
    pairs: {
      type: 'array', description: 'the change as a FEW test-first units, in dependency order (one entry per plan.json unit)',
      items: {
        type: 'object', additionalProperties: false, required: ['pair', 'title', 'test', 'code', 'allDone'],
        properties: {
          pair: { type: 'string', description: 'unit id, e.g. "01"' }, title: { type: 'string' },
          coversTasks: { type: 'array', items: { type: 'string' }, description: 'tasks.md ordinals this unit realizes' },
          test: TASKREF, code: TASKREF,
          allDone: { type: 'boolean', description: 'unit already done (skip unless retryBlocked/only)' },
        },
      },
    },
  },
}
const RED = {
  type: 'object', additionalProperties: false, required: ['redConfirmed', 'skipRed', 'skipReason', 'testFile', 'failureLog'],
  properties: {
    redConfirmed: { type: 'boolean', description: 'the new test was RUN and FAILED' },
    skipRed: { type: 'boolean' }, skipReason: { type: 'string' },
    testFile: { type: ['string', 'null'] }, failureLog: { type: 'string' },
  },
}
const GREEN = {
  type: 'object', additionalProperties: false, required: ['greenConfirmed', 'codeFile', 'taskTicked', 'committed', 'sha', 'failureLog'],
  properties: {
    greenConfirmed: { type: 'boolean', description: 'the test passes after the implementation' },
    codeFile: { type: ['string', 'null'] }, taskTicked: { type: 'boolean' },
    committed: { type: 'boolean' }, sha: { type: ['string', 'null'] }, failureLog: { type: 'string' },
  },
}
const VERDICT = {
  type: 'object', additionalProperties: false, required: ['pass', 'gatesRun', 'coverage', 'failureLog'],
  properties: {
    pass: { type: 'boolean' }, gatesRun: { type: 'array', items: { type: 'string' } },
    coverage: { type: 'string' }, failureLog: { type: 'string' },
  },
}
const REPAIR = { type: 'object', additionalProperties: false, required: ['fixed', 'notes'], properties: { fixed: { type: 'boolean' }, notes: { type: 'string' } } }
const EVIDENCE = { type: 'object', additionalProperties: false, required: ['written', 'evidenceDir', 'files', 'notes'], properties: { written: { type: 'boolean' }, evidenceDir: { type: 'string' }, files: { type: 'array', items: { type: 'string' } }, notes: { type: 'string' } } }
const SYNCED = { type: 'object', additionalProperties: false, required: ['synced', 'notes'], properties: { synced: { type: 'boolean' }, notes: { type: 'string' } } }
const FINALIZE = {
  type: 'object', additionalProperties: false, required: ['changelogWritten', 'choreCommitted', 'pushed', 'prUrl', 'prExisted', 'notes'],
  properties: {
    changelogWritten: { type: 'boolean' }, choreCommitted: { type: 'boolean' },
    pushed: { type: 'boolean' }, prUrl: { type: ['string', 'null'] },
    prExisted: { type: 'boolean' }, notes: { type: 'string' },
  },
}
// --- Local-path schemas (only used when args.local=true) ---
const REVIEW = {
  type: 'object', additionalProperties: false, required: ['verdict', 'findings', 'axes', 'diffStat', 'notes'],
  properties: {
    verdict: { type: 'string', enum: ['pass', 'fail'] },
    findings: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false, required: ['severity', 'axis', 'location', 'problem', 'suggestion'],
        properties: {
          severity: { type: 'string', enum: ['blocker', 'required', 'nit', 'fyi'] },
          axis: { type: 'string', enum: ['correctness', 'readability', 'architecture', 'security', 'performance'] },
          location: { type: 'string', description: 'file:line or repo-relative path' },
          problem: { type: 'string' }, suggestion: { type: 'string' },
        },
      },
    },
    axes: { type: 'array', items: { type: 'string' }, description: 'axes actually exercised' },
    diffStat: { type: 'string', description: 'raw `git diff <base>..<branch> --stat` output' },
    notes: { type: 'string' },
  },
}
const MERGE = {
  type: 'object', additionalProperties: false, required: ['merged', 'strategy', 'baseSha', 'mergeSha', 'mergeMessage', 'conflicts', 'notes'],
  properties: {
    merged: { type: 'boolean' },
    strategy: { type: 'string', enum: ['squash', 'no-ff', 'ff-only'] },
    baseSha: { type: 'string' },
    mergeSha: { type: ['string', 'null'] },
    mergeMessage: { type: ['string', 'null'] },
    conflicts: { type: 'boolean', description: 'true only if conflicts required manual resolution (always false; we refuse to auto-resolve)' },
    notes: { type: 'string' },
  },
}
const ARCHIVED = {
  type: 'object', additionalProperties: false, required: ['archived', 'archivePath', 'mergeSha', 'reason'],
  properties: {
    archived: { type: 'boolean' },
    archivePath: { type: ['string', 'null'] },
    mergeSha: { type: ['string', 'null'] },
    reason: { type: 'string' },
  },
}
const TAGGED = {
  type: 'object', additionalProperties: false, required: ['tagged', 'tag', 'priorTag', 'reason'],
  properties: {
    tagged: { type: 'boolean' },
    tag: { type: ['string', 'null'] },
    priorTag: { type: ['string', 'null'] },
    reason: { type: 'string' },
  },
}
const FINALIZE_LOCAL = {
  type: 'object', additionalProperties: false, required: ['choreCommitted', 'choreSha', 'branchDeleted', 'pushed', 'pushReason', 'tag', 'archivePath', 'evidenceDir', 'notes'],
  properties: {
    choreCommitted: { type: 'boolean' },
    choreSha: { type: ['string', 'null'] },
    branchDeleted: { type: 'boolean' },
    pushed: { type: 'boolean' },
    pushReason: { type: 'string' },
    tag: { type: ['string', 'null'] },
    archivePath: { type: ['string', 'null'] },
    evidenceDir: { type: 'string' },
    notes: { type: 'string' },
  },
}

// ---------------------------------------------------------------- Phase 1: Preflight (load handoff)
phase('Preflight')
const pre = await agent(
  [
    `Preflight ship-code for OpenSpec change "${change}" on branch "${branch}"${local ? ' (LOCAL PATH — base="' + base + '", mergeStrategy=' + mergeStrategy + ')' : ''}. Use Bash. Steps:`,
    `1. TOOLCHAIN + TOOLS: command -v go openspec ; go version. go.mod requires go 1.${REQUIRED_GO_MINOR}.x. CRITICAL POLICY: do NOT set toolchainOk=false just because go < 1.${REQUIRED_GO_MINOR} is on PATH. First run \`go version\` then \`which -a go\` and look for go1.${REQUIRED_GO_MINOR}.* under /opt/homebrew/Cellar/go@*/bin/go or other PATH locations. If a newer toolchain exists anywhere in PATH, prefer it (export PATH=<dir>:$PATH and re-check).`,
    `   - If a newer toolchain is found and activated, set toolchainOk=true and capture the resolved goVersion.`,
    `   - If NO 1.${REQUIRED_GO_MINOR}+ toolchain can be located anywhere on the machine, set toolchainOk=false AND ok=false AND reason="go >= 1.${REQUIRED_GO_MINOR} not found on this machine (have: <list of versions found>); install with \`brew install go@1.${REQUIRED_GO_MINOR}\` (or equivalent) and re-run" AND STOP. Do NOT silently fall back to "toolchainOk=true" when nothing >= 1.${REQUIRED_GO_MINOR} exists — verify gates WILL fail with cryptic errors and the user will not understand why.`,
    `   - gh is OPTIONAL — only required when args.local is false (the local path never calls gh).`,
    `2. Load the handoff: read "${handoffDir}/plan.json". If it does not exist, set ok=false, reason="no handoff — run /opsx:ship-plan ${change} first" and STOP. It contains a "units" array (a FEW test-first work-units). Map EACH unit to one entry in the returned pairs array (one Red→Green→commit per unit), ordered by ascending id: pair=unit.id; title=unit.title; coversTasks=unit.coversTasks; allDone=(unit.status=="done"); file (for both test and code) = "${handoffDir}/tasks/<unit.id>-<unit.slug>.md"; test={id:unit.id, role:"test", status:unit.status, file, deliverables:unit.testDeliverables, verify:unit.verify, skipRed:unit.skipRed}; code={id:unit.id, role:"code", status:unit.status, file, deliverables:unit.codeDeliverables, verify:unit.verify}. (Legacy fallback: if plan.json has the old "tasks" array instead of "units", group tasks into pairs by their "pair" field and set deliverables=[task.deliverable].)`,
    `3. openspec status --change "${change}" --json — capture changeRoot, proposal/tasks paths, delta-spec paths, title. openspec list --json — capture isActive (change present in active list) and isArchived (change present in archive list).`,
    `4. openspec validate "${change}" --strict (fallback non-strict) — MUST pass else ok=false+reason+STOP.`,
    `5. WORKING-TREE HYGIENE: git status --porcelain. ${handoffDir}/ is gitignored and does not count. Two cases:`,
    `   a. The ONLY tracked changes are under .claude/ (workflow / command / skill dev in flight, unrelated to the change) → treePolicy="dirty-workflow-dev-only", warn but proceed.`,
    `   b. Any other tracked file is dirty → treePolicy="dirty-blocked", ok=false, reason="uncommitted tracked changes outside .claude/; commit or stash first", STOP.`,
    `6. Branch handling:`,
    local
      ? `   - LOCAL PATH: the working branch MUST be "${branch}" (a feat/<change> branch). If currently on "${base}" with no commits ahead, ok=false, reason="on <base> with no commits; create ${branch} and ship from there". If on any other branch, ok=false, reason="currently on <other>; switch to ${branch}". If the branch is named e.g. feat/cNNNN-wrong-slug but the change slug is ${change}, ok=false, reason="branch slug does not match change slug; rename with: git branch -m feat/${change}". If you are already on ${branch}, branchReady=true. Do NOT create the branch — it must exist (the implement phase already worked on it).`
      : `   - Create/checkout branch "${branch}" (git checkout -b "${branch}" or git checkout "${branch}"); confirm not on main; branchReady=true.`,
    local ? `7. For LOCAL PATH: also check the base branch exists (git rev-parse --verify ${base}); capture its sha as baseSha for the merge phase. If base does not exist, ok=false+reason+STOP.` : ``,
    `Return the structured result; do not implement anything here.`,
  ].filter(Boolean).join('\n'),
  { schema: PREFLIGHT, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.ok || !pre.branchReady) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', toolchainOk: pre ? pre.toolchainOk : false, change, branch }
}
const title = pre.title || change
const CONTEXT = [
  `Change "${change}" — "${title}". Ground every decision in: proposal ${pre.proposalPath || '(n/a)'}, tasks ${pre.tasksPath}, delta specs ${(pre.specPaths || []).join(', ') || '(none)'}.`,
].join('\n')

// select pairs to run (normalize ordinals on both sides so --only 2 matches "02"/"002")
let pairs = (pre.pairs || []).slice()
if (onlyPair) pairs = pairs.filter((p) => Number(p.pair) === Number(onlyPair))
if (onlyPair && !pairs.length) {
  return { stage: 'implement', ok: false, reason: `--only ${onlyPair} matched no pair in the handoff (have: ${(pre.pairs || []).map((p) => p.pair).join(', ') || 'none'})`, change, branch, commits: [] }
}
const runnable = pairs.filter((p) => !p.allDone || retryBlocked || onlyPair)
log(`preflight ok — ${pre.pairs.length} pair(s); running ${runnable.length}${dryRun ? ' (dryRun: local commits, no push/PR)' : ''}`)
if (!runnable.length && !local) {
  // Remote path: nothing to implement and no local merge to perform — stop here.
  return { stage: 'implement', ok: true, change, branch, commits: [], notes: 'all pairs already done — nothing to implement', nextStep: `All handoff pairs are marked done. Re-run with retryBlocked to force, or proceed to /opsx:archive ${change} after merge.` }
}
if (!runnable.length) {
  // LOCAL path: the change is fully implemented (e.g. a resumed change whose
  // per-pair commits already exist) but not yet merged/archived. Do NOT stop —
  // fall through with an empty Implement loop to Verify → review → merge → archive
  // so the already-implemented branch still ships.
  log('all pairs already done — skipping Implement; proceeding to Verify + local merge')
}

// ---------------------------------------------------------------- Phase 2: Implement (per pair: Red → Green → one commit)
phase('Implement')
const commits = []
let blocked = null
for (const p of runnable) {
  if (budget && budget.total && budget.remaining() < reserve) {
    log(`budget reserve reached — stopping before pair ${p.pair} (${runnable.length - commits.length} pair(s) left)`); break
  }
  const testFiles = (p.test.deliverables || []).join(', ') || '(none)'
  const codeFiles = (p.code.deliverables || []).join(', ') || '(none)'
  const covers = (p.coversTasks || []).join(', ') || p.pair
  log(`unit ${p.pair}: ${p.title} (covers tasks ${covers})`)

  // --- Red
  const red = await agent(
    [
      `Unit ${p.pair} of change "${change}" — the RED step. ${skillNote('Test')}`,
      CONTEXT,
      TOOLCHAIN_NOTE,
      `Read the unit file "${p.test.file}" (its "Test plan (Red)" section). Write ALL of this unit's test deliverables — ${testFiles} — as table-driven Go tests, with the assertions/table cases the unit specifies (drawn from the delta-spec scenarios).`,
      p.test.skipRed
        ? `This unit is marked skipRed (doc-only/non-testable). Set skipRed=true with the reason; do not fabricate a test.`
        : `Then run: go test ./<the package(s) of those files>/... — and CONFIRM IT FAILS (undefined symbols or failing assertions). Set redConfirmed=true ONLY after observing the failure. Put the failing output in failureLog.`,
      `Write ONLY the test deliverables in this step (no production code). Do NOT commit. Update the unit file's status frontmatter to reflect the Red step done and append a one-line "## Output log" note.`,
    ].join('\n'),
    { schema: RED, label: `red:${p.pair}`, phase: 'Implement', agentType: 'general-purpose' },
  )
  if (!red) { blocked = { pair: p.pair, why: 'red agent returned null' }; break }
  if (!red.skipRed && !red.redConfirmed) {
    blocked = { pair: p.pair, why: 'Red not confirmed — the new test(s) did not fail before implementation. ' + (red.failureLog || '') }; break
  }

  // --- Green + single commit (red+green together)
  const green = await agent(
    [
      `Unit ${p.pair} of change "${change}" — the GREEN step + commit. ${skillNote('Implement')}`,
      CONTEXT,
      TOOLCHAIN_NOTE,
      `Read the unit file "${p.code.file}" (its "Code plan (Green)" section). Make the MINIMAL production change across this unit's code deliverables — ${codeFiles} — to turn the failing test(s) from the Red step GREEN. Do not over-build.`,
      red.skipRed ? `(No Red test — implement the doc/config change described.)` : `Run: go test ./<the package(s)>/... — they MUST pass. Iterate up to ${maxRepairs} times if needed (fix production code, not the tests). If still failing, set greenConfirmed=false and put the output in failureLog (do not commit).`,
      `Tick EVERY OpenSpec task this unit realizes in ${pre.tasksPath} ("- [ ]" → "- [x]" for change task(s) ${covers}); set taskTicked.`,
      `Update the unit file "${p.code.file}" status to "done" + a one-line Output log. ALSO set this unit's "status" to "done" in ${handoffDir}/plan.json (find the units[] entry with id "${p.pair}") so a later resume's preflight sees it done and skips re-implementing it.`,
      `THEN make ONE commit containing the whole unit (all tests + all implementation):`,
      `  git add -A  (note ${handoffDir}/ is gitignored and won't be staged)`,
      `  git commit -m "feat: ${p.title} (${change} unit ${p.pair})" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
      `Set committed=true and sha=<short hash>. If greenConfirmed is false, do NOT commit (committed=false).`,
    ].filter(Boolean).join('\n'),
    { schema: GREEN, label: `green:${p.pair}`, phase: 'Implement', agentType: 'general-purpose' },
  )
  if (!green || !green.greenConfirmed || !green.committed) {
    blocked = { pair: p.pair, why: green ? ('Green/commit failed: ' + (green.failureLog || 'no commit')) : 'green agent returned null' }; break
  }
  commits.push({ pair: p.pair, title: p.title, sha: green.sha })
  log(`pair ${p.pair}: committed ${green.sha} (red+green)`)
}
if (blocked) {
  return { stage: 'implement', ok: false, reason: `pair ${blocked.pair} blocked — stopping before PR. ${blocked.why}`, change, branch, commits, blockedPair: blocked.pair }
}
log(`implement: ${commits.length} per-task commit(s) made`)

// ---------------------------------------------------------------- Phase 3: Verify (deterministic-first + repair loop)
phase('Verify')
const coverProfile = `/tmp/shipcode-${change}.cover`
function verifyPrompt() {
  return [
    `Verify the full tree on branch "${branch}" for change "${change}". DETERMINISTIC gate — pass is exit-code-driven. Use Bash, run in order:`,
    TOOLCHAIN_NOTE,
    `1. go build ./...   2. make vet   3. go test -p 1 -coverprofile=${coverProfile} ./...  (DB tests skip without TEST_DATABASE_URL — not a failure)`,
    `4. go tool cover -func=${coverProfile} | tail -1 → coverage.   5. openspec validate "${change}" --strict (fallback non-strict).`,
    `pass=true only if every gate that ran exited 0 and all tests are green. List gates in gatesRun. On failure, pass=false + first failing gate's trimmed output in failureLog. Do not edit files.`,
  ].join('\n')
}
let verdict = await agent(verifyPrompt(), { schema: VERDICT, label: 'verify', phase: 'Verify', agentType: 'general-purpose' })
let repairs = 0
while (verdict && !verdict.pass && repairs < maxRepairs) {
  if (budget && budget.total && budget.remaining() < reserve) { log('budget reserve reached during repair'); break }
  repairs++
  log(`verify failed — repair ${repairs}/${maxRepairs}`)
  const repaired = await agent(
    [
      `The full verify gate failed for change "${change}". Make the SMALLEST in-scope fix. ${skillNote('Verify')}`,
      `Failing output:\n${verdict.failureLog}`,
      `Prefer fixing production code over weakening tests. Then amend it into the most relevant per-task commit (git add -A && git commit --amend --no-edit) OR a new fixup commit if it spans pairs. Do not push. If out of scope, set fixed=false.`,
    ].join('\n'),
    { schema: REPAIR, label: `repair:${repairs}`, phase: 'Verify', agentType: 'general-purpose' },
  )
  if (!repaired || !repaired.fixed) { log(`repair ${repairs} did not fix it: ${repaired ? repaired.notes : 'null'}`); break }
  verdict = await agent(verifyPrompt(), { schema: VERDICT, label: `verify:retry${repairs}`, phase: 'Verify', agentType: 'general-purpose' })
}
const gatesRun = (verdict && verdict.gatesRun) || []
const coverage = (verdict && verdict.coverage) || ''
if (!verdict || !verdict.pass) {
  return { stage: 'verify', ok: false, reason: 'verification did not pass — stopping before PR', failureLog: verdict ? verdict.failureLog : 'verify agent returned null', repairs, gatesRun, change, branch, commits }
}
log(`verify passed (${repairs} repair(s)); ${coverage || 'coverage n/a'}; gates: ${gatesRun.join(' | ')}`)

// ---------------------------------------------------------------- Phase 3b: Local review (LOCAL PATH ONLY — code-review-and-quality + security-and-hardening)
let review = null
if (local) {
  phase('Local review')
  if (skipReview) {
    log('local review skipped via --skipReview')
    review = { verdict: 'pass', findings: [], axes: [], diffStat: '', notes: 'skipped via --skipReview' }
    // Write a tiny evidence note so the audit trail is intact
    // (the Cleanup phase will stage this into the chore commit)
  } else {
    review = await agent(
      [
        `Local review of change "${change}" on branch "${branch}" vs base "${base}". Read-only audit. ${skillNote('Review')}`,
        `1. Compute the diff: git diff "${base}..${branch}" --stat ; git diff "${base}..${branch}" -- . ':(exclude)openspec/' ':(exclude).handoff/' | head -1500.`,
        `2. Categorize findings by severity: blocker / required / nit / fyi. Axes: correctness / readability / architecture / security / performance. For each finding give file:line, problem, suggestion.`,
        `3. BLOCKER criteria — any of: correctness bug, security issue, breaks a CLAUDE.md invariant (stdin-not-argv, row-locks, sealed creds, 0600/0700 perms, provider-agnostic schema, one active job per runtime, self-retrigger guard, de-dup on UNIQUE constraint), spec contradicts implementation, or test does not actually assert the spec.`,
        `4. PASS = no blockers AND <= 2 required findings. Else FAIL.`,
        `5. Do NOT edit any file — return findings + verdict. Capture the raw diffStat output verbatim.`,
      ].join('\n'),
      { schema: REVIEW, label: 'local-review', phase: 'Local review', agentType: 'general-purpose' },
    )
    log(`local review: ${review ? review.verdict : 'null'} (${review ? review.findings.length : 0} findings)`)
    if (!review || review.verdict !== 'pass') {
      return {
        stage: 'review', ok: false,
        reason: `local review verdict=${review ? review.verdict : 'null'} — halting before merge. Fix locally on ${branch} and re-run; pairs already done are skipped.`,
        change, branch, base, commits, repairs, gatesRun, coverage,
        reviewVerdict: review ? review.verdict : 'fail',
        reviewFindings: review ? review.findings : [],
      }
    }
  }
}

// ---------------------------------------------------------------- Phase 4: Evidence
phase('Evidence')
const evidence = await agent(
  [
    `Write the evidence bundle for change "${change}" into "${pre.changeRoot}/evidence/" (create dir). Use Bash/Write. It moves to the archive with the change and is linked from the PR.`,
    `- gates.md — the gates that ran (${gatesRun.join('; ')}), coverage total (${coverage || 'n/a'}), the per-task commits (${commits.map((c) => c.pair + ':' + c.sha).join(', ')}), repair count (${repairs}), and the governing skills (${allSkills.join(', ')}).`,
    `- test-results.md — go test -p 1 ./... 2>&1 | tail -40 (note DB tests skip without TEST_DATABASE_URL).`,
    `- coverage.txt — go tool cover -func=${coverProfile} 2>/dev/null | tail -20 (else "coverage not captured").`,
    `Concise + factual. Do NOT commit (a later step commits evidence). Return the dir + file list.`,
  ].join('\n'),
  { schema: EVIDENCE, label: 'evidence', phase: 'Evidence', agentType: 'general-purpose' },
)
const evidenceDir = (evidence && evidence.evidenceDir) || `${pre.changeRoot}/evidence`
log(`evidence: ${evidence && evidence.written ? evidence.files.join(', ') : (evidence ? evidence.notes : 'failed')}`)

// ---------------------------------------------------------------- Phase 5: Sync delta specs
phase('Sync')
let synced = { synced: false, notes: 'no delta specs' }
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'finalize', ok: false, reason: 'budget reserve reached before sync/changelog/PR; per-task commits are on the branch (specs not yet synced)', change, branch, commits, gatesRun, coverage, dryRun }
}
if (pre.specPaths && pre.specPaths.length) {
  const s = await agent(
    [
      `Sync the delta specs for change "${change}" into the main specs: invoke Skill({ skill: "openspec-sync-specs" }) for change "${change}". Merges ADDED/MODIFIED/REMOVED/RENAMED into openspec/specs/<capability>/spec.md, idempotent.`,
      `Delta files: ${pre.specPaths.join(', ')}. Do NOT commit. Return whether it ran.`,
    ].join('\n'),
    { schema: SYNCED, label: 'sync-specs', phase: 'Sync', agentType: 'general-purpose' },
  )
  if (s) synced = s
}
log(`sync: ${synced.synced ? 'merged delta specs' : synced.notes}`)

// ---------------------------------------------------------------- Phase 5b: Merge (LOCAL PATH ONLY — git switch base && merge --squash/--no-ff/--ff-only)
let mergeResult = null
let postMergeVerdict = null
let archived = { archived: false, archivePath: null, mergeSha: null, reason: 'archive not requested' }
if (local) {
  phase('Merge')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'merge', ok: false, reason: 'budget reserve reached before merge', change, branch, base, commits, repairs, gatesRun, coverage }
  }
  mergeResult = await agent(
    [
      `Local-merge branch "${branch}" into "${base}" for change "${change}". Use Bash ONLY (no gh, no remote). ${skillNote('LocalMerge')}`,
      `Strategy = "${mergeStrategy}".`,
      `1. git rev-parse --abbrev-ref HEAD — MUST equal "${branch}". Else merged=false, reason="not on ${branch}", STOP.`,
      `2. git rev-parse "${base}" — capture baseSha (the tip of base before merge).`,
      `3. Commit the change's own pre-merge artifacts on ${branch} so the tree is clean for the merge — the Evidence and Sync phases just wrote the evidence bundle (${pre.changeRoot}/evidence/), the synced delta specs (openspec/specs/), and the ticked tasks (${pre.tasksPath}) WITHOUT committing. Stage exactly those paths (NEVER git add -A): \`git add "${pre.changeRoot}" "openspec/specs" "${pre.tasksPath}"\`. If \`git diff --cached --quiet\` shows staged changes, commit them: \`git commit -s -m "chore(${change}): evidence + synced delta specs" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"\`. THEN run git status --porcelain: if the tree is STILL dirty with any file OUTSIDE {${pre.changeRoot}, openspec/specs, ${pre.tasksPath}, .claude/, .handoff/}, merged=false, reason="unexpected dirty tree on ${branch}: <files>; commit/stash first", STOP.`,
      `4. git switch "${base}". If the switch fails because ${base} has uncommitted tracked changes, merged=false, reason="<base> is dirty; commit/stash first" — NEVER auto-stash.`,
      `5. Apply the merge:`,
      `   - squash:    git merge --squash "${branch}"  (stages changes; does NOT commit yet)`,
      `                 then git commit -s -m "<conventional message>" -m "<body>" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
      `   - no-ff:     git merge --no-ff "${branch}" -m "<conventional message>" -m "<body>" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
      `   - ff-only:   git merge --ff-only "${branch}"  (will fail if not fast-forwardable — surface that error verbatim)`,
      `6. Build the conventional commit message:`,
      `     <type>(<scope>): <title>`,
      `     where type ∈ feat|fix (default feat), scope = "${change}".replace(/^c[0-9]+-/, ''), title = the title from proposal.md (single line, sentence case, no trailing period).`,
      `     Body bullets: "- OpenSpec change: ${change}" then a 2-4 line summary from proposal.md's "What" / "Why" section (read it from ${pre.proposalPath || '(see proposal.md)'}).`,
      `     Trailer: Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>.`,
      `7. Capture: mergeSha = git rev-parse HEAD ; mergeMessage = git log -1 --format='%H%n%s%n%b' HEAD.`,
      `8. Verify: git merge-base --is-ancestor "${branch}" "${base}" must succeed (the branch tip is now an ancestor of base).`,
      `9. CRITICAL: NEVER use git add -A in this phase. NEVER bypass hooks for feat/fix commits. NEVER auto-resolve conflicts — if the merge conflicts, set merged=false, reason="<conflicting files>", STOP.`,
      `Return the structured result.`,
    ].join('\n'),
    { schema: MERGE, label: 'merge', phase: 'Merge', agentType: 'general-purpose' },
  )
  if (!mergeResult || !mergeResult.merged) {
    return {
      stage: 'merge', ok: false,
      reason: mergeResult ? mergeResult.reason : 'merge agent returned null',
      change, branch, base, commits, repairs, gatesRun, coverage,
    }
  }
  log(`merge: ${mergeResult.strategy} → ${mergeResult.mergeSha}`)

  // Phase 5c: Post-merge verify — re-run gates on base post-merge
  phase('Post-merge verify')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'post-merge-verify', ok: false, reason: 'budget reserve reached before post-merge verify (merge is already committed locally)', change, branch, base, mergeSha: mergeResult.mergeSha, baseSha: mergeResult.baseSha }
  }
  const pmCover = `/tmp/shipcode-local-${change}.cover`
  const pmVerdict = await agent(
    [
      `Post-merge re-verify on "${base}" AFTER merging "${branch}" for change "${change}". DETERMINISTIC gate — pass is exit-code-driven. Use Bash, run in order:`,
      `1. git rev-parse --abbrev-ref HEAD — must equal "${base}". Else pass=false, reason="not on ${base}".`,
      `2. go build ./...   3. make vet   4. go test -p 1 -coverprofile=${pmCover} ./...  (DB tests skip without TEST_DATABASE_URL — not a failure)`,
      `5. go tool cover -func=${pmCover} | tail -1 → coverage.   6. openspec validate "${change}" --strict (fallback non-strict).`,
      `pass=true only if every gate that ran exited 0 and all tests are green. List gates in gatesRun. On failure, pass=false + first failing gate's trimmed output in failureLog. Do not edit files.`,
    ].join('\n'),
    { schema: VERDICT, label: 'post-merge-verify', phase: 'Post-merge verify', agentType: 'general-purpose' },
  )
  postMergeVerdict = pmVerdict
  if (!postMergeVerdict || !postMergeVerdict.pass) {
    return {
      stage: 'post-merge-verify', ok: false,
      reason: 'post-merge verify failed — the merge is committed locally to ' + base + '; fix and either amend or add a fix commit, then re-run /opsx:ship',
      change, branch, base,
      mergeSha: mergeResult.mergeSha, baseSha: mergeResult.baseSha,
      commits, repairs, gatesRun, coverage,
      postMergeGatesRun: postMergeVerdict ? postMergeVerdict.gatesRun : [],
      postMergeCoverage: postMergeVerdict ? postMergeVerdict.coverage : '',
      postMergeFailureLog: postMergeVerdict ? postMergeVerdict.failureLog : 'post-merge verify agent returned null',
    }
  }
  log(`post-merge verify passed on ${base}`)

  // Phase 5d: Archive — mv openspec/changes/<c>/ → openspec/changes/archive/YYYY-MM-DD-<c>/
  phase('Archive')
  if (archive) {
    archived = await agent(
      [
        `Archive OpenSpec change "${change}". ${skillNote('Archive')}`,
        `1. openspec list --json — confirm "${change}" is ACTIVE. If not active (already archived?), set archived=false, reason="already archived or not in active list — re-run is a no-op", STOP.`,
        `2. Compute targetDir = "openspec/changes/archive/${DATE}-${change}". If targetDir already exists, archived=false, reason="target exists: <path>", STOP.`,
        `3. mkdir -p openspec/changes/archive && mv openspec/changes/${change} "${targetDir}".`,
        `4. Create or append to openspec/changes/archive/INDEX.md (one row per archived change):`,
        `   | ${DATE} | ${change} | ${mergeResult.mergeSha} | <title from proposal.md> |`,
        `   (Use a markdown table with header row; create the file with the header row if it does not exist.)`,
        `5. Verify: openspec list --json — "${change}" must now NOT be active. openspec status --change "${change}" returns "not found" (that is success).`,
        `6. Do NOT commit — the Cleanup phase bundles a chore commit.`,
        `Return the structured result.`,
      ].join('\n'),
      { schema: ARCHIVED, label: 'archive', phase: 'Archive', agentType: 'general-purpose' },
    )
    log(`archive: ${archived.archived ? archived.archivePath : (archived.reason || 'failed')}`)
    if (!archived.archived) {
      return {
        stage: 'archive', ok: false,
        reason: archived.reason,
        change, branch, base, mergeSha: mergeResult.mergeSha, baseSha: mergeResult.baseSha,
        commits, repairs, gatesRun, coverage,
        postMergeGatesRun: postMergeVerdict ? postMergeVerdict.gatesRun : [],
        postMergeCoverage: postMergeVerdict ? postMergeVerdict.coverage : '',
      }
    }
  } else {
    archived = { archived: false, archivePath: null, mergeSha: mergeResult.mergeSha, reason: 'archive skipped via --no-archive' }
  }

  // Phase 5e: Tag (optional, --bump)
  phase('Tag')
  let tagged = { tagged: false, tag: null, priorTag: null, reason: 'no --bump' }
  if (bump) {
    tagged = await agent(
      [
        `Tag ${base} at HEAD with version vX.Y.Z derived from bump="${bump}". Use Bash. ${skillNote('Archive')}`,
        `1. If bump is null/empty → noop. tagged=false, tag=null.`,
        `2. Read the current latest tag: git describe --tags --abbrev=0  (returns empty + exit 128 if no tags). Capture priorTag.`,
        `3. Parse priorTag as MAJOR.MINOR.PATCH (default to 0.0.0 if no priorTag). Bump per bump:`,
        `   - patch: MAJOR.MINOR.(PATCH+1)`,
        `   - minor: MAJOR.(MINOR+1).0`,
        `   - major: (MAJOR+1).0.0`,
        `4. Compute newTag = "v${MAJOR}.${MINOR}.${PATCH}".`,
        `5. If newTag already exists (git tag -l newTag), tagged=false, reason="tag already exists", STOP.`,
        `6. git tag -a "${newTag}" -m "Release ${newTag}\n\nOpenSpec change: ${change}\nMerge: ${mergeResult.mergeSha}\nArchived: ${archived.archivePath || '(no-archive)'}\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
        `7. git tag -n3 "${newTag}" → confirm the tag was created with the expected message.`,
        `Return the structured result. Do NOT push the tag — that is the Cleanup phase's job (only if --push-main).`,
      ].join('\n'),
      { schema: TAGGED, label: 'tag', phase: 'Tag', agentType: 'general-purpose' },
    )
    log(`tag: ${tagged.tagged ? tagged.tag : (tagged.reason || 'failed')}`)
  }

  // Phase 5e2: Open PR (LOCAL path + --openPr) — push the feature branch and open a PR
  // for the record/human review. Runs BEFORE Cleanup deletes the LOCAL branch; the
  // pushed origin branch (and its PR) persists after the local delete. origin/${base}
  // is NOT updated (noPushMain), so a PR ${branch} → ${base} shows the full change.
  let prResult = { prCreated: false, prUrl: null, prReason: openPr ? 'pending' : 'openPr not requested' }
  if (openPr) {
    phase('Open PR')
    if (budget && budget.total && budget.remaining() < reserve) {
      prResult = { prCreated: false, prUrl: null, prReason: 'budget reserve reached before PR (merge already committed locally)' }
    } else {
      const evDir = archived.archived ? `${archived.archivePath}/evidence` : `${pre.changeRoot}/evidence`
      const pr = await agent(
        [
          `Open a PR for change "${change}" (title: "${title}"). The feature branch "${branch}" holds the change's per-unit commits and has been merged into LOCAL "${base}" (origin/${base} is NOT updated, so a PR ${branch} → ${base} shows the full change diff). Use Bash (git + gh). ${skillNote('PR')}`,
          `0. TARGET THE ORIGIN REPO, NOT AN UPSTREAM PARENT. This repo may be a fork — gh defaults PRs to the parent. Compute the origin slug: REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner) (or parse "git remote get-url origin"). Pass --repo "$REPO" to every gh pr command so the PR is opened on origin (e.g. minhlucncc/mework), never the upstream.`,
          `1. Push the branch: git push -u origin "${branch}". If origin is missing or the push fails, set prCreated=false, prReason=<error>, prUrl=null and STOP (NON-FATAL — the local merge already happened; just report it).`,
          `2. Reuse-or-create: gh pr view "${branch}" --repo "$REPO" --json url,state 2>/dev/null. If an OPEN PR already exists, reuse its url (prCreated=false, prReason="exists"). Otherwise create one:`,
          `   gh pr create --repo "$REPO" --base "${base}" --head "${branch}" --title "feat: ${title} (${change})" --body "<2-4 sentence summary drawn from ${pre.proposalPath || 'the proposal'}. Then: 'Local-merged into ${base} at ${mergeResult.mergeSha}; archived to ${archived.archivePath || '(no-archive)'}. Evidence: ${evDir}.' and a final line 'Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'>"`,
          `3. Capture the resulting PR url. Return { prCreated, prUrl, prReason }. Do NOT merge or close the PR, and do NOT delete the branch here.`,
        ].join('\n'),
        {
          schema: {
            type: 'object', additionalProperties: false, required: ['prCreated', 'prUrl', 'prReason'],
            properties: { prCreated: { type: 'boolean' }, prUrl: { type: ['string', 'null'] }, prReason: { type: 'string' } },
          },
          label: 'open-pr', phase: 'Open PR', agentType: 'general-purpose',
        },
      )
      prResult = pr || { prCreated: false, prUrl: null, prReason: 'open-pr agent returned null' }
      log(`open-pr: ${prResult.prUrl || prResult.prReason}`)
    }
  }

  // Phase 5f: Cleanup — chore commit (evidence+sync+archive+changelog), branch -D, optional push main, post-merge.md
  phase('Cleanup')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'cleanup', ok: false, reason: 'budget reserve reached before cleanup (merge is already committed locally; user completes chore commit manually)', change, branch, base, mergeSha: mergeResult.mergeSha, baseSha: mergeResult.baseSha, commits, repairs, gatesRun, coverage }
  }
  const evidenceDirArchived = archived.archived ? `${archived.archivePath}/evidence` : `${pre.changeRoot}/evidence`
  const fin = await agent(
    [
      `Finalize the LOCAL ship of change "${change}" on ${base}. Use Bash (git ONLY — NO gh). ${skillNote('Archive')}`,
      `Context:`,
      `- branch: ${branch} (still present, to be deleted)`,
      `- base: ${base}`,
      `- mergeSha: ${mergeResult.mergeSha}`,
      `- baseSha: ${mergeResult.baseSha}`,
      `- tag: ${tagged.tag || '(none)'}`,
      `- archivePath: ${archived.archivePath || '(no-archive)'}`,
      `- evidenceDir (where post-merge.md goes): ${evidenceDirArchived}`,
      `- commits: ${commits.length} per-task + merge commit + chore commit (this one)`,
      `- reviews: ${review ? review.verdict : 'n/a'} (${review ? review.findings.length : 0} findings)`,
      `- repairs: ${repairs}`,
      `- pre-merge gates: ${gatesRun.join(' | ')}`,
      `- pre-merge coverage: ${coverage || 'n/a'}`,
      `- post-merge gates: ${(postMergeVerdict && postMergeVerdict.gatesRun || []).join(' | ')}`,
      `- post-merge coverage: ${(postMergeVerdict && postMergeVerdict.coverage) || 'n/a'}`,
      ``,
      `Steps:`,
      `1. CHANGELOG.md: prepend bullet(s) under "## [Unreleased]" (create the file with the Keep a Changelog header if absent), grouped Added/Changed/Removed per the delta sections, each ending " (${change})". Date context: ${DATE}.`,
      `2. Stage ONLY these explicit paths (NEVER git add -A):`,
      `     ${evidenceDirArchived}`,
      `     CHANGELOG.md`,
      `     openspec/specs/`,
      `     openspec/changes/archive/`,
      `     ${tagged.tag ? 'NOTHING (tag is a ref, not a file); skip tag here' : ''}`,
      `   Use: git add <each-path>  (one at a time, in the order above).`,
      `3. ONE chore commit (use -s to sign off, do NOT bypass hooks):`,
      `     git commit -s -m "chore(${change}): evidence, sync, archive, changelog" \\`,
      `       -m "OpenSpec change: ${change}\\nMerge: ${mergeResult.mergeSha}\\nArchive: ${archived.archivePath || '(no-archive)'}${tagged.tag ? '\\nTag: ' + tagged.tag : ''}" \\`,
      `       -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
      `4. Write ${evidenceDirArchived}/post-merge.md (a NEW evidence file separate from gates.md) with this content (substitute values):`,
      `   # Post-merge report — ${change}`,
      `   | Item | Value |`,
      `   |------|-------|`,
      `   | Merged into | ${base} at ${mergeResult.mergeSha} |`,
      `   | Strategy | ${mergeResult.strategy} |`,
      `   | Post-merge verify | ${postMergeVerdict && postMergeVerdict.pass ? 'pass' : 'FAIL'} (gates: ${(postMergeVerdict && postMergeVerdict.gatesRun || []).join(', ')}) |`,
      `   | Delta specs synced | ${synced.synced ? 'yes' : 'no (' + synced.notes + ')'} |`,
      `   | Archived | ${archived.archivePath || '(no-archive)'} |`,
      `   | Tag | ${tagged.tag || 'n/a'} |`,
      `   | Chore commit | <shortSha from step 3> |`,
      `   | Skills applied | ${allSkills.join(', ')} |`,
      `   | Local review | ${review ? review.verdict : 'skipped'} (${review ? review.findings.length : 0} findings) |`,
      `   | Branch | ${branch} ${keepBranch ? 'kept' : 'deleted'} |`,
      `   Stage post-merge.md with the same chore commit (or amend it): git add ${evidenceDirArchived}/post-merge.md && git commit --amend --no-edit.`,
      `5. Cleanup branch: ${keepBranch ? 'KEEP' : 'git branch -D "' + branch + '"'}  (use -D because the merge already brought the content onto base; -d would fail).`,
      `6. ${noPushMain ? 'noPushMain=true — SKIP push. pushed=false, pushReason="fully local — noPushMain=true (use --push-main to push ' + base + ' to origin)".' : 'git push origin "' + base + '". If remote origin is missing or the push fails for any reason, pushed=false, pushReason=<error>. Captured choreSha + choreCommitted.'}`,
      `${tagged.tag && !noPushMain ? '7. Also push the tag: git push origin "' + tagged.tag + '".' : (tagged.tag ? '7. noPushMain=true — tag NOT pushed. (Re-run with --push-main to push the tag.)' : '')}`,
      `Return the structured result.`,
    ].join('\n'),
    { schema: FINALIZE_LOCAL, label: 'cleanup', phase: 'Cleanup', agentType: 'general-purpose' },
  )
  log(`cleanup: choreCommitted=${fin && fin.choreCommitted} branchDeleted=${fin && fin.branchDeleted} pushed=${fin && fin.pushed}`)

  // Local-path final report — skip the remote-PR Finalize phase
  return {
    stage: 'done', ok: true, mode: 'local',
    change, title, branch, base, local: true,
    mergeStrategy: mergeResult.strategy, mergeSha: mergeResult.mergeSha, baseSha: mergeResult.baseSha,
    commits, repairs, gatesRun, coverage,
    reviewVerdict: review ? review.verdict : 'skipped',
    reviewFindings: review ? review.findings : [],
    postMergeGatesRun: postMergeVerdict ? postMergeVerdict.gatesRun : [],
    postMergeCoverage: postMergeVerdict ? postMergeVerdict.coverage : '',
    specsSynced: synced.synced,
    archivePath: archived.archivePath,
    tag: tagged.tag,
    choreSha: fin ? fin.choreSha : null,
    pushed: !!(fin && fin.pushed),
    pushReason: fin ? fin.pushReason : '',
    prCreated: !!prResult.prCreated,
    prUrl: prResult.prUrl || null,
    prReason: prResult.prReason || '',
    evidenceDir: evidenceDirArchived,
    postMergeEvidence: `${evidenceDirArchived}/post-merge.md`,
    skillsApplied: allSkills,
    notes: fin ? fin.notes : 'cleanup agent returned null',
    nextStep: prResult.prUrl
      ? `Merged into ${base} locally and opened PR ${prResult.prUrl} for review. ${base} advanced by ${commits.length + 2} commit(s); ${change} archived to ${archived.archivePath || '(no-archive)'}. The PR shows the change diff; merging/pushing ${base} later closes it.`
      : (fin && fin.pushed)
        ? `Pushed ${base} (and tag ${tagged.tag || ''}) to origin. Verify on origin, then move to the next change.`
        : `Fully local ship complete${openPr ? ' (PR not opened: ' + prResult.prReason + ')' : ''}. ${base} advanced by ${commits.length + 2} commit(s); ${change} archived to ${archived.archivePath || '(no-archive)'}${tagged.tag ? '; tag ' + tagged.tag + ' created locally' : ''}. Inspect: git log --oneline -${commits.length + 3} ; cat ${evidenceDirArchived}/post-merge.md`,
  }
}

// ---------------------------------------------------------------- Phase 6+7: Changelog (+ chore commit) → PR (REMOTE PATH ONLY)
phase('Changelog')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'finalize', ok: false, reason: 'budget reserve reached before changelog/PR; per-task commits are on the branch', change, branch, commits, gatesRun, coverage, dryRun }
}
const fin = await agent(
  [
    `Finalize change "${change}" (title: "${title}") on branch "${branch}". Use Bash (git + gh). ${skillNote('PR')}`,
    `1. CHANGELOG.md: prepend bullet(s) under "## [Unreleased]" (create the file with the Keep a Changelog header if absent), grouped Added/Changed/Removed per the delta sections, each ending " (${change})". Date context: ${DATE}.`,
    `2. Commit the evidence + changelog${synced.synced ? ' + synced specs' : ''} as ONE chore commit. Stage ONLY the intended paths (do NOT use git add -A, to avoid committing a partial or unrelated change):`,
    `   git add "${evidenceDir}" CHANGELOG.md${synced.synced ? ' openspec/specs/' : ''} && git commit -m "chore(${change}): evidence, changelog${synced.synced ? ', spec sync' : ''}" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
    `   Set choreCommitted, changelogWritten.`,
    dryRun
      ? `3. DRY RUN: stop after the chore commit — do NOT run git push and do NOT run gh. Set pushed=false, prUrl=null, prExisted=false, note it was a dry run.`
      : `3. Push: git push -u origin "${branch}".`,
    dryRun ? `` : `4. Existing PR? gh pr view "${branch}" --json url,state 2>/dev/null. If OPEN, the push updated it (prExisted=true, use its url). Else gh pr create --base main --head "${branch}" --title "feat: ${title}" --body <body> with: the proposal's what-and-why; a "## Evidence" section linking ${evidenceDir} + the gates (${gatesRun.join(', ')}) + coverage (${coverage || 'n/a'}); the per-task commits (${commits.map((c) => 'task ' + c.pair).join(', ')}); the CHANGELOG bullet(s); "Skills applied: ${allSkills.join(', ')}"; and a final "🤖 Generated with [Claude Code](https://claude.com/claude-code)". Capture prUrl.`,
    `Return the structured result. If push/gh fails, set pushed=false/prUrl=null and explain in notes — commits stand on the local branch.`,
  ].filter(Boolean).join('\n'),
  { schema: FINALIZE, label: dryRun ? 'finalize (dry-run)' : 'finalize+pr', phase: 'PR', agentType: 'general-purpose' },
)

// ---------------------------------------------------------------- Report (remote path)
return {
  stage: 'done', ok: true, mode: 'remote', change, title, branch, dryRun,
  commits, // per-task red+green commits
  repairs, gatesRun, coverage, evidenceDir, skillsApplied: allSkills,
  specsSynced: synced.synced,
  changelogWritten: !!(fin && fin.changelogWritten),
  choreCommitted: !!(fin && fin.choreCommitted),
  pushed: !!(fin && fin.pushed),
  prExisted: !!(fin && fin.prExisted),
  prUrl: fin ? fin.prUrl : null,
  notes: fin ? fin.notes : 'finalize agent returned null',
  nextStep: dryRun
    ? `Dry run complete on ${branch}: ${commits.length} per-task commit(s) + chore commit. Inspect git log --stat + ${evidenceDir}, then re-run without dryRun to push + open the PR.`
    : `PR ${fin && fin.prExisted ? 'updated' : 'opened'}. Run /opsx:address-review for review feedback; after merge, /opsx:archive ${change}.`,
}
