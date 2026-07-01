export const meta = {
  name: 'ship-code',
  description:
    'Implement an OpenSpec change against an ALREADY-MERGED spec contract (stage 3-5 of the platform workflow; the spec PR from /opsx:spec-pr must be merged first). REMOTE is the default. Base sync (REMOTE paths: git fetch origin <base> → switch to <base> → merge --ff-only origin/<base>, so the change is implemented on top of the latest base; never auto-stashes; --local skips it and ships from feat/<change>) → Preflight (tools+toolchain, validate, clean tree, branch, load .handoff/<change>/plan.json, assert the contract is on base) → for EACH unit Red→Green→one commit → Verify (resolver-selected per-toolchain gates — uv/go/pnpm + ci-free-gates.sh + coverage + node .claude/workflows/lib/openspec.js validate, repair loop) → Review (code-review-and-quality + security-and-hardening audit; advisory on remote, gates the merge on --local) → Evidence → Sync RECONCILE (re-sync delta vs canonical; a non-empty canonical diff = the contract drifted during implementation → STOP and send back to /opsx:spec + /opsx:spec-pr). REMOTE path: CHANGELOG entry → chore commit (evidence+changelog only; specs already on base) → push + open/update the code PR with the agent review findings + a link to the merged spec PR (stops at PR opened, no auto-merge). LOCAL escape hatch (args.local=true) instead bundles the spec sync, merges feat/<change> into <base> locally, re-verifies, archives, optional tag/PR. --worktree runs all implementation phases inside an isolated git worktree (main checkout stays on <base>; pushes and PR creation are deferred for human local verification). --worktree + --local (or --local-worktree) runs implementation in a worktree then does merge/archive/cleanup in the main checkout — best of both worlds when you want local isolation AND automated finishing. Honors dryRun, only:<unit>, retryBlocked, a token budget reserve, mergeStrategy, bump, noPushMain, archive, skipReview, openPr, worktree, localWorktree, and base.',
  phases: [
    { title: 'Base sync',           detail: 'remote paths: git fetch origin <base> → switch to <base> → merge --ff-only origin/<base> (sync base to origin before preflight; clean tree required, never auto-stash; --local skips — it ships from feat/<change>)' },
    { title: 'Preflight',           detail: 'tools+toolchain, validate, branch, load handoff (--local checks base + branch slug match)' },
    { title: 'Implement',           detail: 'per unit Red→Green→one commit (task-by-task). --worktree: also verify, review, evidence, sync, changelog, chore commit all in isolated worktree; then /opsx:ship-pr for PR' },
    { title: 'Verify',              detail: 'resolver-selected per-toolchain gates (uv/go/pnpm) + ci-free-gates.sh + coverage + node .claude/workflows/lib/openspec.js validate, repair loop' },
    { title: 'Review',              detail: 'code-review-and-quality + security-and-hardening audit of diff vs base (gates --local; advisory + posted on PR for remote)' },
    { title: 'Evidence',            detail: 'write test results, coverage, gates to evidence/' },
    { title: 'Merge',               detail: '(--local) git switch <base> && git merge --{squash,no-ff,ff-only} feat/<change>' },
    { title: 'Post-merge verify',   detail: '(--local) re-run gates on <base> post-merge; halt on failure' },
    { title: 'Sync',                detail: 'reconcile delta vs the already-merged canonical specs; drift → stop (remote). --local: merge delta into openspec/specs/' },
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
const REQUIRED_GO_MINOR = 24
// Polyglot toolchain note injected into EVERY phase that runs gates (Red, Green, Verify).
// This repo is a uv Python workspace + standalone Go modules (go 1.24) + a pnpm portal.
// Each agent runs in a fresh shell, so do NOT assume one fixed toolchain — resolve the
// gates for the touched files with the gate resolver (see GATE_PLAN_NOTE).
const TOOLCHAIN_NOTE = `TOOLCHAIN (polyglot repo — do this FIRST): ensure \`uv\`, \`go\` (1.${REQUIRED_GO_MINOR}+; if a stale go such as /usr/local/go shadows it, prefer a newer one via \`which -a go\` / \`ls /opt/homebrew/bin/go /opt/homebrew/Cellar/go@*/*/bin/go 2>/dev/null\` and \`export PATH=<dir>:$PATH\`), \`pnpm\`, and \`openspec\` are on PATH. Do NOT run one fixed toolchain; resolve the gates for the touched files (see the gate resolver in VERIFY). NOTE: this repo is the \`platform/\` git submodule — all git ops happen inside it; never touch the superproject.`
// Shared resolver instruction reused by Verify and post-merge Verify. base is bound when called.
const gatePlanNote = (b) => `Resolve gates from the diff: \`git diff --name-only ${b}...HEAD | node .claude/workflows/lib/gate-resolver.js --stdin\` prints the exact per-toolchain gate commands — uv ruff/format/pyright/pytest for each touched Python member, go build/vet/test -race for each touched Go module (go 1.${REQUIRED_GO_MINOR}), pnpm typecheck/lint/test for the portal, and \`bash benchmarks/ci-free-gates.sh\` whenever Python/bench is touched — plus the always-gates. RUN EVERY printed command; each must exit 0. DB-dependent pytest skips without TEST_DATABASE_URL/pgvector (skip = not a failure). The LLM benchmark gates (faithfulness>=0.85, citation>=0.95, p95<=12s, tenant-isolation, ACL-escape) are NOT in this loop — they run only with --llm-gates or in CI judge-gates.`
const llmGates = A.llmGates === true
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
// --worktree: run implementation phases inside an isolated git worktree.
// The main checkout stays on main, freeing it for parallel work on other changes.
// Unlike the default remote path, worktree mode does NOT push or create a PR —
// it stops after implementation for human local verification.
const worktree = A.worktree === true
if (A.worktree && local) {
  throw new Error('--worktree is incompatible with --local; worktree mode requires the remote path (PR-gated)')
}

if (!change || typeof change !== 'string') {
  throw new Error('ship-code requires args { change, date, dryRun?, only?, retryBlocked?, reserveTokens?, local?, base?, mergeStrategy?, bump?, noPushMain?, archive?, skipReview?, keepBranch?, openPr?, worktree?, localWorktree? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z0-9][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (expected kebab-case slug): ' + change)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)
if (onlyPair && !/^[0-9]{1,3}$/.test(onlyPair)) throw new Error('Unsafe only (expected a pair ordinal like "02"): ' + onlyPair)
if (!/^[A-Za-z0-9._/-]+$/.test(base)) throw new Error('Unsafe base branch: ' + base)
const DATE = date || 'Unreleased'
const branch = `feat/${change}`
const handoffDir = `.handoff/${change}`

// ---------------------------------------------------------------- skills are injected dynamically via prompt hooks
// See extensions/agent-skills/Hooks/on-<event>.prompt.md for the skill references at each phase.

// ---------------------------------------------------------------- agent-form lifecycle hook (best-effort)
// Workflow bodies cannot require() shared libs, so this compact runner is inlined per workflow.
// Runs the repo-defined openspec/hooks/on-<event>.agent.md so a project can wire lifecycle events
// to a backlog board WITHOUT a built-in adapter or config.
const HOOK_RESULT = {
  type: 'object', additionalProperties: false, required: ['found', 'ran'],
  properties: { found: { type: 'boolean' }, ran: { type: 'boolean' }, summary: { type: 'string' }, errors: { type: 'array', items: { type: 'string' } } },
}
async function runAgentHook(event, contextLines) {
  const r = await agent(
    [
      `AGENT-FORM LIFECYCLE HOOK — event "${event}", OpenSpec change "${change}". BEST-EFFORT: NEVER fail; capture problems in errors[].`,
      `Context:`,
      ...contextLines,
      `1. If openspec/hooks/on-${event}.agent.md does NOT exist → return { found:false, ran:false }. STOP (normal no-op — most repos have no hook).`,
      `2. If it exists → READ it and FOLLOW its instructions as your task, using the context above. It may use gh/git/node. It typically reads the linked GitHub issue from openspec/changes/${change}/github.json and updates the issue/board. Summarise what you actually did in summary; set found:true and ran:true (or ran:false + an errors[] entry if its instructions failed).`,
      `Return { found, ran, summary, errors }.`,
    ].join('\n'),
    { schema: HOOK_RESULT, label: `hook:${event}`, phase: 'Hooks', agentType: 'general-purpose' },
  )
  if (r && r.ran) log(`Agent hook on-${event}.agent.md: ${r.summary || 'ran'}`)
  else if (r && r.found) log(`Agent hook on-${event}.agent.md: failed${r.errors && r.errors.length ? ' — ' + r.errors.join('; ') : ''}`)
  return r
}

// Prompt hooks — reads openspec/hooks/on-<event>.prompt.md files and returns their text
async function getPromptHooks(event, context = {}) {
  const change = context.change || ''
  return agent([
    `Event: ${event}`,
    `Change: ${change}`,
    `Read openspec/hooks/on-${event}.prompt.md and each extensions/*/Hooks/on-${event}.prompt.md.`,
    'For each file found: return its contents as a string in the array.',
    'If no files found: return an empty array.',
  ].join('\n'), {
    schema: {
      type: 'object', additionalProperties: false, required: ['prompts'],
      properties: {
        prompts: { type: 'array', items: { type: 'string' }, description: 'Prompt text fragments from hook files' },
      },
    },
    label: `prompt-hooks:${event}`,
    phase: 'Hooks',
  }).then(r => (r && r.prompts) || [])
}

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
          tags: { type: 'array', items: { type: 'string' }, description: 'unit tags inherited from covered tasks, e.g. ["ui", "backend"]. Drives skill/hook loading during implementation.' },
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
const SYNCED = { type: 'object', additionalProperties: false, required: ['synced', 'notes', 'drift', 'driftPaths'], properties: { synced: { type: 'boolean' }, notes: { type: 'string' }, drift: { type: 'boolean' }, driftPaths: { type: 'array', items: { type: 'string' } } } }
const FINALIZE = {
  type: 'object', additionalProperties: false, required: ['changelogWritten', 'choreCommitted', 'pushed', 'prUrl', 'prExisted', 'notes'],
  properties: {
    changelogWritten: { type: 'boolean' }, choreCommitted: { type: 'boolean' },
    pushed: { type: 'boolean' }, prUrl: { type: ['string', 'null'] },
    prExisted: { type: 'boolean' }, notes: { type: 'string' },
  },
}
// --- Worktree-path schema (used when args.worktree=true) ---
const WORKTREE_RESULT = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'change', 'branch', 'commits', 'gatesRun', 'failureLog'],
  properties: {
    ok: { type: 'boolean' },
    change: { type: 'string' },
    branch: { type: 'string' },
    failureStage: { type: 'string' },
    failureLog: { type: 'string' },
    commits: {
      type: 'array', description: 'per-unit red+green commits',
      items: {
        type: 'object', additionalProperties: false,
        required: ['pair', 'sha'],
        properties: { pair: { type: 'string' }, title: { type: 'string' }, sha: { type: 'string' } },
      },
    },
    repairs: { type: 'integer' },
    gatesRun: { type: 'array', items: { type: 'string' } },
    coverage: { type: 'string' },
    reviewVerdict: { type: 'string' },
    reviewFindings: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        required: ['severity', 'axis', 'location', 'problem', 'suggestion'],
        properties: {
          severity: { type: 'string', enum: ['blocker', 'required', 'nit', 'fyi'] },
          axis: { type: 'string' },
          location: { type: 'string' },
          problem: { type: 'string' },
          suggestion: { type: 'string' },
        },
      },
    },
    evidenceDir: { type: 'string' },
    evidenceFiles: { type: 'array', items: { type: 'string' } },
    synced: {
      type: 'object', additionalProperties: false,
      properties: { synced: { type: 'boolean' }, notes: { type: 'string' }, drift: { type: 'boolean' }, driftPaths: { type: 'array', items: { type: 'string' } } },
    },
    changelogWritten: { type: 'boolean' },
    choreCommitted: { type: 'boolean' },
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

// ---------------------------------------------------------------- Phase 0: Base sync
// Sync the base branch to origin BEFORE preflight so the change is built on top of the
// latest <base>: fetch origin, switch to <base>, fast-forward to origin/<base>. Then the
// Preflight phase creates/checks out feat/<change> from the freshened <base>.
// REMOTE paths only (default + --worktree). The --local escape hatch is the offline path:
// it must already be ON feat/<change> (its preflight asserts this) and handles its own base
// in the Merge phase, so we never switch it away here. Never auto-stashes / force-resets.
const BASE_SYNC = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'base', 'baseSha', 'synced'],
  properties: {
    ok: { type: 'boolean' },
    reason: { type: 'string' },
    base: { type: 'string' },
    baseSha: { type: 'string', description: 'sha of <base> after the fast-forward' },
    synced: { type: 'boolean', description: 'true when local <base> now equals origin/<base>' },
  },
}
if (!local) {
  phase('Base sync')
  const sync = await agent(
    [
      `Sync the base branch "${base}" to origin before shipping change "${change}". Use Bash ONLY (git). ${TOOLCHAIN_NOTE}`,
      `All git ops run INSIDE the platform submodule — never touch the superproject. Steps:`,
      `1. git fetch origin "${base}" --prune. If there is no "origin" remote or "${base}" does not exist on origin, ok=false, reason="origin/${base} not found — push ${base} first or choose another base", STOP.`,
      `2. CLEAN-TREE GUARD: git status --porcelain. Ignore anything under .claude/ and .handoff/ (in-flight workflow dev + the gitignored handoff). If ANY other tracked file is dirty, ok=false, reason="uncommitted tracked changes; commit or stash before ship (base sync needs a clean tree)", STOP. NEVER auto-stash.`,
      `3. Switch to the base branch: if a local "${base}" exists, \`git switch "${base}"\`; otherwise create it tracking origin: \`git switch -c "${base}" --track "origin/${base}"\`. If the switch fails, ok=false+reason+STOP.`,
      `4. Fast-forward to origin: \`git merge --ff-only "origin/${base}"\`. If this is NOT a fast-forward (local "${base}" has diverged from origin/${base}), ok=false, reason="local ${base} has diverged from origin/${base}; reconcile it manually (e.g. git reset --hard origin/${base} if the local-only commits are disposable), then re-run", STOP. Do NOT force or merge non-ff.`,
      `5. baseSha = git rev-parse HEAD. synced=true, ok=true. (The next phase creates/checks out feat/${change} from this freshened ${base}.)`,
      `Return the structured result; implement nothing here.`,
    ].join('\n'),
    { schema: BASE_SYNC, label: 'base-sync', phase: 'Base sync', agentType: 'general-purpose' },
  )
  if (!sync || !sync.ok) {
    return { stage: 'base-sync', ok: false, reason: sync ? sync.reason : 'base-sync agent returned null', change, branch, base }
  }
  log(`base sync ok — ${base} fast-forwarded to origin/${base} (${(sync.baseSha || '').slice(0, 8)})`)
}

// ---------------------------------------------------------------- Phase 1: Preflight (load handoff)
phase('Preflight')
const pre = await agent(
  [
    `Preflight ship-code for OpenSpec change "${change}" on branch "${branch}"${local ? ' (LOCAL PATH — base="' + base + '", mergeStrategy=' + mergeStrategy + ')' : ''}. Use Bash. Steps:`,
    `1. TOOLCHAIN + TOOLS (polyglot): test -f .claude/workflows/lib/openspec.js && command -v uv go pnpm node. Resolve which toolchains this change actually needs: \`git diff --name-only ${base}...HEAD | node .claude/workflows/lib/gate-resolver.js --stdin --json\` → its "toolchains" array. Only the tools for those toolchains are required (py→uv, go→go 1.${REQUIRED_GO_MINOR}+, ts→pnpm; openspec.js + node always). For go, run \`go version\`; if < 1.${REQUIRED_GO_MINOR}, look for a newer one via \`which -a go\` / \`ls /opt/homebrew/Cellar/go@*/bin/go\` and export PATH; if a needed tool is missing entirely set toolchainOk=false+ok=false+reason (e.g. "go >= 1.${REQUIRED_GO_MINOR} not found; brew install go") and STOP.`,
    `   - Capture the resolved toolchains + versions; set toolchainOk=true when every NEEDED tool is present.`,
    `   - gh is OPTIONAL — only required when args.local is false (the local path never calls gh).`,
    `2. Load the handoff: read "${handoffDir}/plan.json". If it does not exist, set ok=false, reason="no handoff — run /opsx:ship-plan ${change} first" and STOP. It contains a "units" array (a FEW test-first work-units). Map EACH unit to one entry in the returned pairs array (one Red→Green→commit per unit), ordered by ascending id: pair=unit.id; title=unit.title; coversTasks=unit.coversTasks; tags=unit.tags||[]; allDone=(unit.status=="done"); file (for both test and code) = "${handoffDir}/tasks/<unit.id>-<unit.slug>.md"; test={id:unit.id, role:"test", status:unit.status, file, deliverables:unit.testDeliverables, verify:unit.verify, skipRed:unit.skipRed}; code={id:unit.id, role:"code", status:unit.status, file, deliverables:unit.codeDeliverables, verify:unit.verify}. (Legacy fallback: if plan.json has the old "tasks" array instead of "units", group tasks into pairs by their "pair" field and set deliverables=[task.deliverable].)`,
    `3. node .claude/workflows/lib/openspec.js status --change "${change}" --json — capture changeRoot, proposal/tasks paths, delta-spec paths, title. node .claude/workflows/lib/openspec.js list --json — capture isActive (change present in active list) and isArchived (change present in archive list).`,
    `4. node .claude/workflows/lib/openspec.js validate "${change}" --strict (fallback non-strict) — MUST pass else ok=false+reason+STOP.`,
    `5. WORKING-TREE HYGIENE: git status --porcelain. ${handoffDir}/ is gitignored and does not count. Two cases:`,
    `   a. The ONLY tracked changes are under .claude/ (workflow / command / skill dev in flight, unrelated to the change) → treePolicy="dirty-workflow-dev-only", warn but proceed.`,
    `   b. Any other tracked file is dirty → treePolicy="dirty-blocked", ok=false, reason="uncommitted tracked changes outside .claude/; commit or stash first", STOP.`,
    `6. Branch handling:`,
    local
      ? `   - LOCAL PATH: the working branch MUST be "${branch}" (a feat/<change> branch). If currently on "${base}" with no commits ahead, ok=false, reason="on <base> with no commits; create ${branch} and ship from there". If on any other branch, ok=false, reason="currently on <other>; switch to ${branch}". If the branch is named e.g. feat/cNNNN-wrong-slug but the change slug is ${change}, ok=false, reason="branch slug does not match change slug; rename with: git branch -m feat/${change}". If you are already on ${branch}, branchReady=true. Do NOT create the branch — it must exist (the implement phase already worked on it).`
      : worktree
        ? `   - WORKTREE PATH: branch will be created inside the worktree. branchReady=true, branchName="${branch}". Skip branch creation.`
        : `   - Create/checkout branch "${branch}" (git checkout -b "${branch}" from the freshly-synced "${base}", or git checkout "${branch}" if it already exists); confirm not on "${base}"; branchReady=true.`,
    local ? `7. For LOCAL PATH: also check the base branch exists (git rev-parse --verify ${base}); capture its sha as baseSha for the merge phase. If base does not exist, ok=false+reason+STOP.` : ``,
    `8. SPEC-MERGED GATE: this stage implements AGAINST an already-merged spec contract (the spec PR from /opsx:spec-pr lands canonical specs on ${base} first). For each capability under the change's delta specs, confirm a canonical openspec/specs/<capability>/spec.md exists on this branch. If a delta ADDS a capability whose canonical spec is absent, the spec PR was not merged → ok=false, reason="spec PR not merged — run /opsx:spec-pr ${change}, merge it, then branch ${branch} from an updated ${base}". (A MODIFIED-but-unsynced spec is caught later by the Sync reconcile drift guard.)`,
    dryRun ? `` : `9. LIFECYCLE (best-effort — NEVER fail preflight on this): run \`node .claude/workflows/lib/lifecycle.js before-ship --change "${change}" --branch "${branch}"\` to log "implementation starting" on the linked ticket. No-ops when the change isn't linked to a ticket; on any error, log and CONTINUE.`,
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
if (!dryRun) {
  phase('Hooks')
  await runAgentHook('before-ship', [`- change: ${change}`, `- title: ${title}`, `- branch: ${branch}`])
}
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

// ---------------------------------------------------------------- Worktree path
// When --worktree is set, run all implementation phases (Implement → Verify →
// Review → Evidence → Sync → Changelog) inside a single isolated git worktree
// via agent({ isolation: 'worktree' }).
//
// Two modes:
//   1. worktree-only (remote worktree): The agent does NOT push/create a PR.
//      Stops after implementation so the human can verify locally, then run
//      /opsx:ship-pr <change> to push + PR. Main checkout stays on base.
//   2. worktree + local (--local-worktree): Implementation runs in worktree,
//      then merge/archive/cleanup runs in the main checkout.
let worktreeDidImplement = false
let wtResult = null
const commits = []
let blocked = null
if (worktree) {
  phase('Implement')

  const handoffSummary = runnable.map((p) =>
    `unit ${p.pair}: ${p.title} (covers ${(p.coversTasks || []).join(', ') || 'n/a'})` +
    `; test=${(p.test.deliverables || []).join(', ') || '(none)'}` +
    `; code=${(p.code.deliverables || []).join(', ') || '(none)'}` +
    (p.test.skipRed ? '; skipRed' : '') +
    (p.allDone ? '; already-done' : '')
  ).join('\n')

  const specPathsStr = (pre.specPaths || []).join(', ') || '(none)'

  wtResult = await agent(
    [
      `Implement OpenSpec change "${change}" test-first in this isolated git worktree.`,
      ``,
      `CONTEXT:`,
      `- Change: ${change} — "${title}"`,
      `- Proposal: ${pre.proposalPath || '(n/a)'}`,
      `- Tasks: ${pre.tasksPath}`,
      `- Delta specs: ${specPathsStr}`,
      `- Evidence dir: ${pre.changeRoot}/evidence/`,
      `- All existing code is on base branch "${base}" (this worktree was created from it)`,
      ``,
      `HANDOFF (units to implement):`,
      handoffSummary,
      ``,
      `TOOLCHAIN: ensure uv, go (1.${REQUIRED_GO_MINOR}+; try \`which -a go\` / \`ls /opt/homebrew/Cellar/go@*/bin/go\` if stale), pnpm, openspec, node are on PATH.`,
      `Resolve gates per touched file: \`git diff --name-only ${base}...HEAD | node .claude/workflows/lib/gate-resolver.js --stdin\`.`,
      `DB-dependent pytest skips without TEST_DATABASE_URL/pgvector (skip != failure).`,
      ``,
      `STEPS:`,
      ``,
      `1. WORKTREE SETUP:`,
      `   cd into the worktree root (pwd)`,
      `   git switch -c feat/${change} ${base}`,
      ``,
      `2. For EACH runnable unit, implement test-first:`,
      `   a. RED: Write ALL test deliverables for that unit. Run them — they MUST FAIL.`,
      `      If the unit is skipRed (doc-only), set skipRed=true and skip to Green.`,
      `   b. GREEN: Implement the MINIMAL production code. Tests must PASS.`,
      `      Tick the covered tasks in ${pre.tasksPath} ("- [ ]" → "- [x]").`,
      `      Update the unit's status to "done" in .handoff/${change}/plan.json.`,
      `   c. COMMIT: git add -A && git commit -s -m "feat: <unit title> (${change} unit <id>)" -m "Co-Authored-By: Claude <noreply@anthropic.com>"`,
      `   d. Move to next unit.`,
      ``,
      `3. FULL VERIFY (resolver-driven):`,
      `   - git diff --name-only ${base}...HEAD | node .claude/workflows/lib/gate-resolver.js --stdin → run EVERY printed gate`,
      `   - node .claude/workflows/lib/openspec.js validate "${change}" --strict (best-effort)`,
      `   - If a gate fails, fix and re-run (max ${maxRepairs} repair attempts)`,
      `   - Capture gatesRun array and coverage string`,
      ``,
      `4. CODE REVIEW:`,
      `   - git diff ${base}..HEAD --stat; git diff ${base}..HEAD -- . ':(exclude)openspec/' ':(exclude).handoff/'`,
      `   - Categorize findings: blocker/required/nit/fyi on axes: correctness/readability/architecture/security/performance`,
      `   - BLOCKER: correctness bug; security issue; breaks platform invariant; contradicts merged spec`,
      `   - PASS = no blockers AND <= 2 required findings`,
      `   - Do NOT edit files — just audit and report findings`,
      ``,
      `5. EVIDENCE:`,
      `   - mkdir -p ${pre.changeRoot}/evidence/`,
      `   - Write gates.md (toolchain|unit|gate|command|result table + coverage summary)`,
      `   - Write test-results.md (per-toolchain test tails)`,
      `   - Write coverage.txt (per-toolchain coverage summary)`,
      ``,
      `6. SYNC (RECONCILE):`,
      `   - Confirm openspec/specs/ is clean: git diff --name-only -- openspec/specs/ (expect empty)`,
      `   - Invoke the openspec-sync-specs skill to re-sync delta specs against canonical`,
      `   - DRIFT CHECK: if git diff --name-only -- openspec/specs/ is non-empty, set drift=true, driftPaths=those files, and REVERT`,
      `   - If drift=true, STOP after revert (return ok=false, failureLog="spec drift: <files>")`,
      ``,
      `7. CHANGELOG:`,
      `   - Prepend bullet(s) under "## [Unreleased]" in CHANGELOG.md (create if absent)`,
      `   - Group Added/Changed/Removed per the delta sections, each ending " (${change})"`,
      ``,
      `8. CHORE COMMIT (evidence + changelog only — do NOT push, do NOT create PR):`,
      `   - git add "${pre.changeRoot}/evidence/" CHANGELOG.md`,
      `   - git commit -s -m "chore(${change}): evidence, changelog" -m "Co-Authored-By: Claude <noreply@anthropic.com>"`,
      ``,
      `9. RETURN a structured result matching the WORKTREE_RESULT schema.`,
      local
        ? `   (--local mode) The branch feat/${change} with all commits will be merged into "${base}" after this worktree returns. Set pushed=false, prUrl=null.`
        : `   Set pushed=false and prUrl=null. The branch feat/${change} exists locally with all implementation + chore commits. The user will test locally, then run \`/opsx:ship-pr ${change}\` to push + create the PR.`,
    ].join('\n'),
    {
      schema: WORKTREE_RESULT,
      label: `worktree:${change}`,
      phase: 'Implement in worktree',
      isolation: 'worktree',
      agentType: 'general-purpose',
    },
  )

  if (!wtResult || !wtResult.ok) {
    return {
      stage: 'implement-in-worktree',
      ok: false,
      reason: wtResult ? `worktree implementation failed: ${wtResult.failureLog || '(no details)'}` : 'worktree agent returned null',
      change, branch,
      commits: (wtResult && wtResult.commits) || [],
      gatesRun: (wtResult && wtResult.gatesRun) || [],
      coverage: (wtResult && wtResult.coverage) || '',
      failureLog: wtResult ? wtResult.failureLog : 'worktree agent returned null',
    }
  }

  if (!local) {
    // REMOTE WORKTREE: return results for human verification (no push/PR)
    log(`worktree: ${change} done — ${(wtResult.commits || []).length} commit(s) on feat/${change}. Run /opsx:ship-pr ${change} to create the PR.`)
    return {
      stage: 'done', ok: true, mode: 'worktree',
      change, title, branch, dryRun,
      commits: wtResult.commits || [],
      repairs: wtResult.repairs || 0,
      gatesRun: wtResult.gatesRun || [],
      coverage: wtResult.coverage || '',
      reviewVerdict: wtResult.reviewVerdict || 'n/a',
      reviewFindings: wtResult.reviewFindings || [],
      evidenceDir: wtResult.evidenceDir || `${pre.changeRoot}/evidence`,
      skillsApplied: [],
      specsSynced: wtResult.synced ? wtResult.synced.synced : false,
      changelogWritten: !!wtResult.changelogWritten,
      choreCommitted: !!wtResult.choreCommitted,
      notes: `Worktree path complete. Branch "${branch}" is ready. Main checkout was never switched away from "${base}".`,
      nextStep: `Test locally, then run /opsx:ship-pr ${change} to push + create the PR.`,
    }
  }

  // LOCAL + WORKTREE: extract worktree results, then continue with merge/archive/cleanup in main checkout
  log(`worktree: ${change} done — ${(wtResult.commits || []).length} commit(s). Proceeding to local merge/archive/cleanup in main checkout.`)
  worktreeDidImplement = true
  commits.push(...(wtResult.commits || []))
  gatesRun = wtResult.gatesRun || []
  coverage = wtResult.coverage || ''
  review = { verdict: wtResult.reviewVerdict || 'pass', findings: wtResult.reviewFindings || [] }
  synced = wtResult.synced || { synced: false, notes: 'from worktree' }
  evidenceDir = wtResult.evidenceDir || `${pre.changeRoot}/evidence`
  // Fall through to merge/archive/cleanup — skip inline implement/verify/review/evidence/sync
}

// ---------------------------------------------------------------- Phase 2: Implement (per pair: Red → Green → one commit)
if (!worktreeDidImplement) {
  phase('Implement')
  blocked = null
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
        `Unit ${p.pair} of change "${change}" — the RED step. ${await getPromptHooks('on-test', { change }).then(h => h.join(' '))}`,
        CONTEXT,
        `TAGS: ${(p.tags || []).join(", ") || "(none)"}. Consult .claude/skills/tag-system/SKILL.md for tag→skill mapping.`,
        TOOLCHAIN_NOTE,
        `Read the unit file "${p.test.file}" (its "Test plan (Red)" section). Write ALL of this unit's test deliverables — ${testFiles} — in the deliverables' own language: pytest \`tests/test_*.py\` for Python members, table-driven \`*_test.go\` for Go modules, vitest \`*.test.ts(x)\` for the portal — with the assertions/table cases the unit specifies (drawn from the delta-spec scenarios).`,
        p.test.skipRed
          ? `This unit is marked skipRed (doc-only/non-testable). Set skipRed=true with the reason; do not fabricate a test.`
          : `Then run the touched package's test command (py: \`uv --directory <member> run python -m pytest -q <path>\`; go: \`(cd <module> && go test -race ./...)\`; ts: \`(cd apps/portal && pnpm test)\`) — and CONFIRM IT FAILS (undefined symbols or failing assertions). Set redConfirmed=true ONLY after observing the failure. Put the failing output in failureLog.`,
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
        `Unit ${p.pair} of change "${change}" — the GREEN step + commit. ${await getPromptHooks('on-implement', { change }).then(h => h.join(' '))}`,
        CONTEXT,
        `TAGS: ${(p.tags || []).join(", ") || "(none)"}. Consult .claude/skills/tag-system/SKILL.md for tag→skill mapping.`,
        TOOLCHAIN_NOTE,
        `Read the unit file "${p.code.file}" (its "Code plan (Green)" section). Make the MINIMAL production change across this unit's code deliverables — ${codeFiles} — to turn the failing test(s) from the Red step GREEN. Do not over-build.`,
        red.skipRed ? `(No Red test — implement the doc/config change described.)` : `Run the touched package's test command (py: \`uv --directory <member> run python -m pytest -q\`; go: \`(cd <module> && go test -race ./...)\`; ts: \`(cd apps/portal && pnpm test)\`) — they MUST pass. Iterate up to ${maxRepairs} times if needed (fix production code, not the tests). If still failing, set greenConfirmed=false and put the output in failureLog (do not commit).`,
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
  } // end for
  if (blocked) {
    return { stage: 'implement', ok: false, reason: `pair ${blocked.pair} blocked — stopping before PR. ${blocked.why}`, change, branch, commits, blockedPair: blocked.pair }
  }
  log(`implement: ${commits.length} per-task commit(s) made`)
} // end if (!worktreeDidImplement) — inline implement done

// Shared variables used by both inline and worktree paths.
// Declared here so the local merge path can use them regardless of origin.
let repairs = 0
let gatesRun = []
let coverage = ''
let review = null
let evidenceDir = `${pre.changeRoot}/evidence`
let synced = { synced: false, notes: 'no delta specs', drift: false, driftPaths: [] }

// ---------------------------------------------------------------- Phase 3: Verify (deterministic-first + repair loop)
if (!worktreeDidImplement) {
  phase('Verify')
  const coverProfile = `/tmp/shipcode-${change}.cover`
  function verifyPrompt() {
    return [
      `Verify the full tree on branch "${branch}" for change "${change}". DETERMINISTIC gate — pass is exit-code-driven. Use Bash, run in order:`,
      TOOLCHAIN_NOTE,
      gatePlanNote(base),
      `Capture the coverage line from each toolchain's coverage gate (py: pytest --cov term-missing total; go: \`go tool cover -func\` tail; ts: vitest summary) into the coverage field as a short per-toolchain summary.`,
      llmGates ? `LLM GATES (--llm-gates set): also run the benchmark grid and the preserved threshold scripts — \`bash benchmarks/gates/faithfulness-gte.sh 0.85\`, \`citation-accuracy-gte.sh 0.95\`, \`latency-p95-lte.sh 12000\`, \`tenant-isolation-test.sh\`, \`retrieve-kb-acl-test.sh\` (needs ANTHROPIC_* + a pgvector DB). Record llmGates=passed/failed.` : `LLM gates NOT requested (no --llm-gates) — record llmGates=skipped; do not run the LLM benchmark grid.`,
      `pass=true only if every gate that ran exited 0 and all tests are green. List every command run in gatesRun. On failure, pass=false + first failing gate's trimmed output in failureLog. Do not edit files.`,
    ].join('\n')
  }
  let verdict = await agent(verifyPrompt(), { schema: VERDICT, label: 'verify', phase: 'Verify', agentType: 'general-purpose' })
  repairs = 0
  while (verdict && !verdict.pass && repairs < maxRepairs) {
    if (budget && budget.total && budget.remaining() < reserve) { log('budget reserve reached during repair'); break }
    repairs++
    log(`verify failed — repair ${repairs}/${maxRepairs}`)
    const repaired = await agent(
      [
        `The full verify gate failed for change "${change}". Make the SMALLEST in-scope fix. ${await getPromptHooks('on-verify', { change }).then(h => h.join(' '))}`,
        `Failing output:\n${verdict.failureLog}`,
        `Prefer fixing production code over weakening tests. Then amend it into the most relevant per-task commit (git add -A && git commit --amend --no-edit) OR a new fixup commit if it spans pairs. Do not push. If out of scope, set fixed=false.`,
      ].join('\n'),
      { schema: REPAIR, label: `repair:${repairs}`, phase: 'Verify', agentType: 'general-purpose' },
    )
    if (!repaired || !repaired.fixed) { log(`repair ${repairs} did not fix it: ${repaired ? repaired.notes : 'null'}`); break }
    verdict = await agent(verifyPrompt(), { schema: VERDICT, label: `verify:retry${repairs}`, phase: 'Verify', agentType: 'general-purpose' })
  }
  gatesRun = (verdict && verdict.gatesRun) || []
  coverage = (verdict && verdict.coverage) || ''
  if (!verdict || !verdict.pass) {
    return { stage: 'verify', ok: false, reason: 'verification did not pass — stopping before PR', failureLog: verdict ? verdict.failureLog : 'verify agent returned null', repairs, gatesRun, change, branch, commits }
  }
  log(`verify passed (${repairs} repair(s)); ${coverage || 'coverage n/a'}; gates: ${gatesRun.join(' | ')}`)
} // end if (!worktreeDidImplement) — inline verify done

// ---------------------------------------------------------------- Phase 3b: Code review (BOTH paths — code-review-and-quality + security-and-hardening)
// Agent PRE-review of the diff. LOCAL: gates the local merge (blocker → stop). REMOTE: advisory —
// findings are posted on the code PR for the human approver (agent assists, human decides).
if (!worktreeDidImplement) {
  phase('Review')
  if (skipReview) {
    log('code review skipped via --skipReview')
    review = { verdict: 'pass', findings: [], axes: [], diffStat: '', notes: 'skipped via --skipReview' }
  } else {
    review = await agent(
      [
        `Code review of change "${change}" on branch "${branch}" vs base "${base}". Read-only audit. ${await getPromptHooks('on-review', { change }).then(h => h.join(' '))}`,
        `1. Compute the diff: git diff "${base}..${branch}" --stat ; git diff "${base}..${branch}" -- . ':(exclude)openspec/' ':(exclude).handoff/' | head -1500.`,
        `2. Categorize findings by severity: blocker / required / nit / fyi. Axes: correctness / readability / architecture / security / performance. For each finding give file:line, problem, suggestion.`,
        `3. BLOCKER criteria — any of: correctness bug; security issue; breaks a platform invariant (multi-tenant: tenant_id on every query/cache key + ACL-cohort hash, cross-tenant joins; citations-mandatory: filter refuse_if no_citations; temperature==0 on synthesize/cite/filter; ACL server-side in retrieve_kb with caller identity inherited never trusted from model; append-only BotVersion/KBVersion; MCP internal tool boundary; no LangChain/LangGraph; OpenAPI is the API contract, portal types generated); the implementation contradicts the merged spec; or a test does not actually assert the spec scenario.`,
        `4. PASS = no blockers AND <= 2 required findings. Else FAIL.`,
        `5. Do NOT edit any file — return findings + verdict. Capture the raw diffStat output verbatim.`,
      ].join('\n'),
      { schema: REVIEW, label: 'code-review', phase: 'Review', agentType: 'general-purpose' },
    )
    log(`code review: ${review ? review.verdict : 'null'} (${review ? review.findings.length : 0} findings)${local ? '' : ' — advisory (posted on PR)'}`)
  }
  // LOCAL path gates the merge; REMOTE path surfaces findings on the PR and lets the human decide.
  if (local && (!review || review.verdict !== 'pass')) {
    return {
      stage: 'review', ok: false,
      reason: `code review verdict=${review ? review.verdict : 'null'} — halting before local merge. Fix on ${branch} and re-run; pairs already done are skipped.`,
      change, branch, base, commits, repairs, gatesRun, coverage,
      reviewVerdict: review ? review.verdict : 'fail',
      reviewFindings: review ? review.findings : [],
    }
  }
} // end if (!worktreeDidImplement) — inline review done

// ---------------------------------------------------------------- Phase 4: Evidence
if (!worktreeDidImplement) {
  phase('Evidence')
  const evidence = await agent(
    [
      `Write the evidence bundle for change "${change}" into "${pre.changeRoot}/evidence/" (create dir). Use Bash/Write. It moves to the archive with the change and is linked from the PR.`,
      `- gates.md — a table \`toolchain | unitDir | gate | command | result\` of the gates that ran (${gatesRun.join('; ')}), incl. the free bench ladder and \`node .claude/workflows/lib/openspec.js validate\`, plus the llmGates tier status; the per-unit commits (${commits.map((c) => c.pair + ':' + c.sha).join(', ')}), repair count (${repairs}), and the governing skills (${(await getPromptHooks('on-ship-end', { change }).then(h => h.join(', ')))}).`,
      `- test-results.md — concatenate the per-toolchain test tails: \`uv --directory <member> run python -m pytest -q 2>&1 | tail -40\` per touched Python member, \`(cd <module> && go test -race ./...) 2>&1 | tail -40\` per Go module, \`(cd apps/portal && pnpm test) 2>&1 | tail -40\` if the portal was touched (note DB-dependent pytest skips without TEST_DATABASE_URL; e2e skips without browsers — skips are not failures).`,
      `- coverage.txt — one block per toolchain (py: pytest --cov term-missing summary; go: \`go tool cover -func\` tail; ts: vitest coverage summary), else "coverage not captured".`,
      `Concise + factual. Do NOT commit (a later step commits evidence). Return the dir + file list.`,
    ].join('\n'),
    { schema: EVIDENCE, label: 'evidence', phase: 'Evidence', agentType: 'general-purpose' },
  )
  evidenceDir = (evidence && evidence.evidenceDir) || `${pre.changeRoot}/evidence`
  log(`evidence: ${evidence && evidence.written ? evidence.files.join(', ') : (evidence ? evidence.notes : 'failed')}`)
} // end if (!worktreeDidImplement) — inline evidence done

// ---------------------------------------------------------------- Phase 5: Sync delta specs (RECONCILE + drift guard)
// The contract was already merged by the spec PR (/opsx:spec-pr). Re-running sync here is
// idempotent in the happy path; a non-empty canonical diff means implementation evolved the
// delta beyond the merged contract → DRIFT → stop before the code PR and send back to /opsx:spec.
if (!worktreeDidImplement) {
  phase('Sync')
  synced = { synced: false, notes: 'no delta specs', drift: false, driftPaths: [] }
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'finalize', ok: false, reason: 'budget reserve reached before sync/changelog/PR; per-task commits are on the branch (specs not yet reconciled)', change, branch, commits, gatesRun, coverage, dryRun }
  }
  if (pre.specPaths && pre.specPaths.length) {
    const s = await agent(
      local
        ? [
            // LOCAL escape hatch: no prior spec PR — sync delta→canonical and KEEP the edits
            // (they are committed in the merge chore commit below). No drift guard here.
            `Sync the delta specs for change "${change}" into the canonical specs: invoke Skill({ skill: "openspec-sync-specs" }) for change "${change}". Merges ADDED/MODIFIED/REMOVED/RENAMED from ${pre.specPaths.join(', ')} into openspec/specs/<capability>/spec.md, idempotent.`,
            `Do NOT commit (the merge phase stages openspec/specs/). Set synced=true if it ran, drift=false, driftPaths=[].`,
          ].join('\n')
        : [
            // REMOTE/spec-first: the contract was merged by /opsx:spec-pr — this is a RECONCILE.
            `RECONCILE the delta specs for change "${change}" against the already-merged canonical specs. Use Bash + Skill({ skill: "openspec-sync-specs" }).`,
            `1. Confirm openspec/specs/ is clean: git diff --name-only -- openspec/specs/ (expect empty).`,
            `2. Invoke Skill({ skill: "openspec-sync-specs" }) for change "${change}" — merge from ${pre.specPaths.join(', ')}, idempotent.`,
            `3. DRIFT CHECK: git diff --name-only -- openspec/specs/ . If NON-EMPTY, implementation changed the contract beyond the merged spec PR → set drift=true, driftPaths=those files, and REVERT (git checkout -- openspec/specs/) so this run never silently changes the contract. If EMPTY, drift=false (contract already in sync).`,
            `Do NOT commit. Return synced, drift, driftPaths.`,
          ].join('\n'),
      { schema: SYNCED, label: local ? 'sync-specs' : 'reconcile-specs', phase: 'Sync', agentType: 'general-purpose' },
    )
    if (s) synced = s
  }
  if (synced.drift) {
    return { stage: 'sync', ok: false, change, branch, commits, gatesRun, coverage, dryRun,
      reason: `spec drifted during implementation: ${(synced.driftPaths || []).join(', ')}. The merged contract no longer matches the delta. Author the change to the delta specs, run /opsx:spec ${change} (re-review), then /opsx:spec-pr ${change} (merge the updated contract) before re-shipping. Code commits stand on ${branch}.` }
  }
  log(local ? `sync: merged delta specs (local bundle)` : `sync: reconcile clean (contract in sync with the merged spec PR)${synced.synced ? '' : ' — no delta specs'}`)
} // end if (!worktreeDidImplement) — inline sync done

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
      `Local-merge branch "${branch}" into "${base}" for change "${change}". Use Bash ONLY (no gh, no remote). ${await getPromptHooks('on-local-merge', { change }).then(h => h.join(' '))}`,
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
      `2. Resolve gates from the just-merged delta: \`git diff --name-only HEAD~1 HEAD | node .claude/workflows/lib/gate-resolver.js --stdin\` and RUN every printed per-toolchain gate (uv/go/pnpm + ci-free-gates.sh as applicable). DB-dependent pytest skips without TEST_DATABASE_URL/pgvector (not a failure).`,
      `3. node .claude/workflows/lib/openspec.js validate "${change}" --strict (fallback non-strict). Capture a short per-toolchain coverage summary.`,
      `pass=true only if every gate that ran exited 0 and all tests are green. List every command run in gatesRun. On failure, pass=false + first failing gate's trimmed output in failureLog. Do not edit files.`,
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
  const archiveTarget = `openspec/changes/archive/${DATE}-${change}`
  if (archive) {
    archived = await agent(
      [
        `Archive OpenSpec change "${change}". ${await getPromptHooks('on-archive', { change }).then(h => h.join(' '))}`,
        `1. node .claude/workflows/lib/openspec.js list --json — confirm "${change}" is ACTIVE. If not active (already archived?), set archived=false, reason="already archived or not in active list — re-run is a no-op", STOP.`,
        `2. If the directory "${archiveTarget}" already exists, set archived=false, reason="target exists: ${archiveTarget}", STOP.`,
        `3. mkdir -p openspec/changes/archive && mv "openspec/changes/${change}" "${archiveTarget}".`,
        `4. Create or append to openspec/changes/archive/INDEX.md (one row per archived change):`,
        `   | ${DATE} | ${change} | ${mergeResult.mergeSha} | <title from proposal.md> |`,
        `   (Use a markdown table with header row; create the file with the header row if it does not exist.)`,
        `5. Verify: node .claude/workflows/lib/openspec.js list --json — "${change}" must now NOT be active. node .claude/workflows/lib/openspec.js status --change "${change}" returns "not found" (that is success).`,
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
        `Tag ${base} at HEAD with version vX.Y.Z derived from bump="${bump}". Use Bash. ${await getPromptHooks('on-archive', { change }).then(h => h.join(' '))}`,
        `1. If bump is null/empty → noop. tagged=false, tag=null.`,
        `2. Read the current latest tag: git describe --tags --abbrev=0  (returns empty + exit 128 if no tags). Capture priorTag.`,
        `3. Parse priorTag as MAJOR.MINOR.PATCH (default to 0.0.0 if no priorTag). Bump per bump:`,
        `   - patch: MAJOR.MINOR.(PATCH+1)`,
        `   - minor: MAJOR.(MINOR+1).0`,
        `   - major: (MAJOR+1).0.0`,
        `4. Compute newTag = "v\${MAJOR}.\${MINOR}.\${PATCH}".`,
        `5. If newTag already exists (git tag -l newTag), tagged=false, reason="tag already exists", STOP.`,
        `6. git tag -a "\${newTag}" -m "Release \${newTag}\n\nOpenSpec change: ${change}\nMerge: ${mergeResult.mergeSha}\nArchived: ${archived.archivePath || '(no-archive)'}\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
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
          `Open a PR for change "${change}" (title: "${title}"). The feature branch "${branch}" holds the change's per-unit commits and has been merged into LOCAL "${base}" (origin/${base} is NOT updated, so a PR ${branch} → ${base} shows the full change diff). Use Bash (git + gh). ${await getPromptHooks('on-pr', { change }).then(h => h.join(' '))}`,
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
      `Finalize the LOCAL ship of change "${change}" on ${base}. Use Bash (git ONLY — NO gh). ${await getPromptHooks('on-archive', { change }).then(h => h.join(' '))}`,
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
      `   | Skills applied | ${(await getPromptHooks('on-ship-end', { change }).then(h => h.join(', ')))} |`,
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
    skillsApplied: [],
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
    `Finalize change "${change}" (title: "${title}") on branch "${branch}". Use Bash (git + gh). ${await getPromptHooks('on-pr', { change }).then(h => h.join(' '))}`,
    `1. CHANGELOG.md: prepend bullet(s) under "## [Unreleased]" (create the file with the Keep a Changelog header if absent), grouped Added/Changed/Removed per the delta sections, each ending " (${change})". Date context: ${DATE}.`,
    `2. Commit the evidence + changelog as ONE chore commit. The canonical specs are NOT staged here — they were merged on ${base} by the spec PR (/opsx:spec-pr) and the reconcile left them unchanged. Stage ONLY the intended paths (do NOT use git add -A):`,
    `   git add "${evidenceDir}" CHANGELOG.md && git commit -m "chore(${change}): evidence, changelog" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
    `   Set choreCommitted, changelogWritten.`,
    dryRun
      ? `3. DRY RUN: stop after the chore commit — do NOT run git push and do NOT run gh. Set pushed=false, prUrl=null, prExisted=false, note it was a dry run.`
      : `3. Push: git push -u origin "${branch}".`,
    dryRun ? `` : `4. Existing PR? gh pr view "${branch}" --json url,state 2>/dev/null. If OPEN, the push updated it (prExisted=true, use its url). Else gh pr create --base "${base}" --head "${branch}" --title "feat: ${title}" --body <body> with: the proposal's what-and-why; a "## Spec contract" line noting the spec was reviewed & merged via its spec PR (find it: gh pr list --search "spec(${change})" --state merged --json url,title) and that this code PR implements that locked contract; a "## Agent review" section = the code+security pre-review verdict (${review ? review.verdict : 'n/a'}) and its findings (${review && review.findings ? review.findings.length : 0}) by severity (blockers/required first) for the human approver; a "## Evidence" section linking ${evidenceDir} + the gates (${gatesRun.join(', ')}) + coverage (${coverage || 'n/a'}); the per-task commits (${commits.map((c) => 'task ' + c.pair).join(', ')}); the CHANGELOG bullet(s); "Skills applied: ${(await getPromptHooks('on-ship-end', { change }).then(h => h.join(', ')))}"; and a final "🤖 Generated with [Claude Code](https://claude.com/claude-code)". Capture prUrl.`,
    dryRun ? `` : `5. LIFECYCLE (best-effort — NEVER fail the PR on this): extract the code PR number from prUrl and the merged spec PR number from the "spec(${change})" lookup above, then run \`node .claude/workflows/lib/lifecycle.js after-code-pr-opened --change "${change}" --branch "${branch}" --code-pr "<prUrl>" --code-pr-number "<codePrNumber>" --spec-pr-number "<specPrNumber>"\` (optionally pipe the CHANGELOG bullet: \`printf '%s' "<bullet>" | node .claude/workflows/lib/lifecycle.js after-code-pr-opened … --changelog-ref\`). It comments the linked ticket, assigns it to @me when unassigned, sets it in-review, and records the code PR ref. No-ops when unlinked; on any error, log and CONTINUE.`,
    `Return the structured result. If push/gh fails, set pushed=false/prUrl=null and explain in notes — commits stand on the local branch.`,
  ].filter(Boolean).join('\n'),
  { schema: FINALIZE, label: dryRun ? 'finalize (dry-run)' : 'finalize+pr', phase: 'PR', agentType: 'general-purpose' },
)

// ---------------------------------------------------------------- Hooks — after-code-pr-opened agent hook (best-effort)
if (!dryRun && fin && fin.prUrl) {
  phase('Hooks')
  await runAgentHook('after-code-pr-opened', [`- change: ${change}`, `- branch: ${branch}`, `- code PR: ${fin.prUrl}`])
}

// ---------------------------------------------------------------- Report (remote path)
return {
  stage: 'done', ok: true, mode: 'remote', change, title, branch, dryRun,
  commits, // per-task red+green commits
  repairs, gatesRun, coverage, evidenceDir, skillsApplied: [],
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
