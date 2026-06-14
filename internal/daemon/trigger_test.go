package daemon

import (
	"testing"
	"time"

	"mework/internal/mello"
)

func ts(offset int) *time.Time {
	t := time.Date(2026, 1, 1, 0, 0, offset, 0, time.UTC)
	return &t
}

func TestFindTriggersMatchesKeyword(t *testing.T) {
	comments := []mello.Comment{
		{ID: "c1", UserID: "user", Body: "please /run this", CreatedAt: ts(1)},
		{ID: "c2", UserID: "user", Body: "no keyword here", CreatedAt: ts(2)},
	}
	got := findTriggers(comments, "/run", "self")
	if len(got) != 1 || got[0].Comment.ID != "c1" {
		t.Fatalf("expected only c1, got %+v", got)
	}
}

func TestFindTriggersSkipsOwnComments(t *testing.T) {
	// The daemon's own start/done comments may contain the keyword; they must
	// never re-trigger a run (infinite-loop guard).
	comments := []mello.Comment{
		{ID: "c1", UserID: "self", Body: "🤖 Agent started: /run echoed", CreatedAt: ts(1)},
		{ID: "c2", UserID: "human", Body: "/run again", CreatedAt: ts(2)},
	}
	got := findTriggers(comments, "/run", "self")
	if len(got) != 1 || got[0].Comment.ID != "c2" {
		t.Fatalf("expected only human comment c2, got %+v", got)
	}
}

func TestFindTriggersOrdersByCreatedAt(t *testing.T) {
	comments := []mello.Comment{
		{ID: "late", UserID: "u", Body: "/run", CreatedAt: ts(9)},
		{ID: "early", UserID: "u", Body: "/run", CreatedAt: ts(1)},
	}
	got := findTriggers(comments, "/run", "self")
	if len(got) != 2 || got[0].Comment.ID != "early" || got[1].Comment.ID != "late" {
		t.Fatalf("expected early before late, got %+v", got)
	}
}

func TestStateHandledIdempotency(t *testing.T) {
	t.Setenv("MEWORK_HOME", t.TempDir())
	s, err := LoadState("p")
	if err != nil {
		t.Fatal(err)
	}
	if s.Handled("t1", "c1") {
		t.Fatal("should not be handled before Mark")
	}
	if err := s.Mark("t1", "c1", "in_progress", "now"); err != nil {
		t.Fatal(err)
	}
	if !s.Handled("t1", "c1") {
		t.Fatal("should be handled after Mark")
	}
	// Reload from disk: persistence must survive a restart.
	s2, err := LoadState("p")
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Handled("t1", "c1") {
		t.Fatal("handled state did not persist across reload")
	}
}
