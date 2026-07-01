export const meta = {
  name: 'spec-change',
  description:
    'Author and quality-assure an OpenSpec change spec: draft (if missing) → cross-validate → revise → report. With --worktree, works from a persistent spec worktree (../<project>-spec-<change>/) so the main checkout stays on the base branch.',
  phases: [
    { title: 'Worktree', detail: 'detect or create persistent spec worktree (only with --worktree)' },
    { title: 'Preflight', detail: 'node .claude/workflows/lib/openspec.js status + validate; load or scaffold the change' },
    { title: 'Hooks', detail: 'fire before-spec lifecycle + agent hook (best-effort)' },
    { title: 'Cross-validate', detail: 'one read-only critic per axis, in parallel' },
    { title: 'Revise', detail: 'fix Blocker/Required findings, re-validate (skipped on dryRun)' },
    { title: 'Report', detail: 'write review/REVIEW.md + verdict' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const slug = A.slug
const date = A.date
const dryRun = !!A.dryRun
const reserve = A.reserveTokens || 50000
const maxRevisions = A.maxRevisions ?? 2
const worktree = !!A.worktree  // work from a persistent spec worktree
const base = A.base || 'main'

if (!change || typeof change !== 'string') {
  throw new Error('spec-change requires args { change, date, dryRun?, slug?, maxRevisions?, worktree?, base? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (must start with a letter, kebab-case): ' + change)
if (slug && !/^[a-z0-9][a-z0-9-]*$/.test(slug)) throw new Error('Unsafe slug (kebab-case): ' + slug)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)

// ---------------------------------------------------------------- worktree path detection
let projectDir = ''
try { projectDir = require('path').basename(require('fs').realpathSync('.')) } catch { projectDir = '' }
const worktreeDir = projectDir ? `../${projectDir}-spec-${change}` : `../spec-${change}`

// ---------------------------------------------------------------- agent-form lifecycle hook (best-effort)
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
      `1. If openspec/hooks/on-${event}.agent.md does NOT exist → return { found:false, ran:false }. STOP (normal no-op).`,
      `2. If it exists → READ it and FOLLOW its instructions as your task, using the context above.`,
      `Return { found, ran, summary, errors }.`,
    ].join('\n'),
    { schema: HOOK_RESULT, label: `hook:${event}`, phase: 'Hooks', agentType: 'general-purpose' },
  )
  if (r && r.ran) log(`Agent hook on-${event}.agent.md: ${r.summary || 'ran'}`)
  else if (r && r.found) log(`Agent hook on-${event}.agent.md: failed${r.errors && r.errors.length ? ' — ' + r.errors.join('; ') : ''}`)
  return r
}

// ---------------------------------------------------------------- the review axes
const BASE_AXES = [
  { key: 'structure', title: 'Structure & validity', brief: 'node .claude/workflows/lib/openspec.js validate --strict passes; Purpose + Requirements; SHALL/MUST in the body line; >=1 Scenario each; delta uses ADDED/MODIFIED/REMOVED/RENAMED with the FULL requirement on MODIFY; MODIFIED/REMOVED names exist in the baseline spec. Structural failures are Blockers.' },
  { key: 'clarity', title: 'Clarity & KISS', brief: 'one requirement = one behavior; no "and...and..." packing across distinct concerns; plain-language bodies; specific requirement names.' },
  { key: 'testability', title: 'Testability', brief: 'every scenario decidable with concrete literals and a definite WHEN/THEN; NO soft MAY / "to the extent of" in an intended-behavior THEN; edge/negative scenarios where they matter.' },
  { key: 'minimality', title: 'Minimality & YAGNI', brief: 'normative requirements describe only in-scope/built behavior; future options belong in design.md, not requirements; spec behavior, not process; no requirement that changes no test.' },
  { key: 'consistency', title: 'Consistency & DRY', brief: 'each behavior defined once (reference, do not restate); canonical glossary terms; consistent with the CLAUDE.md invariants (a contradiction is a Blocker).' },
  { key: 'completeness', title: 'Completeness (not partials)', brief: 'every proposal claim -> a requirement; every requirement -> >=1 scenario AND covering task(s) in tasks.md; every task -> a requirement; design.md records the non-trivial decisions; ui.md records UI/visual decisions for user-facing changes with a Design Language Reference citing project design docs (or noting their absence) and UX patterns referenced.' },
]
// UX consistency axis is only added when the change has UI work (ui.md exists)
// This avoids running an irrelevant axis on backend-only changes
const UX_AXIS = { key: 'ux-consistency', title: 'UX Pattern Consistency', brief: 'for UI changes with a ui.md: reads the Design Language Reference and cross-checks against the project\'s existing UX patterns (DESIGN.md, design skills, openspec/patterns/ux-patterns.md, or codebase conventions); visual concept alignment is coherent; abstract business flow maps entry→steps→completion→next; ui.md reads as a final design handoff. If no ui.md exists, report that UX consistency is N/A.' }
// AXES is populated after preflight once we know if UI work exists

// ---------------------------------------------------------------- schemas
const PREFLIGHT = {
  type: 'object', additionalProperties: false,
  required: ['exists', 'ready', 'reason', 'changeRoot', 'tasksPath', 'specPaths'],
  properties: {
    exists: { type: 'boolean' },
    ready: { type: 'boolean' },
    reason: { type: 'string' },
    validatePass: { type: 'boolean' },
    changeRoot: { type: 'string' },
    proposalPath: { type: ['string', 'null'] },
    designPath: { type: ['string', 'null'] },
    uiPath: { type: ['string', 'null'] },
    tasksPath: { type: ['string', 'null'] },
    specPaths: { type: 'array', items: { type: 'string' } },
    title: { type: 'string' },
  },
}
const FINDING = {
  type: 'object', additionalProperties: false,
  required: ['severity', 'location', 'problem', 'suggestion'],
  properties: {
    axis: { type: 'string' },
    severity: { type: 'string', enum: ['Blocker', 'Required', 'Nit', 'FYI'] },
    location: { type: 'string' },
    problem: { type: 'string' },
    suggestion: { type: 'string' },
  },
}
const REVIEW = {
  type: 'object', additionalProperties: false, required: ['axis', 'summary', 'findings'],
  properties: { axis: { type: 'string' }, summary: { type: 'string' }, findings: { type: 'array', items: FINDING } },
}
const REVISE = {
  type: 'object', additionalProperties: false, required: ['applied', 'validatePass', 'remaining', 'notes'],
  properties: { applied: { type: 'integer' }, validatePass: { type: 'boolean' }, remaining: { type: 'array', items: FINDING }, notes: { type: 'string' } },
}
const REPORTW = {
  type: 'object', additionalProperties: false, required: ['written', 'path', 'verdict'],
  properties: { written: { type: 'boolean' }, path: { type: 'string' }, verdict: { type: 'string', enum: ['approve', 'revise'] } },
}

const isOpen = (f) => f && (f.severity === 'Blocker' || f.severity === 'Required')
const SKILL = (name) => `the \`${name}\` skill (.claude/skills/${name}/SKILL.md)`
const reviewSkill = SKILL('spec-review-and-quality')

// Build the cwd instruction prefix for worktree mode. Every subsequent agent
// prompt gets this as a leading line so it operates in the right directory.
const cwdPrefix = worktree ? `IMPORTANT: You are in a spec worktree at "${worktreeDir}". cd there first: cd "${worktreeDir}" && ` : ''

// ---------------------------------------------------------------- Phase 0 (optional): Worktree setup
let worktreePath = null
if (worktree) {
  phase('Worktree')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'worktree', ok: false, reason: 'budget reserve reached before worktree setup' }
  }

  const WTRES = {
    type: 'object', additionalProperties: false, required: ['ok'],
    properties: { ok: { type: 'boolean' }, worktreePath: { type: 'string' }, reason: { type: 'string' } },
  }
  const wt = await agent(
    [
      `Set up the spec worktree for change "${change}" on branch "spec/${change}" based on "${base}". Use Bash.`,
      `Steps to detect or create:`,
      `1. Check if worktree exists: test -d "${worktreeDir}" && test -f "${worktreeDir}/.claude/workflows/lib/openspec.js".`,
      `   - If YES: cd "${worktreeDir}" && git branch --show-current (must be "spec/${change}") && git pull origin "spec/${change}" --ff-only 2>/dev/null || true. Return { ok:true, worktreePath:"${worktreeDir}" }.`,
      `   - If NO: create it:`,
      `     a. git worktree add -b "spec/${change}" "${worktreeDir}" "${base}" 2>&1`,
      `     b. Verify: cd "${worktreeDir}" && git branch --show-current | grep "spec/${change}"`,
      `     c. ln -s "${worktreeDir}" ".worktree-${change}" 2>/dev/null || true`,
      `2. If the branch already exists on origin but not locally: git branch -t "spec/${change}" "origin/spec/${change}" && git worktree add "${worktreeDir}" "spec/${change}".`,
      `Return { ok:true, worktreePath:"${worktreeDir}" }.`,
    ].join('\n'),
    { schema: WTRES, label: 'setup-worktree', phase: 'Worktree' },
  )
  if (!wt || !wt.ok) {
    return { stage: 'worktree', ok: false, reason: wt ? wt.reason : 'worktree agent returned null', change }
  }
  worktreePath = wt.worktreePath
  log(`Spec worktree ready: ${worktreePath}`)
}

// ---------------------------------------------------------------- Phase 1: Preflight
phase('Preflight')
let pre = await agent(
  [
    cwdPrefix ? `IMPORTANT: ${cwdPrefix}` : '',
    `Preflight for OpenSpec change "${change}". Use Bash. Read-only except the scaffold step below.`,
    `1. Check if openspec/changes/${change}/ exists. Run: node .claude/workflows/lib/openspec.js status --change "${change}" --json (it errors if absent).`,
    `2. If it EXISTS: parse changeRoot, proposal/design/tasks artifact paths; read proposal.md for a one-line title; run node .claude/workflows/lib/openspec.js validate "${change}" --strict and report validatePass.`,
    `3. IMPORTANT — detect delta specs by scanning the filesystem: find openspec/changes/${change}/specs/ -name 'spec.md' 2>/dev/null | sort. Capture these paths as specPaths. Set ready=true ONLY when proposal + tasks exist AND at least one delta spec.md is found under specs/. If no delta specs found, set ready=false, reason="no delta specs under specs/<capability>/ — draft them first".`,
    !dryRun ? `4. LIFECYCLE (best-effort): once change EXISTS, run \`node .claude/workflows/lib/lifecycle.js before-spec --change "${change}"\` (no-op when unlinked).` : '',
    !dryRun && slug
      ? `5. If it does NOT exist: scaffold it now via "node .claude/workflows/lib/openspec.js new change "${change}"" and draft artifacts following ${SKILL('openspec-propose')} and ${SKILL('spec-driven-development')}.`
      : `5. If it does NOT exist: set exists=false, ready=false, reason="change not found"${dryRun ? ' (dryRun)' : ' (no slug given to draft)'} and STOP.`,
    `Return the structured result. The "specPaths" field must be populated from the filesystem scan (step 3), NOT from openspec status.`,
  ].filter(Boolean).join('\n'),
  { schema: PREFLIGHT, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.exists || !pre.ready) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change }
}
const title = pre.title || change
const CONTEXT = [
  cwdPrefix ? `IMPORTANT: ${cwdPrefix}` : '',
  `Change "${change}" — "${title}". Artifacts (read them):`,
  pre.proposalPath ? `- proposal: ${pre.proposalPath}` : '',
  pre.designPath ? `- design: ${pre.designPath}` : '',
  pre.tasksPath ? `- tasks: ${pre.tasksPath}` : '',
  pre.uiPath ? `- ui (visual design): ${pre.uiPath}` : '',
  pre.specPaths && pre.specPaths.length ? `- delta specs: ${pre.specPaths.join(', ')}` : '- delta specs: (none)',
].filter(Boolean).join('\n')
log(`preflight ok — ${change}: ${pre.specPaths.length} delta spec(s); validate ${pre.validatePass ? 'passed' : 'NOT passing'}`)

// ---------------------------------------------------------------- Phase 1b: before-spec hook
if (!dryRun) {
  phase('Hooks')
  await runAgentHook('before-spec', [`- change: ${change}`, `- title: ${title}`])
}

// ---------------------------------------------------------------- Phase 2: Cross-validate (parallel critics)
// Conditionally add UX consistency axis when change has UI work
const AXES = pre.uiPath ? [...BASE_AXES, UX_AXIS] : BASE_AXES
phase('Cross-validate')
const reviews = (await parallel(
  AXES.map((ax) => () =>
    agent(
      [
        cwdPrefix ? `IMPORTANT: ${cwdPrefix}` : '',
        `Review OpenSpec change "${change}" on ONE axis only: ${ax.title}.`,
        `Apply ${reviewSkill}. READ-ONLY — do NOT edit any file.`,
        CONTEXT,
        `Axis focus: ${ax.brief}`,
        ax.key === 'structure' ? `Run: node .claude/workflows/lib/openspec.js validate "${change}" --strict --no-interactive — a failure is a Blocker; quote the error.` : '',
        ax.key === 'consistency' || ax.key === 'completeness' || ax.key === 'ux-consistency' ? `You MAY read the baseline openspec/specs/<capability>/spec.md, openspec/patterns/ux-patterns.md, DESIGN.md, and CLAUDE.md to cross-check.` : '',
        `Return findings strictly scoped to THIS axis. If clean, return an empty findings array with a one-line summary.`,
      ].filter(Boolean).join('\n'),
      { schema: REVIEW, label: `critic:${ax.key}`, phase: 'Cross-validate', agentType: 'general-purpose' },
    ),
  ),
)).filter(Boolean)

let findings = reviews.flatMap((r) => (r.findings || []).map((f) => ({ ...f, axis: r.axis })))
let openCount = findings.filter(isOpen).length
log(`cross-validate: ${reviews.length}/${AXES.length} axes reviewed, ${findings.length} finding(s), ${openCount} Blocker/Required`)

// ---------------------------------------------------------------- Phase 3: Revise
let revisions = 0
let lastValidate = pre.validatePass
if (dryRun) {
  log('dryRun: skipping revise')
} else {
  phase('Revise')
  while (openCount > 0 && revisions < maxRevisions) {
    if (budget && budget.total && budget.remaining() < reserve) {
      log(`revise: stopping — budget reserve (${reserve}) reached with ${openCount} open finding(s)`)
      break
    }
    revisions++
    const open = findings.filter(isOpen)
    const rev = await agent(
      [
        cwdPrefix ? `IMPORTANT: ${cwdPrefix}` : '',
        `Revise OpenSpec change "${change}" to resolve ${open.length} Blocker/Required finding(s). Apply ${reviewSkill}. Use Bash + edit the change's artifacts only.`,
        CONTEXT,
        `Findings to fix:`,
        ...open.map((f, i) => `${i + 1}. [${f.severity}] (${f.axis}) ${f.location} — ${f.problem} → ${f.suggestion}`),
        `Make minimal edits that resolve each finding. Keep deltas in ADDED/MODIFIED/REMOVED/RENAMED form. Then run: node .claude/workflows/lib/openspec.js validate "${change}" --strict --no-interactive.`,
        `Return applied (count fixed), validatePass, remaining (any you could NOT resolve, with the original axis preserved).`,
      ].join('\n'),
      { schema: REVISE, label: `revise:${revisions}`, phase: 'Revise', agentType: 'general-purpose' },
    )
    if (!rev) { log(`revise:${revisions} returned null — stopping`); break }
    lastValidate = rev.validatePass
    findings = (rev.remaining || []).map((f) => ({ ...f, axis: f.axis || 'revise' }))
    openCount = findings.filter(isOpen).length
    log(`revise:${revisions}: applied ${rev.applied}, validate ${rev.validatePass ? 'pass' : 'FAIL'}, ${openCount} open`)
  }
}

// ---------------------------------------------------------------- Phase 4: Report
phase('Report')
const verdict = openCount === 0 ? 'approve' : 'revise'
const reportLines = findings.map((f) => `- **${f.severity}** _(${f.axis})_ ${f.location} — ${f.problem}${f.suggestion ? ` → ${f.suggestion}` : ''}`)
const rep = await agent(
  [
    cwdPrefix ? `IMPORTANT: ${cwdPrefix}` : '',
    `Write the spec-review report for change "${change}" to "${pre.changeRoot}/review/REVIEW.md". Use Bash/Write.`,
    `Header: "# Spec review — ${change}${date ? ` (${date})` : ''}" then "Verdict: ${verdict.toUpperCase()}", axes reviewed (${AXES.map((a) => a.title).join('; ')}), revisions (${revisions}), validate: ${lastValidate ? 'pass' : 'NOT passing'}.`,
    findings.length ? `Then "## Findings":\n${reportLines.join('\n')}` : `Then "## Findings"\n_No Blocker/Required; spec is clean._`,
    `Return written, path, verdict ("approve" if openCount === ${openCount}).`,
  ].join('\n'),
  { schema: REPORTW, label: 'report', phase: 'Report', agentType: 'general-purpose' },
)

const nextStep = verdict === 'approve'
  ? `Spec "${change}" is clean.${worktreePath ? ` (worktree: ${worktreePath})` : ''} Run /opsx:spec-pr${worktree ? ' --worktree' : ''} ${change} to open the spec PR.`
  : `Spec "${change}" still has ${openCount} Blocker/Required${dryRun ? ' (dryRun)' : ` after ${revisions} revision(s)`}. See ${rep ? rep.path : 'review/REVIEW.md'}.`

return {
  stage: 'done',
  ok: verdict === 'approve',
  change, title, dryRun, verdict, revisions,
  validatePass: lastValidate,
  openFindings: openCount,
  totalFindings: findings.length,
  reviewReport: rep ? rep.path : `${pre.changeRoot}/review/REVIEW.md`,
  worktreePath,
  nextStep,
}
