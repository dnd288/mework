// Package scheduler manages time-based dispatches of agents. It supports
// cron, interval, and one-shot run-at schedules with pause/resume/cancel
// lifecycle, timezone-aware cron evaluation, and a missed-fire policy.
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/catalog"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
)

// Sentinel errors.
var (
	ErrScheduleNotFound = errors.New("schedule not found")
	ErrNotActive        = errors.New("schedule is not active; must be active to fire")
	ErrNotPaused        = errors.New("schedule is not paused")
	ErrInvalidSpec      = errors.New("invalid schedule spec")
)

// scheduleRow mirrors the database row for a schedule.
type scheduleRow struct {
	ID        string
	TenantID  string
	Kind      string
	Cron      *string
	Every     *string
	At        *string
	TZ        string
	Agent     string
	Target    string
	GrantData []byte
	Missed    string
	State     string
	NextFire  *time.Time
	LastFire  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Service implements the core scheduling logic backed by PostgreSQL.
type Service struct {
	pool          *pgxpool.Pool
	agentHandlers *catalog.AgentHandlers

	mu      sync.Mutex
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running bool
}

// NewService creates a new scheduler Service.
func NewService(pool *pgxpool.Pool, agentHandlers *catalog.AgentHandlers) *Service {
	return &Service{
		pool:          pool,
		agentHandlers: agentHandlers,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the background fire loop that polls for due schedules every 10s.
func (s *Service) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	s.wg.Add(1)
	go s.fireLoop()
}

// Stop terminates the background fire loop.
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.wg.Wait()
}

// fireLoop polls the database for due schedules and fires them.
func (s *Service) fireLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processDueSchedules()
		}
	}
}

// processDueSchedules finds and fires all schedules whose next_fire <= now.
func (s *Service) processDueSchedules() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, kind, cron, every, at, tz, agent, target, grant_data, missed, state, next_fire, last_fire, created_at, updated_at
		FROM schedules
		WHERE state = 'active' AND next_fire <= NOW()
		ORDER BY next_fire ASC
		LIMIT 50
	`)
	if err != nil {
		return // log would go here
	}
	defer rows.Close()

	for rows.Next() {
		var r scheduleRow
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Kind, &r.Cron, &r.Every, &r.At, &r.TZ, &r.Agent, &r.Target, &r.GrantData, &r.Missed, &r.State, &r.NextFire, &r.LastFire, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		s.fireSchedule(ctx, r)
	}
}

// fireSchedule dispatches the agent for a schedule at its due fire time.
func (s *Service) fireSchedule(ctx context.Context, row scheduleRow) {
	now := time.Now().UTC()

	// If the next fire time has already passed by more than a grace period (30s),
	// check the missed-fire policy.
	if row.NextFire != nil && now.Sub(*row.NextFire) > 30*time.Second {
		switch core.MissedPolicy(row.Missed) {
		case core.MissedCatchUp:
			// Proceed to fire — catch-up coalesces missed fires into one dispatch.
		case core.MissedSkip:
			// Skip this missed fire: advance the schedule without dispatching.
			s.advanceSchedule(ctx, row, now)
			return
		default:
			// Unknown policy — skip to be safe.
			s.advanceSchedule(ctx, row, now)
			return
		}
	}

	// Build the dispatch and publish it.
	agentName, agentVersion := parseAgentRef(row.Agent)

	g := &grant.Grant{}
	if len(row.GrantData) > 0 {
		if err := json.Unmarshal(row.GrantData, g); err != nil {
			// Invalid grant data — skip this fire.
			s.advanceSchedule(ctx, row, now)
			return
		}
	}

	if agentVersion != "" {
		if err := s.agentHandlers.DispatchVersionToRunner(ctx, agentName, agentVersion, row.Target, g); err != nil {
			// Dispatch failed — try again next poll cycle.
			return
		}
	} else {
		if err := s.agentHandlers.DispatchToRunner(ctx, agentName, row.Target, g); err != nil {
			return
		}
	}

	s.advanceSchedule(ctx, row, now)
}

// advanceSchedule computes the next fire time and updates the schedule row.
// For a one-shot "at" schedule, it marks the schedule as canceled (completed).
func (s *Service) advanceSchedule(ctx context.Context, row scheduleRow, firedAt time.Time) {
	switch core.ScheduleKind(row.Kind) {
	case core.ScheduleCron, core.ScheduleInterval:
		next := s.computeNextFire(row, firedAt)
		if next == nil {
			// No next fire — mark as canceled (terminal).
			_, _ = s.pool.Exec(ctx, `
				UPDATE schedules SET state = 'canceled', last_fire = $1, updated_at = NOW()
				WHERE id = $2
			`, firedAt, row.ID)
			return
		}
		_, _ = s.pool.Exec(ctx, `
			UPDATE schedules SET next_fire = $1, last_fire = $2, updated_at = NOW()
			WHERE id = $3
		`, *next, firedAt, row.ID)

	case core.ScheduleAt:
		// One-shot: fire once then complete.
		_, _ = s.pool.Exec(ctx, `
			UPDATE schedules SET state = 'canceled', last_fire = $1, updated_at = NOW()
			WHERE id = $2
		`, firedAt, row.ID)

	default:
		// Unknown kind — cancel to avoid spin.
		_, _ = s.pool.Exec(ctx, `
			UPDATE schedules SET state = 'canceled', updated_at = NOW()
			WHERE id = $1
		`, row.ID)
	}
}

// computeNextFire computes the next fire time after the given reference time
// based on the schedule's kind and parameters.
func (s *Service) computeNextFire(row scheduleRow, after time.Time) *time.Time {
	switch core.ScheduleKind(row.Kind) {
	case core.ScheduleCron:
		if row.Cron == nil || *row.Cron == "" {
			return nil
		}
		loc := time.UTC
		if row.TZ != "" {
			if l, err := time.LoadLocation(row.TZ); err == nil {
				loc = l
			}
		}
		next, err := nextCronFire(*row.Cron, after.In(loc))
		if err != nil {
			return nil
		}
		return &next

	case core.ScheduleInterval:
		if row.Every == nil || *row.Every == "" {
			return nil
		}
		d, err := time.ParseDuration(*row.Every)
		if err != nil {
			return nil
		}
		next := after.Add(d)
		return &next

	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Public API: Schedule, Pause, Resume, Cancel, List, Get
// ---------------------------------------------------------------------------

// Schedule creates a new schedule and persists it in the active state.
func (s *Service) Schedule(ctx context.Context, tenantID string, spec core.ScheduleSpec) (string, error) {
	if err := validateSpec(spec); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSpec, err)
	}

	now := time.Now().UTC()
	nextFire := s.computeInitialFire(spec, now)

	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO schedules (tenant_id, kind, cron, every, at, tz, agent, target, grant_data, missed, state, next_fire)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'active', $11)
		RETURNING id
	`, tenantID, string(spec.Kind), nullString(spec.Cron), nullString(spec.Every), nullString(spec.At), spec.TZ, spec.Agent, spec.Target, spec.Grant, string(spec.Missed), nextFire).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert schedule: %w", err)
	}
	return id, nil
}

// Pause suppresses fires for a schedule without discarding it.
func (s *Service) Pause(ctx context.Context, tenantID, scheduleID string) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE schedules SET state = 'paused', updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND state = 'active'
	`, scheduleID, tenantID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

// Resume re-arms a paused schedule so it becomes eligible to fire again.
func (s *Service) Resume(ctx context.Context, tenantID, scheduleID string) error {
	// Fetch the paused schedule to recompute next_fire.
	var row scheduleRow
	err := s.pool.QueryRow(ctx, `
		SELECT id, kind, cron, every, at, tz, agent, target, grant_data, missed, state
		FROM schedules WHERE id = $1 AND tenant_id = $2 AND state = 'paused'
	`, scheduleID, tenantID).Scan(&row.ID, &row.Kind, &row.Cron, &row.Every, &row.At, &row.TZ, &row.Agent, &row.Target, &row.GrantData, &row.Missed, &row.State)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrScheduleNotFound
		}
		return err
	}

	spec := core.ScheduleSpec{
		Kind:   core.ScheduleKind(row.Kind),
		Cron:   nullableString(row.Cron),
		Every:  nullableString(row.Every),
		At:     nullableString(row.At),
		TZ:     row.TZ,
		Agent:  row.Agent,
		Target: row.Target,
		Grant:  row.GrantData,
		Missed: core.MissedPolicy(row.Missed),
	}
	nextFire := s.computeInitialFire(spec, time.Now().UTC())

	_, err = s.pool.Exec(ctx, `
		UPDATE schedules SET state = 'active', next_fire = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3
	`, nextFire, scheduleID, tenantID)
	if err != nil {
		return err
	}
	return nil
}

// Cancel permanently removes a schedule.
func (s *Service) Cancel(ctx context.Context, tenantID, scheduleID string) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE schedules SET state = 'canceled', updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND state IN ('active', 'paused')
	`, scheduleID, tenantID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

// List returns all non-canceled schedule IDs for the given tenant.
func (s *Service) List(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM schedules WHERE tenant_id = $1 AND state != 'canceled' ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// Get returns the schedule spec and state for a given schedule ID.
func (s *Service) Get(ctx context.Context, tenantID, scheduleID string) (*core.ScheduleSpec, core.ScheduleState, error) {
	var r scheduleRow
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, kind, cron, every, at, tz, agent, target, grant_data, missed, state, next_fire, last_fire, created_at, updated_at
		FROM schedules WHERE id = $1 AND tenant_id = $2
	`, scheduleID, tenantID).Scan(
		&r.ID, &r.TenantID, &r.Kind, &r.Cron, &r.Every, &r.At, &r.TZ, &r.Agent, &r.Target, &r.GrantData, &r.Missed, &r.State, &r.NextFire, &r.LastFire, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrScheduleNotFound
		}
		return nil, "", err
	}

	spec := &core.ScheduleSpec{
		Kind:   core.ScheduleKind(r.Kind),
		Cron:   nullableString(r.Cron),
		Every:  nullableString(r.Every),
		At:     nullableString(r.At),
		TZ:     r.TZ,
		Agent:  r.Agent,
		Target: r.Target,
		Grant:  r.GrantData,
		Missed: core.MissedPolicy(r.Missed),
	}
	return spec, core.ScheduleState(r.State), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// validateSpec checks that a ScheduleSpec has consistent fields per its Kind.
func validateSpec(spec core.ScheduleSpec) error {
	if spec.Agent == "" {
		return errors.New("agent is required")
	}
	if spec.Target == "" {
		return errors.New("target is required")
	}

	switch spec.Kind {
	case core.ScheduleCron:
		if spec.Cron == "" {
			return errors.New("cron expression is required for cron kind")
		}
		if _, err := parseCronFields(spec.Cron); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	case core.ScheduleInterval:
		if spec.Every == "" {
			return errors.New("every duration is required for interval kind")
		}
		if _, err := time.ParseDuration(spec.Every); err != nil {
			return fmt.Errorf("invalid every duration: %w", err)
		}
	case core.ScheduleAt:
		if spec.At == "" {
			return errors.New("at time is required for at kind")
		}
		if _, err := time.Parse(time.RFC3339, spec.At); err != nil {
			return fmt.Errorf("invalid at time (must be RFC3339): %w", err)
		}
	default:
		return fmt.Errorf("unknown schedule kind: %s", spec.Kind)
	}

	if spec.Missed == "" {
		spec.Missed = core.MissedSkip
	}
	switch spec.Missed {
	case core.MissedSkip, core.MissedCatchUp:
	default:
		return fmt.Errorf("unknown missed policy: %s", spec.Missed)
	}

	return nil
}

// computeInitialFire computes the first fire time for a new or resumed schedule.
func (s *Service) computeInitialFire(spec core.ScheduleSpec, now time.Time) *time.Time {
	switch spec.Kind {
	case core.ScheduleCron:
		loc := time.UTC
		if spec.TZ != "" {
			if l, err := time.LoadLocation(spec.TZ); err == nil {
				loc = l
			}
		}
		next, err := nextCronFire(spec.Cron, now.In(loc))
		if err != nil {
			return nil
		}
		return &next

	case core.ScheduleInterval:
		d, err := time.ParseDuration(spec.Every)
		if err != nil {
			return nil
		}
		next := now.Add(d)
		return &next

	case core.ScheduleAt:
		t, err := time.Parse(time.RFC3339, spec.At)
		if err != nil {
			return nil
		}
		// If the at time is in the past, skip it.
		if t.Before(now) {
			return nil
		}
		return &t

	default:
		return nil
	}
}

// parseAgentRef splits an agent reference like "code-fixer@1.2.0" into name and version.
// If there is no "@", version is empty.
func parseAgentRef(ref string) (name, version string) {
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}

// nullString returns a *string for use with nullable SQL columns.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullableString returns the string value of a *string, or empty if nil.
func nullableString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---------------------------------------------------------------------------
// Cron expression parsing and next-fire computation
// ---------------------------------------------------------------------------

// cronField holds the parsed bitset for one cron field.
type cronField struct {
	values []int // list of matching minute/hour/dom/month/dow values
	all    bool  // true if '*' (match all)
}

// cronExpr holds the parsed representation of a 5-field cron expression.
type cronExpr struct {
	minute cronField
	hour   cronField
	dom    cronField
	month  cronField
	dow    cronField
}

var cronFieldPattern = regexp.MustCompile(`^(\*|\d+(-\d+)?(/\d+)?)(,(\*|\d+(-\d+)?(/\d+)?))*$`)

// parseCronFields splits and validates a 5-field cron expression.
func parseCronFields(expr string) (cronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return cronExpr{}, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	var ce cronExpr
	var err error

	ce.minute, err = parseCronField(fields[0], 0, 59)
	if err != nil {
		return cronExpr{}, fmt.Errorf("minute: %w", err)
	}
	ce.hour, err = parseCronField(fields[1], 0, 23)
	if err != nil {
		return cronExpr{}, fmt.Errorf("hour: %w", err)
	}
	ce.dom, err = parseCronField(fields[2], 1, 31)
	if err != nil {
		return cronExpr{}, fmt.Errorf("day-of-month: %w", err)
	}
	ce.month, err = parseCronField(fields[3], 1, 12)
	if err != nil {
		return cronExpr{}, fmt.Errorf("month: %w", err)
	}
	ce.dow, err = parseCronField(fields[4], 0, 6)
	if err != nil {
		return cronExpr{}, fmt.Errorf("day-of-week: %w", err)
	}

	return ce, nil
}

// parseCronField parses a single cron field (e.g. "*", "5", "1-5", "*/15", "1,3,5").
func parseCronField(field string, min, max int) (cronField, error) {
	if field == "*" {
		return cronField{all: true}, nil
	}

	parts := strings.Split(field, ",")
	var values []int
	for _, part := range parts {
		if part == "" {
			continue
		}
		var (
			start, end int
			step       = 1
		)

		if strings.Contains(part, "/") {
			stepParts := strings.Split(part, "/")
			if len(stepParts) != 2 {
				return cronField{}, fmt.Errorf("invalid step: %s", part)
			}
			stepVal, err := strconv.Atoi(stepParts[1])
			if err != nil || stepVal <= 0 {
				return cronField{}, fmt.Errorf("invalid step value: %s", stepParts[1])
			}
			step = stepVal

			rangePart := stepParts[0]
			if rangePart == "*" {
				start = min
				end = max
			} else if strings.Contains(rangePart, "-") {
				rangeParts := strings.SplitN(rangePart, "-", 2)
				s, err1 := strconv.Atoi(rangeParts[0])
				e, err2 := strconv.Atoi(rangeParts[1])
				if err1 != nil || err2 != nil || s > e {
					return cronField{}, fmt.Errorf("invalid range: %s", rangePart)
				}
				start = s
				end = e
			} else {
				start, _ = strconv.Atoi(rangePart)
				end = max
			}
			for i := start; i <= end; i += step {
				values = append(values, i)
			}
		} else if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			s, err1 := strconv.Atoi(rangeParts[0])
			e, err2 := strconv.Atoi(rangeParts[1])
			if err1 != nil || err2 != nil || s > e {
				return cronField{}, fmt.Errorf("invalid range: %s", part)
			}
			start = s
			end = e
			for i := start; i <= end; i++ {
				values = append(values, i)
			}
		} else {
			v, err := strconv.Atoi(part)
			if err != nil {
				return cronField{}, fmt.Errorf("invalid value: %s", part)
			}
			values = append(values, v)
		}
	}
	return cronField{values: values}, nil
}

// matches reports whether a value is in this field's set or the field matches all.
func (f cronField) matches(v int) bool {
	if f.all {
		return true
	}
	for _, x := range f.values {
		if x == v {
			return true
		}
	}
	return false
}

// nextCronFire computes the next datetime matching the cron expression after 'after'.
func nextCronFire(expr string, after time.Time) (time.Time, error) {
	ce, err := parseCronFields(expr)
	if err != nil {
		return time.Time{}, err
	}

	// Start searching from the next minute.
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search up to 4 years ahead to avoid infinite loops.
	deadline := after.AddDate(4, 0, 0)

	for t.Before(deadline) {
		monthOK := ce.month.matches(int(t.Month()))
		if !monthOK {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		domOK := ce.dom.matches(t.Day())
		dowOK := ce.dow.matches(int(t.Weekday()))

		if domOK && ce.dom.all && !ce.dow.all {
			dowOK = true
		}
		if dowOK && ce.dow.all && !ce.dom.all {
			domOK = true
		}

		if monthOK && domOK && dowOK && ce.hour.matches(t.Hour()) && ce.minute.matches(t.Minute()) {
			return t, nil
		}

		t = t.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("no matching fire time found within 4 years")
}

// Ensure Service implements the port interface at compile time.
var _ interface {
	Schedule(context.Context, string, core.ScheduleSpec) (string, error)
	Pause(context.Context, string, string) error
	Resume(context.Context, string, string) error
	Cancel(context.Context, string, string) error
	List(context.Context, string) ([]string, error)
} = (*Service)(nil)
