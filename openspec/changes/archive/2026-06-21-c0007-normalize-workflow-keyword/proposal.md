# Proposal: Normalize the parsed workflow keyword

## Why

`ParseTrigger` (`internal/server/webhook/parse.go`) recognizes the workflow
keyword case-insensitively (via `isRecognizedWorkflow`, which lowercases), but then
returns the **original** token as the `workflow` value. So `@mework dev Review ...`
parses `workflow = "Review"`, while `@mework dev review ...` parses `workflow =
"review"`. Downstream consumers that switch on the workflow name (or persist it)
then see inconsistent values for the same intent, and a capitalized keyword can
silently route to the default path.

## What

Normalize the recognized workflow keyword to its canonical lowercase form when
parsing, so a trigger always yields one of `plan|cook|test|review|ship|journal`
(or empty when no recognized workflow is present). Expose a small, pure
`NormalizeWorkflow` helper and use it inside `ParseTrigger`.

This is a behavior-preserving hardening of the existing trigger grammar — no new
keywords, no change to which comments fire a job.

## Impact

- `internal/server/webhook/parse.go` — add `NormalizeWorkflow`; use it in `ParseTrigger`.
- `internal/server/webhook/parse_test.go` — cover normalization (mixed case, padding, unknown).
- Spec: `webhook-pipeline` "Trigger grammar" requirement gains a normalization scenario.
