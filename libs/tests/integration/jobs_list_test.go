package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/hub"
	"mework/libs/server/platform/store"
	"mework/libs/server/platform/token"
)

// jobListItem represents a single job in the list endpoint JSON response.
type jobListItem struct {
	ID           string  `json:"id"`
	ProviderCode string  `json:"provider_code"`
	Status       string  `json:"status"`
	Instructions string  `json:"instructions"`
	ResultSummary *string `json:"result_summary,omitempty"`
}

// TestJobsListEndpoint verifies the GET /api/v1/jobs?provider=&status=&since= endpoint.
func TestJobsListEndpoint(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping job list integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer pool.Close()

	// Clean DB
	if _, err := pool.Exec(ctx,
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM accounts;`,
	); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	// Setup
	serverKey := "test-server-key-16chars"
	secretKey := "test-secret-key-16ch-x"

	// Seed account and runtime with known token_lookup.
	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('List Test Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	rtToken := "rt_list-test-token"
	tokenLookup := token.ComputeLookup(rtToken, serverKey)

	var runtimeID string
	err = pool.QueryRow(ctx, `
		INSERT INTO runtimes (account_id, code, label, token_lookup)
		VALUES ($1, 'list-test', 'List Test Worker', $2)
		RETURNING id
	`, accountID, tokenLookup).Scan(&runtimeID)
	if err != nil {
		t.Fatalf("seed runtime: %v", err)
	}

	// Start the full hub server (the list route is NOT registered yet in RED).
	cfg := &hub.Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		ServerKey:       serverKey,
		MeworkSecretKey: secretKey,
	}
	srv := hub.NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	listURL := httpSrv.URL + "/api/v1/jobs"

	// -----------------------------------------------------------------------
	// Helper: seed a job with explicit fields, return its ID.
	// -----------------------------------------------------------------------
	seedJob := func(provider, status string, createdAt time.Time, resultSummary *string) string {
		t.Helper()
		var jobID string
		err := pool.QueryRow(ctx, `
			INSERT INTO jobs
				(account_id, runtime_id, external_task_id, external_event_id, provider_code,
				 status, instructions, task_title, task_description, ttl_expires_at,
				 result_summary, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, '', '', NOW() + INTERVAL '1 day', $8, $9)
			RETURNING id
		`, accountID, runtimeID,
			"ch_"+provider, "evt-"+provider+"-"+status,
			provider, status, "instructions for "+provider+"/"+status,
			resultSummary, createdAt,
		).Scan(&jobID)
		if err != nil {
			t.Fatalf("seed job (%s/%s): %v", provider, status, err)
		}
		return jobID
	}

	// -----------------------------------------------------------------------
	// Helper: GET /api/v1/jobs and decode the response.
	// -----------------------------------------------------------------------
	getJobs := func(token, queryParams string) (int, []jobListItem) {
		t.Helper()
		url := listURL
		if queryParams != "" {
			url += "?" + queryParams
		}
		req, _ := http.NewRequest("GET", url, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("list request failed: %v", err)
		}
		defer resp.Body.Close()

		var items []jobListItem
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
				t.Fatalf("decode response: %v", err)
			}
		}
		return resp.StatusCode, items
	}

	// -----------------------------------------------------------------------
	// Subtest: list with no matches returns empty array (runs before seeding)
	// -----------------------------------------------------------------------
	t.Run("list with no matches returns empty array", func(t *testing.T) {
		statusCode, items := getJobs(rtToken, "provider=mezon")
		if statusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", statusCode)
		}
		if items == nil {
			t.Fatal("expected empty JSON array [], got null")
		}
		if len(items) != 0 {
			t.Errorf("expected 0 items, got %d", len(items))
		}
	})

	// -----------------------------------------------------------------------
	// Seed a base set of jobs with staggered created_at for the remaining cases.
	//   job1: mezon, queued   (t - 3s)
	//   job2: mezon, done     (t - 2s)
	//   job3: mello, done     (t - 1s)
	// -----------------------------------------------------------------------
	now := time.Now()
	job1ID := seedJob("mezon", "queued", now.Add(-3*time.Second), nil)
	job2ID := seedJob("mezon", "done", now.Add(-2*time.Second), strPtr("mezon result"))
	job3ID := seedJob("mello", "done", now.Add(-1*time.Second), strPtr("mello result"))

	// -----------------------------------------------------------------------
	// Table-driven list query cases
	// -----------------------------------------------------------------------
	listCases := []struct {
		name        string
		token       string
		queryParams string
		wantStatus  int
		wantIDs     []string // expected job IDs in order; empty = skip ID check
		wantCount   int      // -1 = skip count check; 0+ = exact
	}{
		{
			name:        "list by provider",
			token:       rtToken,
			queryParams: "provider=mezon",
			wantStatus:  http.StatusOK,
			wantIDs:     []string{job1ID, job2ID},
			wantCount:   2,
		},
		{
			name:        "list by status",
			token:       rtToken,
			queryParams: "status=done",
			wantStatus:  http.StatusOK,
			wantIDs:     []string{job2ID, job3ID},
			wantCount:   2,
		},
		{
			name:        "list by provider + status",
			token:       rtToken,
			queryParams: "provider=mezon&status=done",
			wantStatus:  http.StatusOK,
			wantIDs:     []string{job2ID},
			wantCount:   1,
		},
		{
			name:        "list with since cursor",
			token:       rtToken,
			queryParams: "since=" + job2ID,
			wantStatus:  http.StatusOK,
			wantIDs:     []string{job3ID},
			wantCount:   1,
		},
		{
			name:        "list with invalid status returns 400",
			token:       rtToken,
			queryParams: "status=invalid",
			wantStatus:  http.StatusBadRequest,
			wantCount:   -1,
		},
		{
			name:        "unauthenticated request returns 401",
			token:       "",
			queryParams: "",
			wantStatus:  http.StatusUnauthorized,
			wantCount:   -1,
		},
	}

	for _, tc := range listCases {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, items := getJobs(tc.token, tc.queryParams)

			if statusCode != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, statusCode)
			}

			if tc.wantCount >= 0 && len(items) != tc.wantCount {
				t.Errorf("expected %d items, got %d", tc.wantCount, len(items))
			}

			if len(tc.wantIDs) > 0 {
				if len(items) != len(tc.wantIDs) {
					t.Fatalf("expected %d items, got %d", len(tc.wantIDs), len(items))
				}
				for i, wantID := range tc.wantIDs {
					if items[i].ID != wantID {
						t.Errorf("item[%d].ID = %q, want %q", i, items[i].ID, wantID)
					}
				}
			}

			// When filtering by provider, verify every returned item has the
			// expected provider_code.
			if tc.queryParams == "provider=mezon" {
				for _, item := range items {
					if item.ProviderCode != "mezon" {
						t.Errorf("expected provider_code 'mezon', got %q", item.ProviderCode)
					}
				}
			}
		})
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
