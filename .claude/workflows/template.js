export const meta = {
  name: 'template',
  description:
    'Author / update / remove / list the optional, skill-like task-planning templates under templatesDir (default openspec/templates/). A template is a folder <name>/TEMPLATE.md with name+description frontmatter and a body that is the planning guide ("readme") — HOW to break this kind of work into tasks for this project. Templates are OPTIONAL: during planning the agent lists them and uses one only if it matches; most of the time there is none and planning proceeds normally. Dispatched by args.action: "list" prints the catalog (read-only); "create" drafts a guide from --prompt and writes it; "update" revises an existing guide; "remove" deletes one. All file ops go through the templates CLI (.claude/workflows/lib/templates.js), the same way ship-code.js calls the gate-resolver CLI.',
  phases: [
    { title: 'Resolve', detail: 'list catalog / draft or load the guide body' },
    { title: 'Apply', detail: 'write / update / remove the template file' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const action = A.action
const name = A.name // template name (kebab-case)
const description = A.description // one-line "when this applies"
const prompt = A.prompt // what the planning guide should say (for create/update)
const fromChange = A.fromChange // derive the guide from how a change was planned

if (!['list', 'create', 'update', 'remove'].includes(action)) {
  throw new Error('template requires args { action: "list"|"create"|"update"|"remove", name?, ... }; got action=' + action)
}
if (action !== 'list' && !name) throw new Error(`template action "${action}" requires a name`)
const CLI = 'node .claude/workflows/lib/templates.js'

// ---------------------------------------------------------------- schemas
const RESULT = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason'],
  properties: {
    ok: { type: 'boolean' }, reason: { type: 'string' },
    name: { type: 'string' }, description: { type: 'string' },
    file: { type: ['string', 'null'] },
    catalog: { type: 'array', items: { type: 'object' }, description: 'only for action=list' },
  },
}

// ================================================================ LIST
if (action === 'list') {
  phase('Resolve')
  const r = await agent(
    `List the available task-planning templates. Run: ${CLI} list --json — then return ok:true and the parsed array as "catalog" ([{name,description,file}]). Read-only; create/change nothing.`,
    { schema: RESULT, label: 'template-list', phase: 'Resolve', agentType: 'general-purpose' },
  )
  if (!r) return { stage: 'list', ok: false, reason: 'list agent returned null' }
  return {
    stage: 'list', ok: true, action, catalog: r.catalog || [],
    nextStep: (r.catalog || []).length
      ? 'Templates listed. During /opsx:spec planning, apply one only if it matches the change.'
      : 'No templates yet — that is fine. Author one with /opsx:template-create when a flow recurs.',
  }
}

// ================================================================ REMOVE
if (action === 'remove') {
  phase('Apply')
  const r = await agent(
    [
      `Remove the task-planning template "${name}". Use Bash.`,
      `1. Confirm it exists: ${CLI} show ${name} --json (if not found, return ok:false with the reason).`,
      `2. Delete it: ${CLI} remove ${name}.`,
      `Return ok, reason, name.`,
    ].join('\n'),
    { schema: RESULT, label: 'template-remove', phase: 'Apply', agentType: 'general-purpose' },
  )
  if (!r || !r.ok) return { stage: 'remove', ok: false, reason: r ? r.reason : 'remove agent returned null', action, name }
  return { stage: 'done', ok: true, action, name, nextStep: `Removed template "${name}".` }
}

// ================================================================ CREATE / UPDATE
{
  phase('Resolve')
  const draft = await agent(
    [
      `${action === 'create' ? 'Author' : 'Revise'} the planning guide for the "${name}" task-planning template. Do NOT write the file yet — just produce the guide text.`,
      action === 'update'
        ? `First read the current guide: ${CLI} show ${name}. Revise it per the instruction below; preserve what still applies.`
        : `This is a NEW template. Confirm it does not already exist: ${CLI} show ${name} (expect "not found").`,
      fromChange
        ? `Derive the guide from how change "${fromChange}" was planned — read openspec/changes/${fromChange}/{proposal.md,design.md,tasks.md} and generalize the task breakdown into a reusable playbook.`
        : `Guide intent: ${JSON.stringify(prompt || '')}`,
      `A good guide is a PLANNING PLAYBOOK, not the work itself: it tells a planner HOW to break this kind of change into tasks for THIS project's landscape — which sections/deliverables to cover, landscape-aware splits, which gates verify each task, and what to skip (YAGNI). Reference existing project standards rather than restating them.`,
      `Also produce a one-line "description" (when this template applies) if not already given (given: ${JSON.stringify(description || '')}).`,
      `Return ok:true with description set; keep the guide body for the next phase.`,
    ].join('\n'),
    { schema: RESULT, label: `template-draft:${name}`, phase: 'Resolve', agentType: 'general-purpose' },
  )
  if (!draft || !draft.ok) return { stage: 'draft', ok: false, reason: draft ? draft.reason : 'draft agent returned null', action, name }

  phase('Apply')
  const made = await agent(
    [
      `Write the "${name}" template to disk. Use Bash; pipe the guide body via stdin.`,
      `Run: printf '%s' "<the guide body you just wrote>" | ${CLI} create ${name} --description ${JSON.stringify(draft.description || description || '')}${action === 'update' ? ' --force' : ''}`,
      `Then verify with ${CLI} show ${name} --json and report its file path.`,
      `Return ok, reason, name, description, file.`,
    ].join('\n'),
    { schema: RESULT, label: `template-write:${name}`, phase: 'Apply', agentType: 'general-purpose' },
  )
  if (!made || !made.ok) return { stage: 'apply', ok: false, reason: made ? made.reason : 'write agent returned null', action, name }
  return {
    stage: 'done', ok: true, action, name: made.name || name, description: made.description || draft.description || '', file: made.file || null,
    nextStep: `${action === 'create' ? 'Authored' : 'Updated'} template "${name}". It will be offered during planning when it matches a change.`,
  }
}
