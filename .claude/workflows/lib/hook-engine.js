// hook-engine.js — shared hook driver for the core pipeline
// Extracted from duplicated inline code in spec-change.js, spec-pr.js, ship-code.js, merge-pr.js
//
// Two hook types:
//   Agent hook (.agent.md)  — natural-language instructions → spawns agent() to execute
//   Prompt hook (.prompt.md) — prompt text → returned for injection into the running agent
//
// Hooks are discovered from:
//   1. openspec/hooks/on-<event>.agent.md  (or .prompt.md)
//   2. extensions/*/Hooks/on-<event>.agent.md (or .prompt.md)

const HOOK_RESULT = {
  type: 'object',
  description: 'Result of running an agent-form hook',
  properties: {
    found: { type: 'boolean', description: 'Whether a hook file was found' },
    ran: { type: 'boolean', description: 'Whether the hook was actually executed' },
    summary: { type: 'string', description: 'What the hook did, or empty string' },
    errors: {
      type: 'array',
      items: { type: 'string' },
      description: 'Any errors encountered during hook execution',
    },
  },
  required: ['found', 'ran', 'summary', 'errors'],
}

// --- Pipeline-phase events ---
// Maps from new pipeline-phase event names to the old lifecycle names (for backwards compat)
const PIPELINE_EVENTS = [
  'on-propose',
  'on-spec-start',
  'on-spec-end',
  'on-spec-pr-start',
  'on-spec-pr-end',
  'on-ship-start',
  'on-ship-end',
  'on-merge',
]

const EVENT_MAP = {
  'on-propose': null,
  'on-spec-start': 'before-spec',
  'on-spec-end': null,
  'on-spec-pr-start': null,
  'on-spec-pr-end': 'after-spec-pr-opened',
  'on-ship-start': 'before-ship',
  'on-ship-end': 'after-code-pr-opened',
  'on-merge': 'after-code-pr-merged',
}

/**
 * Run an agent hook for the given event.
 * Checks openspec/hooks/ and extensions/*/Hooks/ for on-<event>.agent.md files.
 * If found, spawns an agent to execute the instructions.
 *
 * @param {string} event — pipeline event name (e.g. 'on-spec-start')
 * @param {object} context — { change, ...additional context }
 * @returns {Promise<{found: boolean, ran: boolean, summary: string, errors: string[]}>}
 */
async function runAgentHook(event, context = {}) {
  const change = context.change || ''
  const lines = [
    `Event: ${event}`,
    `Change: ${change}`,
    `Context: ${JSON.stringify(context)}`,
    `Check openspec/hooks/on-${event}.agent.md and each extensions/*/Hooks/on-${event}.agent.md.`,
    'If a hook file exists: read it and follow its instructions using available tools.',
    'If no hook file exists: return { found: false, ran: false, summary: "", errors: [] }.',
  ]
  return agent(lines.join('\n'), { schema: HOOK_RESULT, label: `hook:${event}`, phase: 'Hooks' })
}

/**
 * Read all prompt hooks for the given event.
 * Returns an array of prompt text fragments to inject into the running agent's context.
 *
 * @param {string} event — pipeline event name
 * @param {object} context — { change, ... }
 * @returns {Promise<string[]>} — prompt fragments
 */
async function getPromptHooks(event, context = {}) {
  const change = context.change || ''
  const lines = [
    `Event: ${event}`,
    `Change: ${change}`,
    `Read openspec/hooks/on-${event}.prompt.md and each extensions/*/Hooks/on-${event}.prompt.md.`,
    'For each file found: return its contents as a string in the array.',
    'If no files found: return an empty array.',
  ]
  return agent(lines.join('\n'), {
    schema: {
      type: 'object',
      properties: {
        prompts: {
          type: 'array',
          items: { type: 'string' },
          description: 'Prompt text fragments from hook files',
        },
      },
      required: ['prompts'],
    },
    label: `prompt-hooks:${event}`,
    phase: 'Hooks',
  }).then(r => (r && r.prompts) || [])
}

/**
 * Fire all hooks (agent + prompt) for an event.
 * Convenience wrapper that runs both hook types.
 *
 * @param {string} event — pipeline event name
 * @param {object} context — { change, ... }
 * @returns {Promise<{agentHooks: object[], promptHooks: string[], errors: string[]}>}
 */
async function fireHooks(event, context = {}) {
  const [agentResult, prompts] = await Promise.all([
    runAgentHook(event, context).catch(e => ({ found: false, ran: false, summary: '', errors: [e.message] })),
    getPromptHooks(event, context).catch(e => []),
  ])
  return {
    agentHooks: agentResult ? [agentResult] : [],
    promptHooks: prompts,
    errors: (agentResult && agentResult.errors) || [],
  }
}

// Workflow files access these via the global scope — no require() needed
