## ADDED Requirements

### Requirement: Cron-scheduled dispatch

The scheduler SHALL support **cron** schedules: a schedule whose `Kind` is `cron`
MUST fire at the times matched by its cron expression and, at each fire, dispatch
the configured agent version to the target runner via the catalog dispatch path.

#### Scenario: Cron schedule fires at the scheduled time

- **WHEN** a cron schedule `0 9 * * 1-5` dispatching `code-fixer@1.2.0` to runner `R` is created and the clock advances to the next 09:00 weekday
- **THEN** a dispatch for `code-fixer@1.2.0` is published to runner `R`'s topic at that fire time

#### Scenario: Cron does not fire outside its matched times

- **WHEN** the clock advances to a time the cron expression does not match
- **THEN** no dispatch is published for that schedule

### Requirement: Recurring schedules re-arm

The scheduler SHALL treat `cron` and `interval` schedules as **recurring**: after a
fire the scheduler MUST compute the next fire and re-arm, so a recurring schedule
fires once per period rather than once total.

#### Scenario: Interval schedule fires once per interval and re-arms

- **WHEN** an interval schedule with `Every` = 1h is created and two intervals elapse
- **THEN** the schedule fires once per interval (twice over two intervals) and remains armed for the next interval

### Requirement: One-shot run-at dispatch

The scheduler SHALL support a one-shot **at** schedule: a schedule whose `Kind` is
`at` MUST dispatch the agent exactly once when its `At` instant arrives and then
complete, with no re-arm.

#### Scenario: Run-at fires once then completes

- **WHEN** an at-time schedule for a fixed future instant is created and that instant arrives
- **THEN** the agent is dispatched exactly once and the schedule completes (it does not fire again)

### Requirement: Pause, resume, and cancel a schedule

The scheduler SHALL expose lifecycle controls: `Pause` MUST suppress fires without
discarding the schedule, `Resume` MUST re-arm a paused schedule, and `Cancel` MUST
remove the schedule permanently. A paused schedule MUST NOT fire; a canceled
schedule MUST NOT fire and is terminal.

#### Scenario: Paused schedule does not fire

- **WHEN** an active recurring schedule is paused and its next fire time elapses
- **THEN** no dispatch is published while the schedule is paused

#### Scenario: Resume re-arms and cancel removes

- **WHEN** a paused schedule is resumed and then canceled
- **THEN** resuming makes it eligible to fire again and canceling removes it so it no longer fires or appears in listings

### Requirement: Missed-fire policy while the runner is offline

The scheduler SHALL apply each schedule's `MissedPolicy` when a fire time elapses
with no runner online for the target: `skip` MUST drop the missed fire, and
`catch_up` MUST dispatch once on the next runner availability, coalescing multiple
missed fires into a single run.

#### Scenario: catch_up runs once on next availability

- **WHEN** a schedule with `Missed` = `catch_up` has its fire time elapse with no online runner, then the target runner comes online
- **THEN** the scheduler dispatches the missed run once on next availability

#### Scenario: skip drops the missed fire

- **WHEN** a schedule with `Missed` = `skip` has its fire time elapse with no online runner, then the target runner comes online
- **THEN** the missed fire is dropped and no catch-up dispatch is published

### Requirement: Timezone-aware cron evaluation

The scheduler SHALL evaluate a cron schedule's expression against the schedule's
IANA `TZ`, so the same expression fires at the local wall-clock time of its zone;
two schedules with the same expression but different `TZ` MUST fire at different
absolute instants.

#### Scenario: Same expression, different timezones fire at different instants

- **WHEN** two cron schedules use the expression `0 9 * * *` but one has `TZ` = `Asia/Ho_Chi_Minh` and the other `TZ` = `America/New_York`, and a day progresses
- **THEN** each fires at 09:00 in its own timezone, at different absolute instants

### Requirement: Per-tenant schedule listing and isolation

The scheduler SHALL scope every schedule to its creating tenant and MUST return,
for a `List` by tenant, only that tenant's schedules; there MUST be no
cross-tenant visibility of schedules.

#### Scenario: Listing returns only the tenant's schedules

- **WHEN** schedules are created under tenant `acme` and an `acme` operator lists schedules
- **THEN** only `acme`'s schedules are returned and no other tenant's schedules are visible
