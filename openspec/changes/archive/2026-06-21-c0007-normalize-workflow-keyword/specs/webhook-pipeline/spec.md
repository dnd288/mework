## MODIFIED Requirements

### Requirement: Trigger grammar

The system SHALL fire a job only for comments that match the trigger grammar
`@mework [profile] [workflow] [free instructions]`, where `profile` is the first
token, `workflow` is the second token when it is one of the recognized workflows
(`plan`, `cook`, `test`, `review`, `ship`, `journal`), and the remainder is free
instructions. `@mework` MUST be matched only at a word boundary (start of body,
or preceded by a space or newline). When the second token is a recognized
workflow, the parsed `workflow` value MUST be normalized to its canonical
lowercase form regardless of the casing or surrounding whitespace used in the
comment.

#### Scenario: Profile and workflow present

- **WHEN** a comment body is `@mework dev review fix the login bug`
- **THEN** the system parses profile `dev`, workflow `review`, and instructions `fix the login bug`

#### Scenario: Profile only

- **WHEN** a comment body is `@mework dev fix it`
- **THEN** the system parses profile `dev`, empty workflow, and instructions `fix it`

#### Scenario: Workflow keyword normalized to canonical case

- **WHEN** a comment body is `@mework dev Review fix the login bug`
- **THEN** the system parses workflow `review` (lowercased), not `Review`

#### Scenario: Not a trigger

- **WHEN** a comment body merely contains `@mework` inside another token (e.g. an email `test@mework.com`)
- **THEN** the system does NOT treat it as a trigger
