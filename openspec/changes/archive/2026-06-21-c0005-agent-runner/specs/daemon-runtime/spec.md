## MODIFIED Requirements

### Requirement: Stateless poll worker

The daemon SHALL operate as an **enrolled SSE runner**, not a poll worker. After
one-time enrollment it MUST subscribe to its topics over SSE (per `message-bus`),
receive dispatches by push, and drive a **pull → run → report** loop, persisting
only its durable runner identity and in-flight bookkeeping. It MUST NOT poll a
claim endpoint on a fixed interval.

#### Scenario: No interval polling

- **WHEN** an enrolled runner is online and idle
- **THEN** it holds an open SSE subscription and issues no periodic claim/poll requests

#### Scenario: Driven by dispatch

- **WHEN** the hub dispatches work to the runner
- **THEN** the runner pulls the agent, runs it, and reports the result, acknowledging the dispatch on terminal handling
