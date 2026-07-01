export const meta = {
  name: 'propose',
  description:
    'Scaffold a new OpenSpec change from a free-text prompt — the github-agnostic front door to the pipeline. With --worktree, creates a persistent git worktree (../<project>-spec-<change>/) on a spec/<change> branch and scaffolds inside it, so the main checkout stays on the base branch and multiple specs can be worked simultaneously.',
  phases: [
    { title: 'Worktree', detail: 'create a persistent spec worktree (only with --worktree)' },
    { title: 'Scaffold', detail: 'compute cNNNN-<slug>, openspec new change, seed proposal.md from the prompt' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const prompt = (A.prompt != null ? String(A.prompt) : '').trim()
const title = (A.title != null ? String(A.title) : '').trim()
const slug = A.slug
const date = A.date
const worktree = !!A.worktree  // create a persistent spec worktree
const base = A.base || 'main'
const reserve = A.reserveTokens || 20000

if (!prompt && !title) throw new Error('propose requires args { prompt: "<what to build>", title?, slug?, date?, worktree?, base? }')
if (slug && !/^[a-z0-9][a-z0-9-]*$/.test(slug)) throw new Error('Unsafe slug (kebab-case): ' + slug)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)

const SKILL = (name) => `the \`${name}\` skill (.claude/skills/${name}/SKILL.md)`

// ---------------------------------------------------------------- helper: compute the next change name
// We need the change name BEFORE scaffolding to name the worktree. Run a minimal
// agent just for this.
const COMPUTE = {
  type: 'object', additionalProperties: false, required: ['ok', 'change'],
  properties: { ok: { type: 'boolean' }, change: { type: 'string' }, reason: { type: 'string' } },
}
const computed = await agent(
  [
    `Compute the next OpenSpec change name. Use Bash.`,
    `Run: ls -1d openspec/changes/c[0-9]* 2>/dev/null | sed -E 's#.*/c([0-9]+).*#\\1#' | sort -n | tail -1`,
    `Next ordinal = max + 1 (start at 1 if none). Zero-pad to 4 digits.`,
    `Slug: ${slug ? `"${slug}"` : `derive a kebab slug from the title "${title}" (<= 48 chars, lower-case, hyphens)`}.`,
    `Return { ok: true, change: "cNNNN-<slug>" }.`,
  ].join('\n'),
  { schema: COMPUTE, label: 'compute-change-name', agentType: 'general-purpose' },
)
if (!computed || !computed.ok || !computed.change) {
  return { stage: 'compute', ok: false, reason: computed ? (computed.reason || 'could not compute change name') : 'compute agent returned null' }
}
const changeName = computed.change

// ---------------------------------------------------------------- try to detect project dir name from cwd for worktree path
let projectDir = ''
try {
  projectDir = require('path').basename(require('fs').realpathSync('.'))
} catch { projectDir = '' }
const worktreeDir = projectDir ? `../${projectDir}-spec-${changeName}` : `../spec-${changeName}`

// ---------------------------------------------------------------- Phase 1 (optional): Create persistent spec worktree
if (worktree) {
  phase('Worktree')
  if (budget && budget.total && budget.remaining() < reserve) {
    return { stage: 'worktree', ok: false, reason: 'budget reserve reached before worktree creation' }
  }

  const WTRES = {
    type: 'object', additionalProperties: false, required: ['ok'],
    properties: { ok: { type: 'boolean' }, worktreePath: { type: 'string' }, reason: { type: 'string' } },
  }
  const wt = await agent(
    [
      `Create a persistent spec worktree for change "${changeName}". Use Bash.`,
      `The main checkout is at $(pwd). Create the worktree at ${worktreeDir} on a new branch "spec/${changeName}" based on "${base}".`,
      `Steps:`,
      `1. Check the worktree doesn't already exist: test -d "${worktreeDir}" && test -f "${worktreeDir}/.claude/workflows/lib/openspec.js" → if yes, return { ok: true, worktreePath: "${worktreeDir}" } (reuse).`,
      `2. Ensure tree is clean enough for worktree creation: git status --porcelain. Stash or commit any critical changes first.`,
      `3. Create the worktree: git worktree add -b "spec/${changeName}" "${worktreeDir}" "${base}" 2>&1.`,
      `4. Verify: cd "${worktreeDir}" && git branch --show-current (should be "spec/${changeName}") && ls .claude/workflows/lib/openspec.js >/dev/null.`,
      `5. Report: ln -s "${worktreeDir}" .worktree-${changeName} 2>/dev/null || true (a convenience symlink from the main checkout to the worktree).`,
      `Return { ok:true, worktreePath:"${worktreeDir}" } on success.`,
    ].join('\n'),
    { schema: WTRES, label: 'create-worktree', phase: 'Worktree' },
  )
  if (!wt || !wt.ok) {
    return { stage: 'worktree', ok: false, reason: wt ? wt.reason : 'worktree agent returned null' }
  }
  log(`Spec worktree created at ${wt.worktreePath}`)
}

// ---------------------------------------------------------------- scaffold target directory (worktree or main)
const scaffoldCwd = worktree ? worktreeDir : '.'
const cwdNote = worktree ? `IMPORTANT: You are working inside the worktree at "${worktreeDir}". cd there first: cd "${worktreeDir}"` : ''

// ---------------------------------------------------------------- Phase 2: Scaffold
const SCAFFOLD = {
  type: 'object', additionalProperties: false, required: ['ok', 'reason'],
  properties: {
    ok: { type: 'boolean' }, reason: { type: 'string' },
    change: { type: 'string', description: 'the created cNNNN-<slug> change name' },
    proposalPath: { type: ['string', 'null'] },
    uiPath: { type: ['string', 'null'] },
  },
}
phase('Scaffold')
const made = await agent(
  [
    `Scaffold a new OpenSpec change from this request, then seed its proposal. Use Bash. Apply ${SKILL('openspec-propose')} for the numbering + scaffolding conventions.`,
    cwdNote,
    `Request (what to build): ${JSON.stringify(prompt || title)}`,
    title && title !== prompt ? `Human title: ${JSON.stringify(title)}` : '',
    `The change name is already computed: "${changeName}". Use this exact name.`,
    `1. Create it: node .claude/workflows/lib/openspec.js new change "${changeName}".`,
    `2. Seed proposal.md: get its path (node .claude/workflows/lib/openspec.js instructions proposal --change "${changeName}" --json) and write a proposal whose "## What" / "## Why" are grounded in the request above. Keep it faithful to the request; do NOT invent scope.`,
    `2a. Optionally create ui.md: if the change has a visible user-facing surface (screens, components, forms, portals — check the request for keywords like "UI", "page", "screen", "component", "form", "frontend", or "portal"), create ui.md following the ui-design skill. Path: node .claude/workflows/lib/openspec.js instructions ui --change "${changeName}" --json. If no UI surface, skip it.`,
    `Return ok, reason, change ("${changeName}"), proposalPath, uiPath.`,
  ].filter(Boolean).join('\n'),
  { schema: SCAFFOLD, label: 'propose-scaffold', phase: 'Scaffold', agentType: 'general-purpose' },
)

if (!made || !made.ok || !made.change) {
  return { stage: 'scaffold', ok: false, reason: made ? (made.reason || 'scaffold returned no change') : 'scaffold agent returned null' }
}

const worktreePath = worktree ? worktreeDir : null
const nextStep = worktreePath
  ? `Scaffolded change ${made.change} in spec worktree at ${worktreePath}. To work on specs: cd ${worktreePath} && /opsx:spec ${made.change}. Main checkout stays on ${base}.`
  : `Scaffolded change ${made.change}. Review/refine proposal.md, then run /opsx:spec ${made.change}.`

return {
  stage: 'done', ok: true, change: made.change, proposalPath: made.proposalPath || null,
  uiPath: made.uiPath || null,
  worktreePath,
  nextStep,
}
