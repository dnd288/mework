export const meta = {
  name: 'spec-pr',
  description:
    'Open the SPEC PR for an OpenSpec change whose 6-axis review has APPROVED (stage 2 of the platform workflow: spec review & merge). Preflight (resolve the change, require review/REVIEW.md verdict=APPROVE + `node .claude/workflows/lib/openspec.js validate --strict`, clean tree) → Sync (merge the change delta specs into canonical openspec/specs/ via the openspec-sync-specs skill — the contract becomes law) → Spec PR (branch spec/<slug>, commit ONLY the spec artifacts + the synced canonical specs, push, `gh pr create --base main` a code-free spec PR, STOP — no auto-merge). The change stays ACTIVE for implementation. Honors dryRun (commit on the branch, no push/gh), base, reserveTokens, and worktree. --worktree runs the sync + commit inside an isolated git worktree so the main checkout stays on the base branch. Submodule-scoped: all git ops happen inside platform/; never touches the superproject.',
  phases: [
    { title: 'Preflight', detail: 'resolve change, require REVIEW.md=APPROVE + validate --strict + clean tree' },
    { title: 'Sync',      detail: 'merge delta specs into canonical openspec/specs/ (openspec-sync-specs)' },
    { title: 'Spec PR',   detail: 'branch spec/<slug>, commit spec artifacts + synced specs, push, gh pr create (stops)' },
    { title: 'Hooks',     detail: 'fire after-spec-pr-opened agent hook (best-effort)' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const date = A.date
const dryRun = !!A.dryRun
const base = A.base || 'main'
const reserve = A.reserveTokens || 40000
// --worktree: run sync + commit inside an isolated git worktree so the
// main checkout stays on the base branch. After the worktree agent returns,
// push + PR creation runs in the main checkout.
let worktree = A.worktree === true
if (A.localWorktree === true) { worktree = true }

if (!change || typeof change !== 'string') {
  throw new Error('spec-pr requires args { change, date?, dryRun?, base?, worktree?, localWorktree? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (must start with a letter, kebab-case): ' + change)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)

const branch = `spec/${change}`
const SKILL = (name) => `the \`${name}\` skill (.claude/skills/${name}/SKILL.md)`

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

// ---------------------------------------------------------------- Schemas
const SYNCED = { type: 'object', additionalProperties: false, required: ['synced', 'notes'], properties: { synced: { type: 'boolean' }, notes: { type: 'string' }, canonicalPaths: { type: 'array', items: { type: 'string' } } } }
const FIN = {
  type: 'object', additionalProperties: false,
  required: ['committed', 'pushed', 'prUrl', 'prExisted', 'notes'],
  properties: { committed: { type: 'boolean' }, pushed: { type: 'boolean' }, prUrl: { type: ['string', 'null'] }, prExisted: { type: 'boolean' }, notes: { type: 'string' } },
}
// Schema for the worktree agent result — sync + commit inside the worktree, no push/PR
const WORKTREE_RESULT = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'branch', 'committed', 'canonicalPaths', 'notes'],
  properties: {
    ok: { type: 'boolean' },
    branch: { type: 'string' },
    committed: { type: 'boolean' },
    canonicalPaths: { type: 'array', items: { type: 'string' } },
    notes: { type: 'string' },
    failureLog: { type: 'string' },
  },
}

// ---------------------------------------------------------------- Phase 1: Preflight
phase('Preflight')
const PRE = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'reviewApproved', 'validateOk', 'treeClean', 'changeRoot', 'specPaths', 'title'],
  properties: {
    ok: { type: 'boolean' },
    reason: { type: 'string' },
    reviewApproved: { type: 'boolean' },
    validateOk: { type: 'boolean' },
    treeClean: { type: 'boolean' },
    changeRoot: { type: 'string' },
    specPaths: { type: 'array', items: { type: 'string' } },
    proposalPath: { type: 'string' },
    designPath: { type: 'string' },
    tasksPath: { type: 'string' },
    title: { type: 'string' },
  },
}
const pre = await agent(
  [
    `Preflight the SPEC PR for OpenSpec change "${change}" (base "${base}"). Use Bash. Steps:`,
    `1. TOOLS: test -f .claude/workflows/lib/openspec.js gh git node. gh is required (this opens a remote PR). If missing → ok=false+reason+STOP.`,
    `2. node .claude/workflows/lib/openspec.js status --change "${change}" --json — capture changeRoot, the proposal path, design path, tasks path, and the delta-spec paths (specs/**/*.md under the change), and the change title. If the change does not exist → ok=false, reason="no such change — run /opsx:propose first", STOP.`,
    `3. REVIEW GATE: read "<changeRoot>/review/REVIEW.md". Set reviewApproved=true ONLY if its Verdict is APPROVE. If the file is missing or Verdict is REVISE → reviewApproved=false, ok=false, reason="run /opsx:spec ${change} until it APPROVES before opening the spec PR".`,
    `4. node .claude/workflows/lib/openspec.js validate "${change}" --strict (fallback non-strict). validateOk=true only if it passes. Else ok=false+reason+STOP.`,
    `5. WORKING TREE: git status --porcelain. treeClean=true if empty OR the only changes are under openspec/ (spec authoring in flight). If unrelated code is dirty → ok=false, reason="commit or stash unrelated changes first".`,
    `6. The change MUST have at least one delta spec (specPaths non-empty) — a spec PR with no spec changes is meaningless. If empty → ok=false, reason="no delta specs to merge".`,
    `Set ok=true only when reviewApproved && validateOk && treeClean && specPaths non-empty. Do NOT edit any file.`,
  ].join('\n'),
  { schema: PRE, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.ok) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change, branch }
}
log(`preflight ok: review APPROVED, validate clean, ${pre.specPaths.length} delta spec(s)`)

// ---------------------------------------------------------------- Phase 2: Sync delta → canonical (+ commit on branch)
// Two paths: inline (existing behavior) or worktree (isolated git worktree).
// Both produce the same result: the spec/<slug> branch exists locally with the
// spec commit, ready for push + PR.
let specCommitted = false
let specCanonicalPaths = []
let specNotes = ''

if (worktree) {
  phase('Sync + Commit (in worktree)')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'sync', ok: false, reason: 'budget reserve reached before sync', change, branch }
  }

  const wtResult = await agent(
    [
      `Open the SPEC PR for change "${change}" (title "${pre.title}") inside this isolated git worktree.`,
      `The main checkout stays on "${base}". After you return, the main checkout will push + create the PR.`,
      ``,
      `CONTEXT:`,
      `- Change: ${change} — "${pre.title}"`,
      `- Proposal: ${pre.proposalPath || '(n/a)'}`,
      `- Design: ${pre.designPath || '(n/a)'}`,
      `- Tasks: ${pre.tasksPath}`,
      `- Delta specs: ${pre.specPaths.join(', ')}`,
      `- Base branch: ${base}`,
      `- Branch: ${branch}`,
      ``,
      `STEPS:`,
      ``,
      `1. WORKTREE SETUP:`,
      `   cd into the worktree root (pwd)`,
      `   git switch -c "${branch}" "${base}"  (if the branch already exists, switch to it and reset to ${base}: git switch "${branch}").`,
      ``,
      `2. SYNC: Invoke Skill({ skill: "openspec-sync-specs" }) for change "${change}": merge ADDED/MODIFIED/REMOVED/RENAMED from the change's delta specs (${pre.specPaths.join(', ')}) into openspec/specs/<capability>/spec.md. Idempotent — re-running is a no-op.`,
      ``,
      `3. VALIDATE: node .claude/workflows/lib/openspec.js validate --specs MUST pass after the merge. If not, fix the canonical specs and re-validate. Capture the list of changed canonical spec files (canonicalPaths).`,
      ``,
      `4. STAGE ONLY SPEC ARTIFACTS (do NOT use git add -A):`,
      `   git add "${pre.changeRoot}" openspec/specs/`,
      `   Confirm git status shows no source/app/package code staged — if any non-spec/non-openspec file is staged, unstage it.`,
      ``,
      `5. COMMIT:`,
      `   git commit -s -m "spec(${change}): ${pre.title}" -m "Review & merge the contract before implementation." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`,
      `   Set committed=true.`,
      ``,
      `6. RETURN a structured result matching the WORKTREE_RESULT schema.`,
      `   The branch "${branch}" with the spec commit exists locally. The main checkout will push + create the PR from here.`,
    ].join('\n'),
    {
      schema: WORKTREE_RESULT,
      label: `worktree:${change}`,
      phase: 'Sync + Commit',
      isolation: 'worktree',
      agentType: 'general-purpose',
    },
  )

  if (!wtResult || !wtResult.ok) {
    return {
      stage: 'sync-in-worktree', ok: false,
      reason: wtResult ? `worktree sync failed: ${wtResult.failureLog || '(no details)'}` : 'worktree agent returned null',
      change, branch,
    }
  }

  specCommitted = wtResult.committed
  specCanonicalPaths = wtResult.canonicalPaths || []
  specNotes = wtResult.notes || 'sync + commit in worktree'
  log(`worktree: spec commit on ${branch} (${specCanonicalPaths.length} spec file(s) changed)`)
  log(`worktree cleanup complete — continuing with push + PR in main checkout`)

} else {
  // INLINE PATH (existing behavior): sync + commit in main checkout
  phase('Sync')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'sync', ok: false, reason: 'budget reserve reached before sync', change, branch }
  }

  const synced = await agent(
    [
      `On a NEW branch "${branch}" created from "${base}", merge the delta specs for change "${change}" into the canonical specs. Use Bash + ${SKILL('openspec-sync-specs')}.`,
      `1. git switch -c "${branch}" "${base}"  (if the branch already exists, switch to it and reset to ${base}: git switch "${branch}").`,
      `2. Invoke Skill({ skill: "openspec-sync-specs" }) for change "${change}": merge ADDED/MODIFIED/REMOVED/RENAMED from the change's delta specs (${pre.specPaths.join(', ')}) into openspec/specs/<capability>/spec.md. Idempotent — re-running is a no-op.`,
      `3. node .claude/workflows/lib/openspec.js validate --specs MUST pass after the merge. If not, fix the canonical specs and re-validate. Capture the list of canonical spec files changed (canonicalPaths). Do NOT commit yet.`,
      `Return synced=true if the merge ran (even if idempotent no-op on an already-synced spec).`,
    ].join('\n'),
    { schema: SYNCED, label: 'sync-specs', phase: 'Sync', agentType: 'general-purpose' },
  )
  if (!synced || !synced.synced) {
    return { stage: 'sync', ok: false, reason: synced ? synced.notes : 'sync agent returned null', change, branch }
  }
  specCanonicalPaths = synced.canonicalPaths || []
  log(`sync: delta → canonical (${specCanonicalPaths.length} spec file(s))`)

  // Phase 3: commit (inline — before push + PR in main checkout)
  phase('Spec PR')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'spec-pr', ok: false, reason: 'budget reserve reached before committing the spec PR; synced specs are uncommitted on ' + branch, change, branch }
  }

  const inlineFin = await agent(
    [
      `Commit the spec artifacts for change "${change}" (title "${pre.title}") on branch "${branch}". Use Bash. This PR carries the SPEC ONLY — proposal + delta specs + design + tasks + the synced canonical specs. NO production code.`,
      `1. Stage ONLY the spec artifacts (do NOT use git add -A): git add "${pre.changeRoot}" openspec/specs/ . Confirm git status shows no source/app/package code staged — if any non-spec/non-openspec file is staged, unstage it.`,
      `2. git commit -s -m "spec(${change}): ${pre.title}" -m "Review & merge the contract before implementation." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>". Set committed.`,
      `3. Do NOT push and do NOT create a PR. Return the structured result.`,
    ].join('\n'),
    { schema: { type: 'object', additionalProperties: false, required: ['committed', 'notes'], properties: { committed: { type: 'boolean' }, notes: { type: 'string' } } }, label: 'commit-specs', phase: 'Spec PR', agentType: 'general-purpose' },
  )
  specCommitted = !!(inlineFin && inlineFin.committed)
  specNotes = inlineFin ? inlineFin.notes : 'inline commit'
  log(`inline: spec commit on ${branch} (committed=${specCommitted})`)
}

// ---------------------------------------------------------------- Phase 3: Push + open the SPEC PR (main checkout for both paths)
// This phase runs in the main checkout for both inline and worktree paths.
// The branch exists locally with the spec commit; we push and create the PR.
phase('Spec PR')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'spec-pr', ok: false, reason: 'budget reserve reached before push/PR; spec commit is on ' + branch, change, branch }
}

const fin = await agent(
  [
    `Open the SPEC PR for change "${change}" (title "${pre.title}") on branch "${branch}". Use Bash (git + gh). This PR carries the SPEC ONLY — proposal + delta specs + design + tasks + the synced canonical specs. NO production code.`,
    `The branch "${branch}" already exists locally with the spec commit committed on it (by ${worktree ? 'a worktree agent' : 'the previous phase'}).`,
    dryRun
      ? `DRY RUN: stop here — do NOT push and do NOT run gh. Set pushed=false, prUrl=null, prExisted=false, notes="dry run".`
      : `1. Push: git push -u origin "${branch}".`,
    dryRun ? `` : `2. Read the linked issue from openspec/changes/${change}/github.json (if it exists, it has an "issue.number"). Capture issueNumber for the PR body — if none, leave it empty.`,
    dryRun ? `` : `3. Existing PR? gh pr view "${branch}" --json url,state 2>/dev/null. If OPEN → prExisted=true, reuse its url (the push updated it). Else: a) gh pr create --base "${base}" --head "${branch}" --title "spec(${change}): ${pre.title}" --body <body> where body = the proposal's what-and-why; a "## Spec contract" section listing the touched capabilities (${specCanonicalPaths.join(', ') || 'see openspec/specs/'}); a "## Review checklist" (requirements use SHALL/MUST+≥1 scenario; minimal/testable/consistent); a note "Merging this PR locks the contract; implementation follows via /opsx:ship ${change}"; and "🤖 Generated with [Claude Code](https://claude.com/claude-code)". b) If issueNumber is set, link the new PR to the issue's "Development" section via the GitHub GraphQL API: get the PR node ID (\`gh pr view <prNumber> --json id --jq .id\`), get the issue node ID (\`gh issue view <issueNumber> --json id --jq .id\`), then run \`gh api graphql -f query='mutation($issue:ID!,$pr:ID!){addLinkedIssue(input:{issueId:$issue,linkedPullRequestId:$pr}){issue{id}}}' -f issue=$ISSUE_ID -f pr=$PR_ID\` — this links the PR to the Development section WITHOUT auto-closing the issue (no "Closes"/"Fixes"/"Resolves" keywords used). Capture prUrl.`,
    dryRun ? `` : `4. LIFECYCLE (best-effort — NEVER fail the spec PR on this): extract the PR number from prUrl, then run \`node .claude/workflows/lib/lifecycle.js after-spec-pr-opened --change "${change}" --branch "${branch}" --spec-pr "<prUrl>" --spec-pr-number "<prNumber>"\`. It comments the linked ticket + records the spec PR ref; if the change isn't linked to a ticket it no-ops; if it errors, log and CONTINUE.`,
    `Return the structured result. If push/gh fails, set pushed=false/prUrl=null and explain in notes — the commit stands on ${branch}.`,
  ].filter(Boolean).join('\n'),
  { schema: FIN, label: dryRun ? 'spec-pr (dry-run)' : 'spec-pr', phase: 'Spec PR', agentType: 'general-purpose' },
)

// ---------------------------------------------------------------- Phase 4: Hooks — after-spec-pr-opened agent hook (best-effort)
if (!dryRun && fin && fin.prUrl) {
  phase('Hooks')
  await runAgentHook('after-spec-pr-opened', [`- change: ${change}`, `- branch: ${branch}`, `- spec PR: ${fin.prUrl}`])
}

return {
  stage: 'done', ok: true, change, title: pre.title, branch, base, dryRun,
  specsSynced: specCommitted,
  committed: !!(fin && fin.committed),
  pushed: !!(fin && fin.pushed),
  prExisted: !!(fin && fin.prExisted),
  prUrl: fin ? fin.prUrl : null,
  notes: fin ? fin.notes : 'spec-pr agent returned null',
  nextStep: dryRun
    ? `Dry run complete on ${branch}: spec artifacts + synced canonical specs committed. Inspect git log --stat, then re-run without dryRun to push + open the spec PR.`
    : `Spec PR ${fin && fin.prExisted ? 'updated' : 'opened'}. After a human reviews & MERGES it (canonical specs land on ${base}), run /opsx:ship ${change} to implement against the locked contract.`,
}
