package daemon

import (
	"encoding/json"
	"os"
	"sync"

	"mework/internal/cli"
)

// ticketState records which trigger comments have been handled for one ticket,
// plus the run status. handled_comment_ids is a SET (map→struct{}) because
// comment ids may be UUIDs (non-monotonic), so we cannot use a max-id cursor.
type ticketState struct {
	HandledCommentIDs map[string]struct{} `json:"handled_comment_ids"`
	Status            string              `json:"status"`
	LastRunAt         string              `json:"last_run_at,omitempty"`
}

// State is the daemon's persisted trigger-idempotency cache.
type State struct {
	mu      sync.Mutex
	profile string
	Tickets map[string]*ticketState `json:"tickets"`
}

// LoadState reads the state cache for a profile (empty cache if absent).
func LoadState(profile string) (*State, error) {
	s := &State{profile: profile, Tickets: map[string]*ticketState{}}
	data, err := os.ReadFile(cli.StatePath(profile))
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Tickets == nil {
		s.Tickets = map[string]*ticketState{}
	}
	s.profile = profile
	return s, nil
}

// Handled reports whether a ticket's comment has already triggered a run.
func (s *State) Handled(ticketID, commentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := s.Tickets[ticketID]
	if ts == nil {
		return false
	}
	_, ok := ts.HandledCommentIDs[commentID]
	return ok
}

// Mark records a comment as handled with the given status and persists the
// cache. Marking happens BEFORE the agent runs (status "in_progress") so a
// crash mid-run does not cause a re-trigger on restart.
func (s *State) Mark(ticketID, commentID, status, when string) error {
	s.mu.Lock()
	ts := s.Tickets[ticketID]
	if ts == nil {
		ts = &ticketState{HandledCommentIDs: map[string]struct{}{}}
		s.Tickets[ticketID] = ts
	}
	if ts.HandledCommentIDs == nil {
		ts.HandledCommentIDs = map[string]struct{}{}
	}
	ts.HandledCommentIDs[commentID] = struct{}{}
	ts.Status = status
	if when != "" {
		ts.LastRunAt = when
	}
	s.mu.Unlock()
	return s.save()
}

// save writes the cache to disk with private permissions. It must NOT hold the
// mutex while marshaling, because MarshalJSON acquires the same mutex.
func (s *State) save() error {
	if err := os.MkdirAll(cli.ProfileDir(s.profile), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cli.StatePath(s.profile), data, 0o600)
}

// MarshalJSON omits the mutex and profile (unexported already skipped) but we
// implement it to serialize only the Tickets map under a stable key.
func (s *State) MarshalJSON() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(struct {
		Tickets map[string]*ticketState `json:"tickets"`
	}{Tickets: s.Tickets})
}
