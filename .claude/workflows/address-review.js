export const meta = {
  name: 'address-review',
  description:
    'Address human review feedback on the code PR for an OpenSpec change (stage 5 of the platform workflow: PR review). Preflight (find the open feat/<change> PR via gh, collect unresolved review threads/comments) → Address (for each actionable comment: edit code on the branch, re-run the resolver-selected gates on the touched files, make a focused commit) → Push & reply (push the branch, reply to/resolve each thread, leave the rest for the human) → STOP for human re-review. Never merges. Honors dryRun (list what it would address; no edits/push), base, reserveTokens. Submodule-scoped: all git/gh ops inside platform/.',
  phases: [
    { title: 'Preflight', detail: 'find the open code PR + collect unresolved review threads' },
    { title: 'Address',   detail: 'per actionable comment: fix on branch + re-run touched-file gates + focused commit' },
    { title: 'Push & reply', detail: 'push, reply to/resolve threads, stop for human re-review' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const dryRun = !!A.dryRun
const base = A.base || 'main'
const reserve = A.reserveTokens || 40000

if (!change || typeof change !== 'string') {
  throw new Error('address-review requires args { change, dryRun?, base? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (must start with a letter, kebab-case): ' + change)

const branch = `feat/${change}`
// Skills are injected dynamically via prompt hooks — extensions/agent-skills/Hooks/on-address-review.prompt.md
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

// ---------------------------------------------------------------- Phase 1: Preflight — find the PR + unresolved threads
phase('Preflight')
const PRE = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'prUrl', 'prNumber', 'threads'],
  properties: {
    ok: { type: 'boolean' },
    reason: { type: 'string' },
    prUrl: { type: ['string', 'null'] },
    prNumber: { type: ['integer', 'null'] },
    threads: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        required: ['id', 'path', 'line', 'author', 'body', 'actionable'],
        properties: {
          id: { type: 'string' }, path: { type: 'string' }, line: { type: ['integer', 'null'] },
          author: { type: 'string' }, body: { type: 'string' }, actionable: { type: 'boolean' },
        },
      },
    },
  },
}
const pre = await agent(
  [
    `Preflight /opsx:address-review for OpenSpec change "${change}" (branch "${branch}", base "${base}"). Use Bash (git + gh). Steps:`,
    `1. TOOLS: command -v gh git node. If gh is missing or not authenticated (gh auth status) → ok=false+reason+STOP.`,
    `2. Find the open PR for the branch: gh pr view "${branch}" --json number,url,state,reviewDecision. If none OPEN → ok=false, reason="no open PR for ${branch} — run /opsx:ship ${change} first". Capture prNumber, prUrl.`,
    `3. Switch to the branch: git switch "${branch}"; git pull --ff-only 2>/dev/null || true.`,
    `4. Collect UNRESOLVED review threads + review comments: use \`gh api\` on the repo's pulls/<number>/comments and the GraphQL reviewThreads(isResolved:false) for the PR. For EACH, capture {id, path, line, author, body} and judge actionable=true if it requests a concrete code/spec/test change (false for praise/questions/nits you should leave for the human).`,
    `Set ok=true if a PR was found (even with zero threads). Do NOT edit anything here.`,
  ].join('\n'),
  { schema: PRE, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.ok) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change, branch }
}
const actionable = (pre.threads || []).filter((t) => t.actionable)
log(`PR #${pre.prNumber}: ${(pre.threads || []).length} unresolved thread(s), ${actionable.length} actionable`)
if (!actionable.length) {
  return { stage: 'done', ok: true, change, branch, prUrl: pre.prUrl, addressed: 0,
    nextStep: `No actionable review comments on PR #${pre.prNumber}. If the human approved, merge the PR; then run /opsx:archive ${change}.` }
}
if (dryRun) {
  return { stage: 'dry-run', ok: true, change, branch, prUrl: pre.prUrl,
    wouldAddress: actionable.map((t) => `${t.path}:${t.line || '?'} — ${t.body.slice(0, 80)}`),
    nextStep: `Dry run: would address ${actionable.length} comment(s). Re-run without dryRun to fix + push + reply.` }
}

// ---------------------------------------------------------------- Phase 2: Address each actionable thread
phase('Address')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'address', ok: false, reason: 'budget reserve reached before addressing review', change, branch, prUrl: pre.prUrl }
}
const RESULT = {
  type: 'object', additionalProperties: false,
  required: ['addressed', 'commits', 'verifyOk', 'notes'],
  properties: {
    addressed: { type: 'array', items: { type: 'object', additionalProperties: false, required: ['id', 'path', 'action'], properties: { id: { type: 'string' }, path: { type: 'string' }, action: { type: 'string' } } } },
    commits: { type: 'array', items: { type: 'string' } },
    verifyOk: { type: 'boolean' },
    notes: { type: 'string' },
  },
}
const res = await agent(
  [
    `Address the ${actionable.length} actionable review comment(s) on PR #${pre.prNumber} for change "${change}", branch "${branch}". Use Bash + Edit. ${await getPromptHooks('on-address-review', { change }).then(h => h.join(' '))}.`,
    `Comments:`,
    ...actionable.map((t, i) => `  ${i + 1}. [${t.id}] ${t.path}:${t.line || '?'} (${t.author}): ${t.body}`),
    `For EACH actionable comment: make the MINIMAL code/test/spec change that resolves it on ${branch}. If a comment requests a behavior change that contradicts the merged spec contract, do NOT silently change it — note it in notes and leave that thread for the human (the contract changes via /opsx:spec + /opsx:spec-pr, not here).`,
    `After the edits, RE-VERIFY only the touched packages: git diff --name-only ${base}...HEAD | node .claude/workflows/lib/gate-resolver.js --stdin → run every printed gate; set verifyOk=true only if all pass (DB-dependent pytest skips without TEST_DATABASE_URL = not a failure).`,
    `Make focused commit(s): git add <only the touched paths> && git commit -s -m "fix(${change}): address review — <thread summary>" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>". Capture commit shas. Do NOT push here.`,
    `Return addressed (each comment id + path + one-line action), commits, verifyOk, notes (incl. any threads left for the human).`,
  ].join('\n'),
  { schema: RESULT, label: 'address', phase: 'Address', agentType: 'general-purpose' },
)
if (!res) {
  return { stage: 'address', ok: false, reason: 'address agent returned null', change, branch, prUrl: pre.prUrl }
}
if (!res.verifyOk) {
  return { stage: 'address', ok: false, reason: 'gates failed after addressing review — fix on ' + branch + ' and re-run; nothing pushed', change, branch, prUrl: pre.prUrl, notes: res.notes }
}
log(`addressed ${res.addressed.length}; ${res.commits.length} commit(s); gates green`)

// ---------------------------------------------------------------- Phase 3: Push + reply/resolve
phase('Push & reply')
const FIN = {
  type: 'object', additionalProperties: false,
  required: ['pushed', 'replied', 'resolved', 'notes'],
  properties: { pushed: { type: 'boolean' }, replied: { type: 'integer' }, resolved: { type: 'integer' }, notes: { type: 'string' } },
}
const fin = await agent(
  [
    `Push the review fixes and respond on PR #${pre.prNumber} for change "${change}", branch "${branch}". Use Bash (git + gh).`,
    `1. git push origin "${branch}".`,
    `2. For each addressed thread (${res.addressed.map((a) => a.id).join(', ')}), reply with a one-line note pointing at the fixing commit and resolve the thread (gh api GraphQL resolveReviewThread, or gh pr comment for plain review comments). Count replied + resolved.`,
    `3. LIFECYCLE (best-effort): extract the code PR number (gh pr view "${branch}" --json number --jq .number), then run \`node .claude/workflows/lib/lifecycle.js after-review-addressed --change "${change}" --code-pr "<prUrl>" --code-pr-number "<prNumber>"\`. It comments the linked ticket that review feedback was addressed. If the change isn't linked to a ticket it no-ops; if it errors, log and CONTINUE.`,
    `4. Do NOT merge and do NOT approve the PR — a human re-reviews. Threads left for the human (per the address step's notes) stay unresolved with an explanatory reply.`,
    `Return pushed, replied, resolved, notes.`,
  ].join('\n'),
  { schema: FIN, label: 'push-reply', phase: 'Push & reply', agentType: 'general-purpose' },
)

return {
  stage: 'done', ok: true, change, branch, prUrl: pre.prUrl,
  addressed: res.addressed.length,
  commits: res.commits,
  pushed: !!(fin && fin.pushed),
  replied: fin ? fin.replied : 0,
  resolved: fin ? fin.resolved : 0,
  notes: (res.notes ? res.notes + ' ' : '') + (fin ? fin.notes : ''),
  nextStep: `Pushed ${res.commits.length} fix commit(s) to PR #${pre.prNumber} and responded to ${fin ? fin.resolved : 0} thread(s). A human re-reviews; on approval, merge the PR, then run /opsx:archive ${change}.`,
}
