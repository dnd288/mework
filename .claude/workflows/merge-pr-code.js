export const meta = {
  name: 'merge-pr-code',
  description:
    'Merge a CODE PR — the implementation branch (feat/<change>). Preflights (incl. changelog check), prepares, archives the change on the branch, merges via GitHub, closes linked issues, then fires the after-code-pr-merged lifecycle (ticket comment + board update + close).',
  phases: [
    { title: 'Preflight', detail: 'find the code PR, check status, detect linked issues, check changelog' },
    { title: 'Prepare',   detail: 'update PR title/body/closing keywords' },
    { title: 'Archive',   detail: 'archive the OpenSpec change as a commit on the branch before merge' },
    { title: 'Merge',     detail: 'merge via GitHub API, close linked issues' },
    { title: 'Hooks',     detail: 'fire after-code-pr-merged lifecycle (ticket comment, board → Done, close)' },
    { title: 'Summary',   detail: 'report merge SHA, closed issues, archive + lifecycle status' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
let change = A.change
const pr = A.pr
const dryRun = !!A.dryRun
const strategy = A.strategy || 'squash'
const title = A.title || ''
const body = A.body || ''
let repo = A.repo || ''
const closes = A.closes || ''
const base = A.base || 'main'
const skipArchive = !!A.skipArchive
const reserve = A.reserveTokens || 20000

if (!change && !pr) {
  throw new Error('merge-pr-code requires either { change } or { pr }.')
}
if (change && !/^[a-z][a-z0-9-]*$/.test(change)) {
  throw new Error('Unsafe change name: ' + change)
}
if (!['squash', 'merge', 'rebase'].includes(strategy)) {
  throw new Error('strategy must be squash|merge|rebase')
}

const codeBranch = change ? `feat/${change}` : null
let prNumber, prUrl, owner

// ---------------------------------------------------------------- Phase 1: Preflight
phase('Preflight')
const PRE = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'prNumber', 'prUrl', 'owner', 'repo', 'state', 'isDraft', 'mergeable', 'headSha', 'headRefName', 'baseRef', 'title', 'body', 'closingIssues', 'changelogEntry', 'isCodeBranch'],
  properties: {
    ok: { type: 'boolean' }, reason: { type: 'string' },
    prNumber: { type: 'integer' }, prUrl: { type: 'string' },
    owner: { type: 'string' }, repo: { type: 'string' },
    state: { type: 'string' }, isDraft: { type: 'boolean' },
    mergeable: { type: ['string', 'null'] },
    headSha: { type: 'string' }, headRefName: { type: 'string' }, baseRef: { type: 'string' },
    title: { type: 'string' }, body: { type: 'string' },
    closingIssues: { type: 'array', items: { type: 'integer' } },
    changelogEntry: { type: 'string' },
    isCodeBranch: { type: 'boolean' },
  },
}
const pre = await agent(
  [
    `Preflight for merge-pr-code. Use Bash (gh, git, node).`,
    `1. TOOLS: command -v gh git node; gh auth status → ok=false+reason+STOP if missing.`,
    codeBranch
      ? `2. Find the open PR for branch "${codeBranch}": gh pr view "${codeBranch}" --json number,url,state,isDraft,mergeable,headRefName,baseRefName,headRefOid,title,body,closingIssuesReferences. If none OPEN → ok=false+reason+STOP.`
      : `2. Parse PR# from "${pr}" (URL or number). gh pr view <number> --json number,url,state,isDraft,mergeable,headRefName,baseRefName,headRefOid,title,body,closingIssuesReferences.`,
    `3. Parse owner/repo: from repo arg, gh repo view, or URL.`,
    `4. Extract: prNumber, prUrl, state, isDraft, mergeable, headSha(headRefOid), headRefName, baseRef, title, body, closingIssues[]. If closingIssuesReferences empty, check body for "Closes #N" / "Fixes #N".`,
    `5. Changelog: git fetch origin "${codeBranch}" 2>/dev/null; git diff origin/main...origin/"${codeBranch}" -- CHANGELOG.md | head -80. Capture changelogEntry (added lines) or "". Skip if cross-repo.`,
    `6. VALIDATE:`,
    `   - state must be "OPEN"`,
    `   - isDraft must be false`,
    `   - mergeable != "CONFLICTING"`,
    `   - headRefName MUST start with "feat/" → set isCodeBranch=true. If not, ok=false + reason "Not a feat/ branch — use merge-pr-spec instead".`,
    `Return ok, reason, prNumber, prUrl, owner, repo, state, isDraft, mergeable, headSha, headRefName, baseRef, title, body, closingIssues[], changelogEntry, isCodeBranch.`,
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
log(`CODE PR #${prNumber}: "${pre.title}" — ${pre.state}, draft=${pre.isDraft}, mergeable=${pre.mergeable}`)
if (pre.closingIssues.length) log(`Linked issues: #${pre.closingIssues.join(', #')}`)
if (pre.changelogEntry) log(`Changelog entry found (${pre.changelogEntry.length} chars)`)
else if (change) log(`No CHANGELOG.md entry found for "${change}"`)

const FEAT_BRANCH_RE = /^feat\/([a-z][a-z0-9-]*)$/
if (!change && pre.headRefName) {
  const m = pre.headRefName.match(FEAT_BRANCH_RE)
  if (m) { change = m[1]; log(`Auto-detected change "${change}" from branch`) }
}

// ---------------------------------------------------------------- Phase 2: Prepare
phase('Prepare')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'prepare', ok: false, reason: 'budget reserve reached' }
}

const finalTitle = title || pre.title
let finalBody = body || pre.body
if (closes) {
  const closeNums = closes.split(',').map((s) => s.trim().replace(/^#/, '')).filter(Boolean)
  const closeLines = closeNums.map((n) => `Closes #${n}`).join('\n')
  if (!finalBody.includes(closeLines)) {
    finalBody = finalBody ? finalBody + '\n\n' + closeLines : closeLines
  }
}

const needsUpdate = finalTitle !== pre.title || finalBody !== pre.body
if (needsUpdate) {
  if (dryRun) {
    log(`Would update PR title/body`)
  } else {
    const updated = await agent(
      [
        `Update PR #${prNumber} in ${owner}/${repo}.`,
        `Title: "${finalTitle}"`,
        `Run: gh pr edit ${prNumber} --title "${finalTitle.replace(/"/g, '\\"')}" --body "${finalBody.replace(/"/g, '\\"')}" ${repo ? `--repo "${owner}/${repo}"` : ''}`,
        `Return { ok: true } on success.`,
      ].join('\n'),
      {
        label: 'update-pr', phase: 'Prepare',
        schema: { type: 'object', additionalProperties: false, required: ['ok'], properties: { ok: { type: 'boolean' }, error: { type: 'string' } } },
        agentType: 'general-purpose',
      },
    )
    if (!updated || !updated.ok) {
      return { stage: 'prepare', ok: false, reason: 'Failed to update PR' + (updated ? updated.error : ''), ...pre }
    }
    log(`PR #${prNumber} updated`)
  }
} else {
  log('PR title/body unchanged')
}

if (dryRun) {
  return {
    stage: 'dry-run', ok: true, prUrl, prNumber, owner, repo, headSha: pre.headSha,
    mergeStrategy: strategy, closingIssues: pre.closingIssues,
    wouldMerge: true, wouldArchive: !skipArchive && !!change,
    nextStep: `Dry run complete for code PR #${prNumber}.`,
  }
}

// ---------------------------------------------------------------- Phase 3: Archive (on branch, BEFORE merge)
phase('Archive')
let archived = false, archiveReason = '', archiveSha = ''
if (!change || skipArchive) {
  archiveReason = skipArchive ? 'skipped via --skip-archive' : 'not an OpenSpec change'
} else if (budget && budget.total && budget.remaining() < reserve) {
  archiveReason = 'budget reserve reached'
} else {
  const arch = await agent(
    [
      `Archive change "${change}" on the branch BEFORE merging PR #${prNumber}.`,
      `Branch: "${codeBranch}"`,
      `1. git switch "${codeBranch}" 2>/dev/null || git switch -c "${codeBranch}" origin/"${codeBranch}"`,
      `2. git pull origin "${codeBranch}" --ff-only 2>/dev/null || true`,
      `3. Archive: node .claude/workflows/lib/openspec.js archive "${change}" -y 2>/dev/null || { TODAY=$(date +%Y-%m-%d); mkdir -p openspec/changes/archive; mv openspec/changes/"${change}" "openspec/changes/archive/$\{TODAY}-${change}"; }`,
      `4. git add openspec/changes/archive/ && git commit -m "chore(${change}): archive completed change"`,
      `5. git push origin "${codeBranch}"`,
      `Return { archived: true, commitSha } on success.`,
    ].join('\n'),
    {
      label: 'archive', phase: 'Archive',
      schema: { type: 'object', additionalProperties: false, required: ['archived'], properties: { archived: { type: 'boolean' }, reason: { type: 'string' }, commitSha: { type: 'string' } } },
      agentType: 'general-purpose',
    },
  )
  archived = !!(arch && arch.archived)
  archiveSha = arch ? (arch.commitSha || '') : ''
  archiveReason = arch ? (arch.reason || '') : ''
  if (archived) log(`Archived "${change}" on branch (${archiveSha.slice(0, 7)})`)
}

// ---------------------------------------------------------------- Phase 4: Merge + close issues
phase('Merge')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'merge', ok: false, reason: 'budget reserve reached', prUrl, prNumber }
}

const mergeResult = await agent(
  [
    `Merge code PR #${prNumber} in ${owner}/${repo} using ${strategy}.`,
    `Head SHA: ${pre.headSha}`,
    archived ? `Archive commit ${archiveSha.slice(0, 7)} on branch, included in merge.` : '',
    ``,
    `1. gh pr merge ${prNumber} --${strategy} --delete-branch ${repo ? `--repo "${owner}/${repo}"` : ''}`,
    `2. Capture: gh pr view ${prNumber} --json mergedAt,mergeCommit,state`,
    `3. For linked issues #${pre.closingIssues.join(', #') || '(none)'}:`,
    `   gh issue close <num> --comment "Closed by merge of PR #${prNumber}" ${repo ? `--repo "${owner}/${repo}"` : ''}`,
    `4. Parse PR body for "Closes #N" / "Fixes #N" if closingIssues was empty.`,
    `Return { merged: true, mergeSha, state, issuesClosed: [numbers] } or { merged: false, error }`,
  ].join('\n'),
  {
    label: 'merge', phase: 'Merge',
    schema: {
      type: 'object', additionalProperties: false, required: ['merged'],
      properties: { merged: { type: 'boolean' }, mergeSha: { type: 'string' }, state: { type: 'string' }, issuesClosed: { type: 'array', items: { type: 'integer' } }, error: { type: 'string' } },
    },
    agentType: 'general-purpose',
  },
)

if (!mergeResult || !mergeResult.merged) {
  phase('Summary')
  return { stage: 'merge-failed', ok: false, prUrl, prNumber, error: mergeResult ? mergeResult.error : 'merge agent returned null' }
}

// ---------------------------------------------------------------- Phase 5: Hooks — after-code-pr-merged lifecycle
phase('Hooks')
let hooks = { ran: false, skipped: change ? '' : 'not-an-openspec-change' }
if (change && !(budget && budget.total && budget.remaining() < reserve)) {
  const archiveGlob = `$(ls -d openspec/changes/archive/*-${change} 2>/dev/null | head -1)`
  const lifecycleCmd =
    `node .claude/workflows/lib/lifecycle.js after-code-pr-merged --change "${change}" ` +
    `--merged-sha "${mergeResult.mergeSha}" --code-pr-number ${prNumber}` +
    `${archived ? ` --archive-path "${archiveGlob}"` : ''} --json`
  const HOOKS = {
    type: 'object', additionalProperties: false, required: ['ran'],
    properties: {
      ran: { type: 'boolean' },
      lifecycle: { type: 'object', additionalProperties: true, properties: { commented: { type: 'boolean' }, statusSet: { type: 'boolean' }, projectUpdated: { type: 'boolean' }, skipped: { type: 'string' }, shellHookRan: { type: 'boolean' } } },
      errors: { type: 'array', items: { type: 'string' } },
    },
  }
  const hookResult = await agent(
    [
      `AGENT HOOK RUNNER for merge-pr-code. Code PR #${prNumber} merged.`,
      `Fire after-code-pr-merged for change "${change}". BEST-EFFORT.`,
      `Context: change=${change}, PR #${prNumber} (${prUrl}), merge SHA=${mergeResult.mergeSha}`,
      `Linked issues closed: #${(mergeResult.issuesClosed || []).join(', ')}`,
      ``,
      `STEP 1 — deterministic lifecycle:`,
      `  Run: ${lifecycleCmd}`,
      `  Parse JSON stdout. Map: lifecycle.commented <- did.commented, lifecycle.statusSet <- did.statusSet,`,
      `  lifecycle.projectUpdated <- did.projectUpdated, lifecycle.shellHookRan <- did.hookRan,`,
      `  lifecycle.skipped <- top-level "skipped" (e.g. "no-link" = normal).`,
      ``,
      `Return { ran:true, lifecycle:{...}, errors:[...] }.`,
    ].join('\n'),
    { schema: HOOKS, label: 'hook-runner', phase: 'Hooks', agentType: 'general-purpose' },
  )
  hooks = hookResult || { ran: false, errors: ['hook-runner returned null'] }
  const lc = hooks.lifecycle || {}
  if (lc.commented) log(`Lifecycle after-code-pr-merged: commented ticket${lc.projectUpdated ? ' + board → Done' : ''}${lc.statusSet ? ' + closed' : ''}`)
  else if (lc.skipped) log(`Lifecycle: ${lc.skipped}`)
  if (hooks.errors && hooks.errors.length) log(`Hook errors: ${hooks.errors.join('; ')}`)
}

// ---------------------------------------------------------------- Phase 6: Summary
phase('Summary')
const issuesClosed = mergeResult.issuesClosed || []
const lc = hooks.lifecycle || {}
const notes = []
if (archived) notes.push(`- 📦 Change archived: chore(${change}): archive (${archiveSha.slice(0, 7)})`)
if (change) {
  if (lc.commented) notes.push(`- 🪝 Lifecycle fired — ticket commented + board → Done + closed${lc.projectUpdated ? ' + board updated' : ''}.`)
  else if (lc.skipped === 'no-link') notes.push(`- 🪝 No linked issue (no github.json) — link via /opsx:propose-gh.`)
  else if (lc.skipped) notes.push(`- 🪝 Lifecycle skipped (${lc.skipped}).`)
}
if (hooks.errors && hooks.errors.length) notes.push(`- ⚠️ Hook errors (non-fatal): ${hooks.errors.join('; ')}`)
if (!pre.changelogEntry && change) notes.push(`- 📝 Consider adding a CHANGELOG entry if missing.`)

return {
  stage: 'done', ok: true,
  prUrl, prNumber,
  mergeSha: mergeResult.mergeSha,
  mergeStrategy: strategy,
  titleUpdated: needsUpdate,
  issuesClosed,
  archived, archiveReason, archiveSha,
  changelogPresent: !!pre.changelogEntry,
  lifecycleEvent: 'after-code-pr-merged',
  hooks,
  nextStep: [
    `✅ Merged CODE PR #${prNumber} (${strategy}) — commit \`${mergeResult.mergeSha}\``,
    issuesClosed.length ? `   Closed issue(s): #${issuesClosed.join(', #')}` : '   No linked issues.',
    `   PR: ${prUrl}`,
    ...notes,
  ].filter(Boolean).join('\n'),
}
