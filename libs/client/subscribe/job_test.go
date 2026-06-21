package subscribe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJobClient(t *testing.T) {
	rtToken := "rt_test_token_123"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHeader := r.Header.Get("Authorization")
		if tokenHeader != "Bearer "+rtToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/api/v1/jobs/claim":
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			job := Job{
				ID:             "job-1",
				ExternalTaskID: "task-1",
				Instructions:   "run something",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(job)

		case "/api/v1/jobs/job-1/ack":
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req AckRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Status != "done" || *req.ResultSummary != "success info" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case "/api/v1/jobs/job-1/heartbeat":
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	client := NewClient(mockServer.URL, 1*time.Second)

	// 1. Test Claim
	job, err := client.Claim(rtToken)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if job == nil || job.ID != "job-1" || job.ExternalTaskID != "task-1" {
		t.Errorf("unexpected job returned: %+v", job)
	}

	// 2. Test Ack
	err = client.Ack(rtToken, "job-1", "done", "success info", "")
	if err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// 3. Test Heartbeat
	err = client.Heartbeat(rtToken, "job-1")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}
