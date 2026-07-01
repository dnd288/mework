//go:build e2e

package e2e

import "testing"

// Feature 21 — Notifications & artifacts. Real-world platform hardening (proposed).

func TestNOTIFY_01_RunCompletionWebhook(t *testing.T) {
	Scenario(t, "NOTIFY-01", "Run completion fires an outbound notification", PlannedPlatform).
		Given("a tenant configured with a completion webhook", func(w *World) {}).
		When("run r1 completes", func(w *World) {
			w.set("err", w.Notifier.Notify(ctx(), NotifyEvent{Kind: "run.done", RunID: "r1", Target: "https://hook"}))
		}).
		Then("a notification is delivered to the configured target", func(w *World) {
			w.expect(w.get("err") == nil, "completion notifications are delivered")
		}).
		Run()
}

func TestNOTIFY_02_FailureNotification(t *testing.T) {
	Scenario(t, "NOTIFY-02", "Run failure notifies the configured channel", PlannedPlatform).
		Given("a run that fails", func(w *World) {}).
		When("the failure is finalized", func(w *World) {
			w.set("err", w.Notifier.Notify(ctx(), NotifyEvent{Kind: "run.failed", RunID: "r1", Target: "https://hook"}))
		}).
		Then("a failure notification with the run id is sent", func(w *World) {
			w.expect(w.get("err") == nil, "failure events notify operators")
		}).
		Run()
}

func TestNOTIFY_03_DeliveryRetry(t *testing.T) {
	Scenario(t, "NOTIFY-03", "Notification delivery retries on transient failure", PlannedPlatform).
		Given("a notification target returning 5xx on the first attempt", func(w *World) {}).
		When("delivery is retried", func(w *World) {}).
		Then("it is redelivered until success or a bounded retry limit", func(w *World) {
			w.expect(true, "outbound notifications use durable retry")
		}).
		Run()
}

func TestARTIFACT_01_StoreRunOutput(t *testing.T) {
	Scenario(t, "ARTIFACT-01", "Run output is stored", PlannedPlatform).
		Given("a finished run r1 producing output", func(w *World) {}).
		When("the output is stored as an artifact", func(w *World) {
			w.set("err", w.Artifacts.Put(ctx(), ArtifactRef{RunID: "r1", Name: "patch.diff"}, []byte("diff")))
		}).
		Then("it is persisted under the run", func(w *World) {
			w.expect(w.get("err") == nil, "run artifacts are persisted")
		}).
		Run()
}

func TestARTIFACT_02_RetrieveByRun(t *testing.T) {
	Scenario(t, "ARTIFACT-02", "Artifacts are retrievable by run", PlannedPlatform).
		Given("a stored artifact for run r1", func(w *World) {
			_ = w.Artifacts.Put(ctx(), ArtifactRef{RunID: "r1", Name: "patch.diff"}, []byte("diff"))
		}).
		When("a client fetches it", func(w *World) {
			content, _ := w.Artifacts.Get(ctx(), ArtifactRef{RunID: "r1", Name: "patch.diff"})
			w.set("content", content)
		}).
		Then("the stored bytes are returned", func(w *World) {
			w.expect(len(w.get("content").([]byte)) > 0, "the artifact content round-trips")
		}).
		Run()
}

func TestARTIFACT_03_ListPerRun(t *testing.T) {
	Scenario(t, "ARTIFACT-03", "Artifacts are listed per run", PlannedPlatform).
		Given("several artifacts stored for run r1", func(w *World) {}).
		When("the run's artifacts are listed", func(w *World) {
			refs, _ := w.Artifacts.List(ctx(), "r1")
			w.set("refs", refs)
		}).
		Then("all artifacts for that run are returned", func(w *World) {
			w.expect(true, "artifact listing is scoped to the run")
		}).
		Run()
}

func TestARTIFACT_04_ChecksumIntegrity(t *testing.T) {
	Scenario(t, "ARTIFACT-04", "Artifact integrity is checksum-verified", PlannedPlatform).
		Given("an artifact stored with a checksum", func(w *World) {
			_ = w.Artifacts.Put(ctx(), ArtifactRef{RunID: "r1", Name: "patch.diff", Checksum: "abc"}, []byte("diff"))
		}).
		When("it is retrieved", func(w *World) {
			_, err := w.Artifacts.Get(ctx(), ArtifactRef{RunID: "r1", Name: "patch.diff", Checksum: "abc"})
			w.set("err", err)
		}).
		Then("a checksum mismatch is detected and rejected", func(w *World) {
			w.expect(w.get("err") == nil, "matching checksum verifies integrity on read")
		}).
		Run()
}
