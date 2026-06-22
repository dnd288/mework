## MODIFIED Requirements

### Requirement: Required server secrets

The system SHALL require `DATABASE_URL`, `SERVER_KEY`, and `MEWORK_SECRET_KEY` at startup and
MUST fail fast if any is missing. The system SHALL additionally enforce a **minimum length**
for `SERVER_KEY` and `MEWORK_SECRET_KEY` and MUST fail fast, naming the offending variable,
when either is shorter than the minimum — so a trivially-weak key is rejected rather than
silently accepted and stretched.

#### Scenario: Missing secret aborts startup

- **WHEN** the server starts without `MEWORK_SECRET_KEY`
- **THEN** it refuses to start rather than run without credential sealing

#### Scenario: Weak key aborts startup

- **WHEN** the server starts with a `SERVER_KEY` or `MEWORK_SECRET_KEY` shorter than the
  minimum length
- **THEN** it refuses to start and the error names the offending variable and the minimum
