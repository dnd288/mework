package orchestrator

import (
	"context"
	"sort"
	"sync"

	"mework/libs/shared/ports"
)

// RunnerStatus represents the current online status of a runner.
type RunnerStatus string

const (
	RunnerOnline   RunnerStatus = "online"
	RunnerDraining RunnerStatus = "draining"
	RunnerOffline  RunnerStatus = "offline"
)

// RunnerInfo holds the eligibility state for a single runner.
type RunnerInfo struct {
	RunnerID         string
	TenantID         string
	Status           RunnerStatus
	ActiveDispatches int
	EnrolledGrants   []string
}

// sessionBinding records which runner a session is bound to.
type sessionBinding struct {
	runnerID string
}

// RunnerSelectorImpl implements ports.RunnerSelector with an in-memory
// eligible-runner index. It load-balances across eligible online runners
// and honours session affinity.
type RunnerSelectorImpl struct {
	mu sync.RWMutex

	// runnersByTenant maps tenantID -> runnerID -> RunnerInfo
	runnersByTenant map[string]map[string]*RunnerInfo

	// sessionAffinity maps sessionID -> bound runnerID
	sessionAffinity map[string]string

	// activeDispatchCount tracks the number of active dispatches per runner.
	activeDispatchCount map[string]int
}

// NewRunnerSelector creates a new in-memory RunnerSelectorImpl.
func NewRunnerSelector() *RunnerSelectorImpl {
	return &RunnerSelectorImpl{
		runnersByTenant:     make(map[string]map[string]*RunnerInfo),
		sessionAffinity:     make(map[string]string),
		activeDispatchCount: make(map[string]int),
	}
}

// Select returns the ID of the best eligible runner for the given tenant
// and criteria. It implements ports.RunnerSelector.
func (s *RunnerSelectorImpl) Select(ctx context.Context, tenant string, criteria ports.SelectionCriteria) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Session affinity: if a session is already bound to an eligible runner, use it.
	if criteria.SessionID != "" {
		if boundRunnerID, ok := s.sessionAffinity[criteria.SessionID]; ok {
			runners, ok := s.runnersByTenant[tenant]
			if ok {
				if info, exists := runners[boundRunnerID]; exists && info.Status == RunnerOnline {
					return boundRunnerID, nil
				}
			}
		}
	}

	// 2. Load-balance: pick the eligible runner with the fewest active dispatches.
	runners, ok := s.runnersByTenant[tenant]
	if !ok || len(runners) == 0 {
		return "", ErrNoEligibleRunner
	}

	type candidate struct {
		id    string
		count int
	}

	var candidates []candidate
	for id, info := range runners {
		if info.Status != RunnerOnline {
			continue
		}
		if info.ActiveDispatches >= 1 {
			continue // per-runner concurrency cap
		}
		count := s.activeDispatchCount[id]
		candidates = append(candidates, candidate{id: id, count: count})
	}

	if len(candidates) == 0 {
		return "", ErrNoEligibleRunner
	}

	// Sort by active count ascending, then by runner ID for determinism.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count < candidates[j].count
		}
		return candidates[i].id < candidates[j].id
	})

	best := candidates[0].id

	// Record session affinity if a session ID is present.
	if criteria.SessionID != "" {
		s.sessionAffinity[criteria.SessionID] = best
	}

	// Increment active dispatch count.
	s.activeDispatchCount[best]++

	return best, nil
}

// UpdateRunnerStatus updates the status of a runner in the index.
func (s *RunnerSelectorImpl) UpdateRunnerStatus(tenantID, runnerID string, status RunnerStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.runnersByTenant[tenantID]; !ok {
		s.runnersByTenant[tenantID] = make(map[string]*RunnerInfo)
	}

	info, exists := s.runnersByTenant[tenantID][runnerID]
	if !exists {
		info = &RunnerInfo{
			RunnerID: runnerID,
			TenantID: tenantID,
		}
		s.runnersByTenant[tenantID][runnerID] = info
	}
	info.Status = status
}

// SetEnrolledGrants sets the grants that a runner is enrolled with.
func (s *RunnerSelectorImpl) SetEnrolledGrants(tenantID, runnerID string, grants []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.runnersByTenant[tenantID]; !ok {
		s.runnersByTenant[tenantID] = make(map[string]*RunnerInfo)
	}

	info, exists := s.runnersByTenant[tenantID][runnerID]
	if !exists {
		info = &RunnerInfo{
			RunnerID: runnerID,
			TenantID: tenantID,
		}
		s.runnersByTenant[tenantID][runnerID] = info
	}
	info.EnrolledGrants = grants
}

// BindSession records a session-to-runner binding for session affinity.
func (s *RunnerSelectorImpl) BindSession(sessionID, runnerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionAffinity[sessionID] = runnerID
}

// UnbindSession removes a session-to-runner binding.
func (s *RunnerSelectorImpl) UnbindSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionAffinity, sessionID)
}

// ReleaseDispatch decrements the active dispatch count for a runner
// when a dispatch completes or fails.
func (s *RunnerSelectorImpl) ReleaseDispatch(runnerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.activeDispatchCount[runnerID]; ok {
		s.activeDispatchCount[runnerID]--
		if s.activeDispatchCount[runnerID] < 0 {
			s.activeDispatchCount[runnerID] = 0
		}
	}
}

// ListEligibleRunners returns all online eligible runners for a tenant.
func (s *RunnerSelectorImpl) ListEligibleRunners(tenantID string) []RunnerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []RunnerInfo
	runners, ok := s.runnersByTenant[tenantID]
	if !ok {
		return result
	}
	for _, info := range runners {
		if info.Status == RunnerOnline {
			result = append(result, *info)
		}
	}
	return result
}

// RemoveRunner removes a runner from the index.
func (s *RunnerSelectorImpl) RemoveRunner(tenantID, runnerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if runners, ok := s.runnersByTenant[tenantID]; ok {
		delete(runners, runnerID)
	}
	delete(s.activeDispatchCount, runnerID)
}

// compile-time check that RunnerSelectorImpl implements ports.RunnerSelector
var _ ports.RunnerSelector = (*RunnerSelectorImpl)(nil)
