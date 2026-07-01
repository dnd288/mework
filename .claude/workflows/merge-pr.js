export const meta = {
  name: 'merge-pr',
  description:
    'PR merge dispatcher. Auto-detects whether the branch is spec/<change> or feat/<change> and delegates to merge-pr-spec or merge-pr-code respectively.',
  phases: [
    { title: 'Detect',  detail: 'detect branch type (spec/ or feat/), find the PR' },
    { title: 'Route', detail: 'delegate to merge-pr-spec or merge-pr-code with same args' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const pr = A.pr
if (!change && !pr) {
  throw new Error('merge-pr requires either { change } or { pr }.')
}

// ---------------------------------------------------------------- Phase 1: Detect branch type
phase('Detect')
const DETECT = {
  type: 'object', additionalProperties: false, required: ['branchType', 'headRefName'],
  properties: {
    branchType: { type: 'string', enum: ['spec', 'feat', 'other', 'unknown'] },
    headRefName: { type: 'string' },
    prNumber: { type: 'integer' },
    prUrl: { type: 'string' },
    reason: { type: 'string' },
  },
}

const detect = await agent(
  [
    `Detect branch type for merge-pr dispatch. Use Bash + gh.`,
    change
      ? `Check if PR exists for spec/${change} or feat/${change}. Try spec first, then feat.`
      : `Parse PR#${pr ? ` from "${pr}"` : ''} and detect its headRefName branch.`,
    ``,
    `Step 1 — Find the PR${change ? ` for change "${change}"` : ''}:`,
    change
      ? [
          `  spec: gh pr view "spec/${change}" --json headRefName,number,url,state 2>/dev/null || true`,
          `  feat: gh pr view "feat/${change}" --json headRefName,number,url,state 2>/dev/null || true`,
          `  Pick the OPEN one. If both OPEN, prefer feat.`,
          `  If only one exists (regardless of state), use that.`,
          `  If neither exists: return { branchType:"unknown", reason:"no PR found for ${change}" }.`,
        ].join('\n')
      : `  gh pr view <number> --json headRefName,number,url,state.`,
    ``,
    `Step 2 — Determine branchType:`,
    `  - headRefName starts with "spec/" → branchType: "spec"`,
    `  - headRefName starts with "feat/" → branchType: "feat"`,
    `  - otherwise → branchType: "other"`,
    ``,
    `Return { branchType, headRefName, prNumber, prUrl, reason }.`,
  ].join('\n'),
  { schema: DETECT, label: 'detect-branch', phase: 'Detect', agentType: 'general-purpose' },
)

if (!detect || !detect.branchType || detect.branchType === 'unknown') {
  return { stage: 'detect', ok: false, reason: detect ? (detect.reason || 'could not detect branch type') : 'detect agent returned null', change, pr }
}

log(`Detected branch type: ${detect.branchType} (${detect.headRefName}) — PR #${detect.prNumber}`)

// ---------------------------------------------------------------- Phase 2: Route
phase('Route')
const workflowName = detect.branchType === 'spec' ? 'merge-pr-spec' : 'merge-pr-code'
log(`Routing to ${workflowName}...`)

const subArgs = { ...A }
if (!subArgs.change || subArgs.change === change) {
  subArgs.change = change || detect.headRefName.replace(/^(spec|feat)\//, '')
}

const result = await workflow(workflowName, subArgs)
return result || { stage: 'route', ok: false, reason: `${workflowName} returned null`, change, pr }
