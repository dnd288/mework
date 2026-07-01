export const meta = {
  name: 'merge-pr-spec',
  description:
    'Merge a SPEC PR — the contract branch (spec/<change>). Preflights, updates title/body, merges via GitHub API, then fires the after-spec-pr-merged lifecycle event (ticket comment + board update). No archive or changelog — spec PRs are contract-only.',
  phases: [
    { title: 'Preflight', detail: 'find the spec PR, check status (OPEN, not draft, no conflicts)' },
    { title: 'Prepare',   detail: 'update PR title/body if requested' },
    { title: 'Merge',     detail: 'merge via GitHub API, delete branch' },
    { title: 'Hooks',     detail: 'fire after-spec-pr-merged lifecycle (ticket comment + board update + shell hook)' },
    { title: 'Summary',   detail: 'report merge SHA, PR URL, lifecycle status' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
let change = A.change             // OpenSpec change slug (auto-detected from branch if omitted)
const pr = A.pr                   // optional: explicit PR URL/number
const dryRun = !!A.dryRun
const strategy = A.strategy || 'squash'
const title = A.title || ''
const body = A.body || ''
let repo = A.repo || ''
const base = A.base || 'main'
const reserve = A.reserveTokens || 20000

if (!change && !pr) {
  throw new Error('merge-pr-spec requires either { change } or { pr }.')
}
if (change && !/^[a-z][a-z0-9-]*$/.test(change)) {
  throw new Error('Unsafe change name: ' + change)
}
if (!['squash', 'merge', 'rebase'].includes(strategy)) {
  throw new Error('strategy must be squash|merge|rebase')
}

const specBranch = change ? `spec/${change}` : null
let prNumber, prUrl, owner

// ---------------------------------------------------------------- Phase 1: Preflight
phase('Preflight')
const PRE = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'prNumber', 'prUrl', 'owner', 'repo', 'state', 'isDraft', 'mergeable', 'headSha', 'headRefName', 'baseRef', 'title', 'body'],
  properties: {
    ok: { type: 'boolean' }, reason: { type: 'string' },
    prNumber: { type: 'integer' }, prUrl: { type: 'string' },
    owner: { type: 'string' }, repo: { type: 'string' },
    state: { type: 'string' }, isDraft: { type: 'boolean' },
    mergeable: { type: ['string', 'null'] },
    headSha: { type: 'string' }, headRefName: { type: 'string' }, baseRef: { type: 'string' },
    title: { type: 'string' }, body: { type: 'string' },
    isSpecBranch: { type: 'boolean' },
  },
}
const pre = await agent(
  [
    `Preflight for merge-pr-spec. Use Bash (gh, git, node). Steps:`,
    `1. TOOLS: command -v gh git node; gh auth status → ok=false+reason+STOP if missing.`,
    repo ? `   NOTE: Using cross-repo mode — all gh commands need --repo "${repo}".` : '',
    specBranch
      ? `2. Find the open PR for branch "${specBranch}": gh pr view "${specBranch}" ${repo ? `--repo "${repo}"` : ''} --json number,url,state,isDraft,mergeable,headRefName,baseRefName,headRefOid,title,body. If none OPEN → ok=false+reason+STOP.`
      : `2. Parse PR# from "${pr}" (URL or number). If full URL, extract owner/repo/number. Then: gh pr view <number> --json number,url,state,isDraft,mergeable,headRefName,baseRefName,headRefOid,title,body.`,
    `3. Parse owner/repo: from repo arg, gh repo view, or URL.`,
    `4. Extract: prNumber, prUrl, state, isDraft, mergeable, headSha(headRefOid), headRefName, baseRef, title, body.`,
    `5. VALIDATE:`,
    `   - state must be "OPEN" → ok=false if not`,
    `   - isDraft must be false → ok=false + reason "PR is a draft"`,
    `   - mergeable should not be "CONFLICTING" → ok=false + reason "PR has merge conflicts"`,
    `   - headRefName MUST start with "spec/" → set isSpecBranch=true. If it doesn't, ok=false + reason "Branch is not a spec/ branch — use merge-pr-code instead".`,
    `Return ok, reason, prNumber, prUrl, owner, repo, state, isDraft, mergeable, headSha, headRefName, baseRef, title, body, isSpecBranch.`,
  ].join('\n'),
  { schema: PRE, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.ok) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change, pr }
}

prNumber = pre.prNumber
prUrl = pre.prUrl
owner = pre.owner
repo = pre.repo
log(`SPEC PR #${prNumber}: "${pre.title}" — ${pre.state}, draft=${pre.isDraft}, mergeable=${pre.mergeable}`)

// Auto-detect change from spec/<slug>
const SPEC_BRANCH_RE = /^spec\/([a-z][a-z0-9-]*)$/
if (!change && pre.headRefName) {
  const m = pre.headRefName.match(SPEC_BRANCH_RE)
  if (m) {
    change = m[1]
    log(`Auto-detected change "${change}" from spec branch "${pre.headRefName}"`)
  }
}

// ---------------------------------------------------------------- Phase 2: Prepare
phase('Prepare')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'prepare', ok: false, reason: 'budget reserve reached before prepare', ...pre }
}

const finalTitle = title || pre.title
let finalBody = body || pre.body
const needsUpdate = finalTitle !== pre.title || finalBody !== pre.body

if (needsUpdate) {
  if (dryRun) {
    log(`Would update PR title/body: title="${finalTitle}"`)
  } else {
    const updated = await agent(
      [
        `Update PR #${prNumber} in ${owner}/${repo}.`,
        `New title: "${finalTitle}"`,
        `Run: gh pr edit ${prNumber} --title "${finalTitle.replace(/"/g, '\\"')}" --body "${finalBody.replace(/"/g, '\\"')}" ${repo ? `--repo "${owner}/${repo}"` : ''}`,
        `Return { ok: true } on success, or { ok: false, error: "..." }.`,
      ].join('\n'),
      {
        label: 'update-pr',
        phase: 'Prepare',
        schema: { type: 'object', additionalProperties: false, required: ['ok'], properties: { ok: { type: 'boolean' }, error: { type: 'string' } } },
        agentType: 'general-purpose',
      },
    )
    if (!updated || !updated.ok) {
      return { stage: 'prepare', ok: false, reason: 'Failed to update PR: ' + (updated ? updated.error : 'unknown'), ...pre }
    }
    log(`PR #${prNumber} title/body updated`)
  }
} else {
  log('PR title/body unchanged')
}

if (dryRun) {
  return {
    stage: 'dry-run', ok: true, prUrl, prNumber, owner, repo, headSha: pre.headSha,
    mergeStrategy: strategy,
    wouldMerge: true,
    wouldUpdatePR: needsUpdate,
    nextStep: `Dry run complete for spec PR #${prNumber}. Re-run without --dryRun to merge.`,
  }
}

// ---------------------------------------------------------------- Phase 3: Merge
phase('Merge')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'merge', ok: false, reason: 'budget reserve reached before merge', prUrl, prNumber }
}

const mergeResult = await agent(
  [
    `Merge spec PR #${prNumber} in ${owner}/${repo} using ${strategy} strategy.`,
    `Head SHA: ${pre.headSha}`,
    `This is a SPEC PR (contract only — no archive needed).`,
    repo ? `Repo: ${owner}/${repo}` : '',
    ``,
    `1. Merge: gh pr merge ${prNumber} --${strategy} --delete-branch ${repo ? `--repo "${owner}/${repo}"` : ''}`,
    `   If --squash: --subject "${finalTitle.replace(/"/g, '\\"')}"`,
    ``,
    `2. Capture result: gh pr view ${prNumber} --json mergedAt,mergeCommit,state ${repo ? `--repo "${owner}/${repo}"` : ''}`,
    `   Extract mergeCommit.oid.`,
    ``,
    `Return { merged: true, mergeSha, state }. On failure: { merged: false, error: "..." }.`,
  ].join('\n'),
  {
    label: 'merge',
    phase: 'Merge',
    schema: {
      type: 'object', additionalProperties: false,
      required: ['merged'],
      properties: { merged: { type: 'boolean' }, mergeSha: { type: 'string' }, state: { type: 'string' }, error: { type: 'string' } },
    },
    agentType: 'general-purpose',
  },
)

if (!mergeResult || !mergeResult.merged) {
  phase('Summary')
  return {
    stage: 'merge-failed', ok: false, prUrl, prNumber,
    error: mergeResult ? mergeResult.error : 'merge agent returned null',
    nextStep: `Merge failed for spec PR #${prNumber}. Resolve and retry.`,
  }
}

// ---------------------------------------------------------------- Phase 4: Hooks — after-spec-pr-merged lifecycle
phase('Hooks')
let hooks = { ran: false, skipped: change ? '' : 'not-an-openspec-change' }
if (change && !(budget && budget.total && budget.remaining() < reserve)) {
  const lifecycleCmd =
    `node .claude/workflows/lib/lifecycle.js after-spec-pr-merged --change "${change}" ` +
    `--merged-sha "${mergeResult.mergeSha}" --spec-pr-number ${prNumber} --json`
  const HOOKS = {
    type: 'object', additionalProperties: false,
    required: ['ran'],
    properties: {
      ran: { type: 'boolean' },
      lifecycle: {
        type: 'object', additionalProperties: true,
        properties: {
          commented: { type: 'boolean' }, statusSet: { type: 'boolean' },
          projectUpdated: { type: 'boolean' },
          skipped: { type: 'string' }, shellHookRan: { type: 'boolean' },
        },
      },
      errors: { type: 'array', items: { type: 'string' } },
    },
  }
  const hookResult = await agent(
    [
      `AGENT HOOK RUNNER for merge-pr-spec. Spec PR #${prNumber} has been merged.`,
      `Fire the after-spec-pr-merged lifecycle for change "${change}". BEST-EFFORT.`,
      ``,
      `Context: change=${change}, merged PR #${prNumber} (${prUrl}), merge SHA=${mergeResult.mergeSha}`,
      ``,
      `STEP 1 — deterministic lifecycle:`,
      `  Run: ${lifecycleCmd}`,
      `  Parse JSON stdout. Map: lifecycle.commented <- did.commented, lifecycle.statusSet <- did.statusSet,`,
      `  lifecycle.projectUpdated <- did.projectUpdated, lifecycle.shellHookRan <- did.hookRan,`,
      `  lifecycle.skipped <- top-level "skipped" (e.g. "no-link" = normal, no ticket).`,
      ``,
      `Return { ran:true, lifecycle:{...}, errors:[...] }.`,
    ].join('\n'),
    { schema: HOOKS, label: 'hook-runner', phase: 'Hooks', agentType: 'general-purpose' },
  )
  hooks = hookResult || { ran: false, errors: ['hook-runner agent returned null'] }
  const lc = hooks.lifecycle || {}
  if (lc.commented) log(`Lifecycle after-spec-pr-merged: commented ticket${lc.projectUpdated ? ' + board updated' : ''}`)
  else if (lc.skipped) log(`Lifecycle: ${lc.skipped} — no ticket linked`)
  if (hooks.errors && hooks.errors.length) log(`Hook errors (non-fatal): ${hooks.errors.join('; ')}`)
} else if (!change) {
  log('Hooks: no change slug — no lifecycle event')
}

// ---------------------------------------------------------------- Phase 5: Summary
phase('Summary')
const lc = hooks.lifecycle || {}
const notes = []
if (change) {
  if (lc.commented) notes.push(`- 🪝 Lifecycle after-spec-pr-merged fired${lc.statusSet ? ' + status advanced' : ''}${lc.projectUpdated ? ' + board updated' : ''}.`)
  else if (lc.skipped === 'no-link') notes.push(`- 🪝 Change not linked to a GitHub issue — no comment. Link via /opsx:propose-gh to enable lifecycle.`)
  else if (lc.skipped) notes.push(`- 🪝 Lifecycle skipped (${lc.skipped}).`)
}
if (hooks.errors && hooks.errors.length) notes.push(`- ⚠️ Hook errors (non-fatal): ${hooks.errors.join('; ')}`)

return {
  stage: 'done', ok: true,
  prUrl, prNumber,
  mergeSha: mergeResult.mergeSha,
  mergeStrategy: strategy,
  titleUpdated: needsUpdate,
  lifecycleEvent: 'after-spec-pr-merged',
  hooks,
  prTitle: finalTitle,
  nextStep: [
    `✅ Merged SPEC PR #${prNumber} (${strategy}) — commit \`${mergeResult.mergeSha}\``,
    `   PR: ${prUrl}`,
    `   The spec contract is now locked on ${base}.`,
    ``,
    `**Next:** /opsx:ship-plan ${change} → /opsx:ship-code to implement.`,
    ...notes,
  ].filter(Boolean).join('\n'),
}
