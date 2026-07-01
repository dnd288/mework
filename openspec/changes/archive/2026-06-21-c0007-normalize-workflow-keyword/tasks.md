# Tasks

- [x] 1. Add a pure `NormalizeWorkflow(w string) (string, bool)` to `internal/server/webhook/parse.go` that trims/lowercases and returns the canonical keyword + true when recognized, else `("", false)`.
- [x] 2. Use `NormalizeWorkflow` inside `ParseTrigger` so the returned `workflow` is canonical lowercase.
- [x] 3. Cover normalization in `internal/server/webhook/parse_test.go` (mixed case, whitespace padding, unknown keyword, empty) — including the spec scenario `@mework dev Review …` → `review`.
