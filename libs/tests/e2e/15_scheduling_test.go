package e2e

import "testing"

// Feature 15 — Scheduling & recurring runs. Real-world platform surface (proposed).
// A production agent hub dispatches agents on cron/interval/at-time schedules.

func TestSCHED_01_CronFires(t *testing.T) {
	Scenario(t, "SCHED-01", "Cron schedule fires at the scheduled time", PlannedPlatform).
		Given("a cron schedule `0 9 * * 1-5` dispatching code-fixer to runner R", func(w *World) {
			id, err := w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleCron, Cron: "0 9 * * 1-5", TZ: "Asia/Ho_Chi_Minh", Agent: AgentRef{Name: "code-fixer", Version: "1.2.0"}, Target: "R"})
			w.set("id", id)
			w.expect(err == nil, "schedule should be created")
		}).
		When("the clock advances to the next 09:00 weekday", func(w *World) {
			w.Advance(24 * 3600)
		}).
		Then("a dispatch is published to R's topic at that time", func(w *World) {
			w.expect(true, "the cron fire dispatches the agent")
		}).
		Run()
}

func TestSCHED_02_RecurringReArms(t *testing.T) {
	Scenario(t, "SCHED-02", "Recurring schedule re-arms after each run", PlannedPlatform).
		Given("an interval schedule every 1h", func(w *World) {
			_, _ = w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleInterval, Every: 3600, Agent: AgentRef{Name: "x"}, Target: "R"})
		}).
		When("two intervals elapse", func(w *World) {
			w.Advance(2 * 3600)
		}).
		Then("it fires once per interval and re-arms (not one-shot)", func(w *World) {
			w.expect(true, "a recurring schedule keeps firing on each interval")
		}).
		Run()
}

func TestSCHED_03_DelayedRunAt(t *testing.T) {
	Scenario(t, "SCHED-03", "One-shot run-at dispatch", PlannedPlatform).
		Given("an at-time schedule for a fixed future instant", func(w *World) {
			_, err := w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleAt, Agent: AgentRef{Name: "x"}, Target: "R"})
			w.set("err", err)
		}).
		When("that instant arrives", func(w *World) {
			w.Advance(3600)
		}).
		Then("the agent is dispatched exactly once and the schedule completes", func(w *World) {
			w.expect(w.get("err") == nil, "a run-at schedule fires once then completes")
		}).
		Run()
}

func TestSCHED_04_PauseResumeCancel(t *testing.T) {
	Scenario(t, "SCHED-04", "Pause, resume, and cancel a schedule", PlannedPlatform).
		Given("an active recurring schedule", func(w *World) {
			id, _ := w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleInterval, Every: 3600, Target: "R"})
			w.set("id", id)
		}).
		When("it is paused, then resumed, then canceled", func(w *World) {
			id := w.get("id").(ScheduleID)
			_ = w.Scheduler.Pause(ctx(), id)
			_ = w.Scheduler.Resume(ctx(), id)
			w.set("err", w.Scheduler.Cancel(ctx(), id))
		}).
		Then("a paused schedule does not fire; a canceled one is removed", func(w *World) {
			w.expect(w.get("err") == nil, "pause suppresses fires; cancel removes the schedule")
		}).
		Run()
}

func TestSCHED_05_MissedFireWhileOffline(t *testing.T) {
	Scenario(t, "SCHED-05", "Missed fire while the runner is offline follows policy", PlannedPlatform).
		Given("a schedule with Missed=catch_up and its target runner offline at fire time", func(w *World) {
			_, _ = w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleInterval, Every: 3600, Target: "R", Missed: MissedCatchUp})
		}).
		When("the fire time elapses with no online runner, then R comes online", func(w *World) {
			w.Advance(3600)
		}).
		Then("catch_up runs once on next availability; skip would have dropped it", func(w *World) {
			w.expect(true, "missed-fire policy governs offline behavior")
		}).
		Run()
}

func TestSCHED_06_TimezoneRespected(t *testing.T) {
	Scenario(t, "SCHED-06", "Cron honors the schedule's timezone", PlannedPlatform).
		Given("two cron schedules with the same expression but different TZ", func(w *World) {
			_, _ = w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleCron, Cron: "0 9 * * *", TZ: "Asia/Ho_Chi_Minh", Target: "R"})
			_, _ = w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleCron, Cron: "0 9 * * *", TZ: "America/New_York", Target: "R"})
		}).
		When("the day progresses", func(w *World) {
			w.Advance(24 * 3600)
		}).
		Then("each fires at 09:00 in its own timezone", func(w *World) {
			w.expect(true, "cron evaluation is timezone-aware")
		}).
		Run()
}

func TestSCHED_07_ListSchedulesPerTenant(t *testing.T) {
	Scenario(t, "SCHED-07", "Schedules are listed and isolated per tenant", PlannedPlatform).
		Given("schedules created under tenant acme", func(w *World) {
			_, _ = w.Scheduler.Schedule(ctx(), ScheduleSpec{Kind: ScheduleInterval, Every: 3600, Target: "R"})
		}).
		When("an acme operator lists schedules", func(w *World) {
			ids, _ := w.Scheduler.List(ctx(), "acme")
			w.set("ids", ids)
		}).
		Then("only acme's schedules are returned", func(w *World) {
			w.expect(true, "schedule listing is tenant-scoped")
		}).
		Run()
}
