export const meta = {
  name: 'ship-plan',
  description:
    'Plan the execution of an APPROVED OpenSpec change as a reviewable handoff under .handoff/<change>/. Groups the change into a FEW test-first work-units (aim 1-4; collapse a small change to one) split along natural seams — package, capability, or spec requirement — NOT one unit per tasks.md line. Each unit is TDD and single-toolchain (py|go|ts): testDeliverables (the failing tests to write first — tests/test_*.py | *_test.go | *.test.ts(x) — asserting the delta-spec scenarios) then codeDeliverables (the production change), and covers one or more tasks.md items. Writes plan.json (the index of units), one tasks/<NN>-<slug>.md per unit (combined test+code plan), and a README.md of shared context. Honors args.local — when true, the plan.json carries localOnly=true so ship-code picks up the local merge path. Writes NO production code and creates NO branch. Idempotent: re-planning preserves any unit already marked done. The handoff is meant to be reviewed (and optionally hand-edited) before /opsx:ship-code executes it.',
  phases: [
    { title: 'Preflight', detail: 'node .claude/workflows/lib/openspec.js status + validate; read change artifacts' },
    { title: 'Plan', detail: 'group change into a few TDD units, write .handoff/<change>/' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const date = A.date // YYYY-MM-DD — passed in; Date.now()/new Date() are unavailable in scripts
const tdd = A.tdd === undefined ? true : !!A.tdd // default test-first
const local = A.local === true // when true, downstream ship-code uses the fully-local path

if (!change || typeof change !== 'string') {
  throw new Error('ship-plan requires args { change, date, tdd?, local? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z0-9][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (expected kebab-case slug): ' + change)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)
const handoffDir = `.handoff/${change}`

// ---------------------------------------------------------------- schemas
const PREFLIGHT = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'changeRoot', 'proposalPath', 'designPath', 'tasksPath', 'specPaths', 'changeTasks'],
  properties: {
    ok: { type: 'boolean', description: 'true only if node .claude/workflows/lib/openspec.js status + validate succeeded' },
    reason: { type: 'string' },
    changeRoot: { type: 'string' },
    proposalPath: { type: ['string', 'null'] },
    designPath: { type: ['string', 'null'] },
    uiPath: { type: ['string', 'null'] },
    tasksPath: { type: 'string' },
    specPaths: { type: 'array', items: { type: 'string' } },
    title: { type: 'string', description: 'human-readable change title from proposal.md' },
    changeTasks: {
      type: 'array',
      description: 'the parsed checklist items from the change tasks.md, in order',
      items: {
        type: 'object', additionalProperties: false, required: ['n', 'text', 'done'],
        properties: {
          n: { type: 'string', description: 'two-digit ordinal, e.g. "01"' },
          text: { type: 'string', description: 'the task line text' },
          done: { type: 'boolean', description: 'already ticked [x]' },
          tags: { type: 'array', items: { type: 'string' }, description: 'tags extracted from task text, e.g. ["ui", "backend"]' },
        },
      },
    },
  },
}
const PLAN = {
  type: 'object', additionalProperties: false, required: ['handoffDir', 'units', 'unitFiles', 'notes'],
  properties: {
    handoffDir: { type: 'string' },
    units: { type: 'integer', description: 'number of TDD work-units planned (a few per change, 1-4)' },
    unitFiles: { type: 'array', items: { type: 'string' }, description: 'all tasks/<NN>-<slug>.md files written' },
    notes: { type: 'string' },
  },
}

// ---------------------------------------------------------------- handoff format (single source of truth for the planner)
const HANDOFF_FORMAT = [
  `HANDOFF FORMAT — write these files under "${handoffDir}/" (create dirs as needed):`,
  ``,
  `1. ${handoffDir}/plan.json — the machine-readable index of UNITS:`,
  `   { "change": "${change}", "title": "<title>", "changeRoot": "<changeRoot>",`,
  `     "localOnly": ${local ? 'true' : 'false'},`,
  `     "units": [ <one object per work-unit> ] }`,
  `   Each unit object (additionalProperties NOT allowed):`,
  `   { "id": "01", "slug": "<kebab>", "title": "<imperative>", "status": "todo",`,
  `     "toolchain": "py|go|ts|bench|meta",  // the unit's primary toolchain (drives Red/Green test commands)`,
  `     "coversTasks": ["1","2"],            // the tasks.md ordinals this unit realizes`,
  `     "tags": ["ui", "backend"],          // tags inherited from covered tasks, e.g. ui, backend, api, db, auth, security`,
  `     "scenarios": ["<delta-spec scenario names this unit's tests assert>"],`,
  `     "testDeliverables": ["<test path: tests/test_*.py | *_test.go | *.test.ts(x)>", ...], // failing tests FIRST (Red)`,
  `     "codeDeliverables": ["<production path: *.py | *.go | *.ts(x)>", ...],               // impl to make them pass (Green)`,
  `     "verify": "<one-line checkable acceptance>", "skipRed": false }`,
  `   GROUPING RULES (this is the whole point — keep it COARSE):`,
  `   - Group the change's OPEN tasks into a FEW coherent units. Aim for 1-4 units total;`,
  `     collapse a small change to ONE unit. Do NOT emit one unit per tasks.md line.`,
  `   - Split along natural seams: package, capability, or spec requirement — and DO NOT mix`,
  `     toolchains in one unit (a unit is py OR go OR ts; the gate resolver groups by toolchain).`,
  `     E.g. "rag-core retrieve_kb ACL", "worker-mello MCP serve (go)", "portal session view (ts)".`,
  `   - Each unit is TEST-FIRST: its testDeliverables are written and must FAIL (Red)`,
  `     before its codeDeliverables (Green). A unit may span several files.`,
  `   - ids are two-digit ordinals "01","02",... in dependency order. Every open tasks.md`,
  `     ordinal MUST appear in exactly one unit's coversTasks.`,
  `   - A doc-only/pure-config unit with no testable behavior sets skipRed=true (with a`,
  `     reason in its Goal) and may have empty testDeliverables — never skip silently.`,
  ``,
  `2. ${handoffDir}/tasks/<id>-<slug>.md per unit (filename e.g. "01-tenant-core.md"):`,
  `   --- (YAML frontmatter, keys: id, slug, title, status: todo, coversTasks, tags,`,
  `       testDeliverables, codeDeliverables, verify, skipRed) ---`,
  `   ## Goal  <1-3 sentences — what this unit delivers>`,
  `   ## Context  Read ../README.md. <pointers to proposal/design + the exact delta-spec`,
  `   scenario(s) this unit realizes.>`,
  `   ## Test plan (Red)  - [ ] <per testDeliverable: the table cases/assertions to write,`,
  `   drawn from the scenarios — these must fail before the code exists>`,
  `   ## Code plan (Green) - [ ] <per codeDeliverable: the minimal production change to turn`,
  `   the Red tests green + the unit's resolver-selected gates green (uv/go/pnpm)>`,
  `   ## Output log`,
  `   <!-- appended by ship-code; leave empty -->`,
  ``,
  `3. ${handoffDir}/README.md — shared context:`,
  `   # ${change} — <title>`,
  `   ## Summary  (2-5 sentences from the proposal)`,
  `   ## Artifacts  (links: proposal, design, tasks.md, delta specs)`,
  `   ## Unit index  (table: id | covers tasks | test files | code files)`,
  `   ## Conventions  (per-toolchain gates via .claude/workflows/lib/gate-resolver.js, tenant_id everywhere, citations-mandatory, temperature==0, server-side ACL, TEST_DATABASE_URL/pgvector for DB tests, evidence dir)`,
  ``,
  `IDEMPOTENCY: if ${handoffDir}/plan.json already exists, read it first and PRESERVE the`,
  `status of any unit already marked "done" (do not regress it to "todo"); you may rewrite`,
  `the rest. Keep plan.json units[*] and the tasks/*.md frontmatter in sync.`,
].join('\n')

// Skills are injected dynamically via prompt hooks — extensions/agent-skills/Hooks/on-plan.prompt.md
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

// ---------------------------------------------------------------- Phase 1: Preflight
phase('Preflight')
const pre = await agent(
  [
    `Preflight planning for OpenSpec change "${change}". Use Bash. Steps:`,
    `1. node .claude/workflows/lib/openspec.js status --change "${change}" --json — parse changeRoot, proposal/design/tasks artifact paths, and the delta-spec paths. Read proposal.md for a one-line title.`,
    `2. node .claude/workflows/lib/openspec.js validate "${change}" --strict (fall back to non-strict) — MUST pass; if not, ok=false + reason and STOP.`,
    `3. Read the change's tasks.md and return its checklist items in order as changeTasks (n = two-digit ordinal by position, text = the line, done = whether it is [x]). For each task, also extract tags: look for "(tags: ...)" in the task title line, e.g. "## Task [1]: Login form  (tags: ui, auth)" → tags: ["ui", "auth"]. If no explicit tags, leave tags empty. Also infer tags from the task text (if it mentions UI keywords like "page", "screen", "component", "form", infer "ui" tag; if "API", "endpoint", "route", infer "api"; if "migration", "schema", infer "db"; etc.).`,
    `Do NOT create a branch and do NOT edit files. Return the structured result.`,
  ].join('\n'),
  { schema: PREFLIGHT, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.ok) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change }
}
const title = pre.title || change
const openTasks = (pre.changeTasks || []).filter((t) => !t.done)
log(`preflight ok — ${pre.changeTasks.length} change task(s), ${openTasks.length} open; grouping into a few TDD units`)
if (!openTasks.length) {
  return { stage: 'plan', ok: true, change, handoffDir, units: 0, unitFiles: [], notes: 'no open change tasks — nothing to plan', nextStep: `No open tasks in ${change}'s tasks.md — nothing to plan.` }
}

// ---------------------------------------------------------------- Phase 2: Plan (write the handoff)
phase('Plan')
const CONTEXT = [
  `Change "${change}" — "${title}". Ground the plan in these artifacts (read them):`,
  pre.proposalPath ? `- proposal (what & why): ${pre.proposalPath}` : '',
  pre.designPath ? `- design (how): ${pre.designPath}` : '',
  pre.uiPath ? `- ui (visual design): ${pre.uiPath}` : '',
  `- tasks (the checklist to expand): ${pre.tasksPath}`,
  pre.specPaths && pre.specPaths.length ? `- delta specs (the scenarios the tests must assert): ${pre.specPaths.join(', ')}` : '- delta specs: (none)',
].join('\n')

const plan = await agent(
  [
    `Write the execution handoff for OpenSpec change "${change}" into "${handoffDir}/". Apply ${await getPromptHooks('on-plan', { change }).then(h => h.join(' '))} (each unit's tests are its Red plan).`,
    CONTEXT,
    `GROUP these OPEN tasks.md items into a FEW test-first units (aim 1-4; collapse a small change to ONE unit). Open tasks: ${openTasks.map((t) => t.n + '. ' + t.text).join(' | ')}.`,
    `- Cluster the tasks by natural seam (package / capability / spec requirement) AND by toolchain (don't mix py/go/ts in one unit). Each cluster becomes ONE unit covering several tasks.md ordinals (coversTasks); set its "toolchain".`,
    `- For each unit: list its testDeliverables (the tests to write first in the unit's language — tests/test_*.py | *_test.go | *.test.ts(x) — with assertions/table cases drawn from the delta-spec scenarios; these fail before the code: Red) and its codeDeliverables (the production files that make them pass: Green).`,
    `- Do NOT emit one unit per tasks.md line. Fewer, larger units is the goal — the implementing agent handles a whole unit in one Red→Green→commit.`,
    `- TAG each unit: set its "tags" array to the union of tags from the covered tasks. E.g. if unit covers task [1] (tags: ui) and task [3] (tags: backend), unit gets tags: ["ui", "backend"]. Units with no task tags leave tags empty. Tags drive skill/hook loading during ship-code — a unit tagged "ui" automatically gets the ui-design skill and relevant hooks.`,
    `- A doc-only/pure-config unit sets skipRed=true with a one-line reason in its Goal (never skip silently).`,
    `Every open tasks.md ordinal MUST be covered by exactly one unit. changeRoot is ${pre.changeRoot}. Write every file per the format below, then return the handoff dir, the unit count, and the list of unit files.`,
    ``,
    HANDOFF_FORMAT,
  ].join('\n'),
  { schema: PLAN, label: 'write-handoff', phase: 'Plan', agentType: 'general-purpose' },
)
if (!plan) return { stage: 'plan', ok: false, reason: 'plan agent returned null', change, handoffDir }
log(`plan: wrote ${plan.units} unit(s), ${plan.unitFiles.length} file(s) under ${handoffDir}`)

// ---------------------------------------------------------------- Report
return {
  stage: 'done',
  ok: true,
  change,
  title,
  handoffDir,
  units: plan.units,
  unitFiles: plan.unitFiles,
  localOnly: local,
  notes: plan.notes,
  nextStep: `Handoff written to ${handoffDir}/ (${plan.units} TDD unit(s))${local ? ' — localOnly=true' : ''}. Review/edit the units, then run /opsx:ship-code ${change}${local ? ' --local' : ''} to implement them (one red+green commit per unit).`,
}
