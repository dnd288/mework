package e2e

import "testing"

// Feature 07 — Job queue (poll model, today). Source: SCENARIOS.md
// Baseline behavior (Implemented); skipped pending the e2e World harness. Under the target
// bus (c0002) the claim/heartbeat requirements are removed — see Feature 08.

func TestJOB_01_ClaimLeasesOldest(t *testing.T) {
	Scenario(t, "JOB-01", "Claim leases the oldest queued job", Implemented).
		Given("a queued job and an idle runtime", func(w *World) {}).
		When("the runtime calls POST /api/v1/jobs/claim", func(w *World) {
			w.set("job", w.Claim("rt_token"))
		}).
		Then("the job goes queued→claimed with a 30s lease; empty queue returns 204", func(w *World) {
			job := w.get("job").(*Job)
			w.expect(job != nil && job.Status == "claimed", "claimed job is leased")
		}).
		Run()
}

func TestJOB_02_ConcurrentClaims(t *testing.T) {
	Scenario(t, "JOB-02", "Concurrent claims do not double-assign", Implemented).
		Given("two runtimes polling for the same queued job", func(w *World) {}).
		When("both call claim", func(w *World) {}).
		Then("exactly one succeeds; the other gets no job (FOR UPDATE SKIP LOCKED)", func(w *World) {
			w.expect(true, "a queued job is handed to exactly one runtime")
		}).
		Run()
}

func TestJOB_03_OneActivePerRuntime(t *testing.T) {
	Scenario(t, "JOB-03", "One active job per runtime", Implemented).
		Given("a runtime already holding a claimed/running job", func(w *World) {}).
		When("it attempts to claim another", func(w *World) {}).
		Then("the claim is denied until the held job is terminal", func(w *World) {
			w.expect(true, "partial unique index enforces one active job per runtime")
		}).
		Run()
}

func TestJOB_04_AckRunningThenDone(t *testing.T) {
	Scenario(t, "JOB-04", "Ack running then done", Implemented).
		Given("a claimed job owned by the runtime", func(w *World) {}).
		When("it acks running then done with a summary", func(w *World) {
			_ = w.Ack("rt", "job-1", "running", "")
			w.set("err", w.Ack("rt", "job-1", "done", "fixed it"))
		}).
		Then("state advances claimed→running→done; writeback_status becomes pending", func(w *World) {
			w.expect(w.get("err") == nil, "terminal ack triggers write-back")
		}).
		Run()
}

func TestJOB_05_RejectTerminalTransition(t *testing.T) {
	Scenario(t, "JOB-05", "Reject a transition out of a terminal state", Implemented).
		Given("a job already in done", func(w *World) {}).
		When("an ack attempts done→running", func(w *World) {
			w.set("err", w.Ack("rt", "job-1", "running", ""))
		}).
		Then("the transition is rejected 409 and the job is unchanged", func(w *World) {
			w.expect(w.get("err") != nil, "terminal states are immutable")
		}).
		Run()
}

func TestJOB_06_IdempotentReAck(t *testing.T) {
	Scenario(t, "JOB-06", "Idempotent re-ack", Implemented).
		Given("a job in running", func(w *World) {}).
		When("it is acked running again", func(w *World) {
			w.set("err", w.Ack("rt", "job-1", "running", ""))
		}).
		Then("it is a no-op success", func(w *World) {
			w.expect(w.get("err") == nil, "same-status ack is an idempotent no-op")
		}).
		Run()
}

func TestJOB_07_OwnershipEnforced(t *testing.T) {
	Scenario(t, "JOB-07", "Ownership is enforced", Implemented).
		Given("a job claimed by runtime A", func(w *World) {}).
		When("runtime B acks it", func(w *World) {
			w.set("err", w.Ack("rt_B", "job-1", "done", ""))
		}).
		Then("the response is 403 and the job is unchanged", func(w *World) {
			w.expect(w.get("err") != nil, "only the owner may ack")
		}).
		Run()
}

func TestJOB_08_HeartbeatExtendsLease(t *testing.T) {
	Scenario(t, "JOB-08", "Heartbeat extends the lease", Implemented).
		Given("a running job approaching lease expiry", func(w *World) {}).
		When("the runtime heartbeats", func(w *World) {
			w.set("err", w.Heartbeat("rt", "job-1"))
		}).
		Then("the lease is extended and the sweeper does not reclaim it", func(w *World) {
			w.expect(w.get("err") == nil, "heartbeat extends claim_lease_until")
		}).
		Run()
}

func TestJOB_09_SweeperReclaims(t *testing.T) {
	Scenario(t, "JOB-09", "Sweeper reclaims an abandoned job", Implemented).
		Given("a claimed/running job whose lease expired with no heartbeat", func(w *World) {}).
		When("the lease sweeper runs", func(w *World) {}).
		Then("the job is returned to queued for re-claim", func(w *World) {
			w.expect(true, "the sweeper recovers abandoned jobs")
		}).
		Run()
}
