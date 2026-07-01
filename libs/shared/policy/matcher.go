package policy

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// matchAttribute reports whether `value` matches `pattern`. Supported patterns:
//
//   "*"              — matches anything
//   "true"/"false"  — boolean equality
//   ">N", "<N", ">=N", "<=N" — numeric comparison
//   "HH:MM-HH:MM"   — UTC time range (start-inclusive, end-exclusive; wraps past midnight)
//   "/regex/"       — regular expression
//   "glob"           — glob pattern (default, no prefix)
func matchAttribute(value, pattern string) (bool, error) {
	// Wildcard.
	if pattern == "*" {
		return true, nil
	}

	// Boolean.
	if pattern == "true" || pattern == "false" {
		return value == pattern, nil
	}

	// Numeric comparison.
	if len(pattern) > 1 && (pattern[0] == '>' || pattern[0] == '<' || pattern[0] == '=') {
		return matchNumeric(value, pattern)
	}

	// Time range.
	if strings.Contains(pattern, "-") && strings.Contains(pattern, ":") {
		return matchTimeRange(value, pattern)
	}

	// Regex (enclosed in /).
	if len(pattern) > 2 && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
		re, err := regexp.Compile(pattern[1 : len(pattern)-1])
		if err != nil {
			return false, fmt.Errorf("bad regex %q: %w", pattern, err)
		}
		return re.MatchString(value), nil
	}

	// Default: glob.
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		return false, fmt.Errorf("bad glob %q: %w", pattern, err)
	}
	return matched, nil
}

// matchNumeric compares value (parsed as float64) against a numeric pattern.
func matchNumeric(value, pattern string) (bool, error) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false, nil // non-numeric value never matches a numeric pattern
	}

	// Extract operator and bound.
	op := ""
	boundStr := pattern
	for _, prefix := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(pattern, prefix) {
			op = prefix
			boundStr = strings.TrimPrefix(pattern, prefix)
			break
		}
	}
	if op == "" {
		return false, nil
	}

	bound, err := strconv.ParseFloat(strings.TrimSpace(boundStr), 64)
	if err != nil {
		return false, nil
	}

	switch op {
	case ">":
		return v > bound, nil
	case "<":
		return v < bound, nil
	case ">=":
		return v >= bound, nil
	case "<=":
		return v <= bound, nil
	}
	return false, nil
}

// matchTimeRange checks whether the current UTC time falls within the range
// specified by pattern ("HH:MM-HH:MM"). The range wraps past midnight.
func matchTimeRange(value, pattern string) (bool, error) {
	parts := strings.SplitN(pattern, "-", 2)
	if len(parts) != 2 {
		return false, nil
	}

	start, err := time.Parse("15:04", parts[0])
	if err != nil {
		return false, nil
	}
	end, err := time.Parse("15:04", parts[1])
	if err != nil {
		return false, nil
	}

	// Use the value time if provided and parseable, otherwise use current UTC.
	var t time.Time
	if value != "" {
		t, err = time.Parse(time.RFC3339, value)
		if err != nil {
			t = time.Now().UTC()
		}
	} else {
		t = time.Now().UTC()
	}

	// Normalise to today's date for comparison.
	now := time.Date(0, 1, 1, t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
	startT := time.Date(0, 1, 1, start.Hour(), start.Minute(), start.Second(), 0, time.UTC)
	endT := time.Date(0, 1, 1, end.Hour(), end.Minute(), end.Second(), 0, time.UTC)

	if endT.Before(startT) || endT.Equal(startT) {
		// Range wraps past midnight: e.g. 22:00-06:00
		return !now.Before(startT) || now.Before(endT), nil
	}
	return !now.Before(startT) && now.Before(endT), nil
}
