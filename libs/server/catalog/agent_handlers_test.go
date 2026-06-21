package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
)

// setupTestRouter creates a chi Router with agent catalog handlers mounted,
// backed by an in-memory broker. It returns the router, the handlers instance,
// and the broker so tests can subscribe to verify dispatch messages.
func setupTestRouter(t *testing.T) (*chi.Mux, *AgentHandlers, *memory.MemoryBroker) {
	t.Helper()
	broker := memory.New()
	h := NewAgentHandlers(nil, broker, nil, nil)
	r := chi.NewRouter()
	r.Route("/api/v1/agents", func(r chi.Router) {
		r.Post("/{name}/versions", h.PublishVersion)
		r.Get("/", h.ListAgents)
		r.Get("/{name}", h.ResolveAgent)
		r.Get("/{name}/versions/{version}/pull", h.PullVersion)
		r.Post("/{name}/dispatch", h.Dispatch)
	})
	return r, h, broker.(*memory.MemoryBroker)
}

// publishVersionHelper is a test helper that publishes an agent version via the
// HTTP handler and returns the response recorder.
func publishVersionHelper(t *testing.T, router *chi.Mux, name, version, form, payload string) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]string{
		"version": version,
		"form":    form,
		"payload": payload,
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal publish request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+name+"/versions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandlers_PublishVersion(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	w := publishVersionHelper(t, router, "code-fixer", "1.2.0", "definition", "manifest content...")
	if w.Code != http.StatusCreated {
		t.Errorf("PublishVersion status = %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["version"] != "1.2.0" {
		t.Errorf("response version = %v, want %q", resp["version"], "1.2.0")
	}
	if resp["form"] != "definition" {
		t.Errorf("response form = %v, want %q", resp["form"], "definition")
	}

	// After publish, the agent should appear in the list.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("ListAgents status = %d, want %d", listW.Code, http.StatusOK)
	}
	var agents []map[string]any
	if err := json.NewDecoder(listW.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents list: %v", err)
	}
	found := false
	for _, a := range agents {
		if a["name"] == "code-fixer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("code-fixer not found in agent list after publish")
	}
}

func TestHandlers_PublishVersion_RejectDuplicate(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	// First publish succeeds.
	w1 := publishVersionHelper(t, router, "code-fixer", "1.2.0", "definition", "manifest")
	if w1.Code != http.StatusCreated {
		t.Fatalf("first publish: status = %d, want %d", w1.Code, http.StatusCreated)
	}

	// Second publish of the same version is rejected with 409.
	w2 := publishVersionHelper(t, router, "code-fixer", "1.2.0", "definition", "manifest")
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate publish: status = %d, want %d. Body: %s", w2.Code, http.StatusConflict, w2.Body.String())
	}
}

func TestHandlers_ResolveLatest(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	// Publish v1.0.0 then v2.0.0.
	publishVersionHelper(t, router, "code-fixer", "1.0.0", "definition", "v1 content")
	publishVersionHelper(t, router, "code-fixer", "2.0.0", "definition", "v2 content")

	// Resolve with ?version=latest should return v2.0.0.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/code-fixer?version=latest", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ResolveLatest status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	// The resolved version should be the concrete version string, not "latest".
	if v, ok := resp["version"].(string); !ok || v != "2.0.0" {
		t.Errorf("resolved version = %v, want %q", resp["version"], "2.0.0")
	}
}

func TestHandlers_ResolveByName(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	publishVersionHelper(t, router, "code-fixer", "1.0.0", "definition", "content")

	// Resolve by name without version returns the agent metadata and current pointer.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/code-fixer", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ResolveByName status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if name, ok := resp["name"].(string); !ok || name != "code-fixer" {
		t.Errorf("agent name = %v, want %q", resp["name"], "code-fixer")
	}
}

func TestHandlers_PullDefinitionForm(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	publishVersionHelper(t, router, "code-fixer", "1.2.0", "definition", "manifest content...")

	// Pull with a valid grant in the Authorization header.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/code-fixer/versions/1.2.0/pull", nil)
	req.Header.Set("Authorization", "Bearer rt_valid_runtime_token")
	req.Header.Set("X-Grant", `{"ops":["agent.pull"]}`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Pull status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode pull response: %v", err)
	}
	if resp["form"] != "definition" {
		t.Errorf("pull form = %v, want %q", resp["form"], "definition")
	}
	if content, ok := resp["content"].(string); !ok || content == "" {
		t.Errorf("pull content = %v, want non-empty", resp["content"])
	}
}

func TestHandlers_PullImageForm(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	// Publish an image-form agent.
	body := map[string]string{
		"version":   "2.0.0",
		"form":      "image",
		"reference": "docker.io/myorg/code-fixer:v2.0.0",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/img-agent/versions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish image-form failed: %d, body: %s", w.Code, w.Body.String())
	}

	// Pull the image-form agent.
	pullReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/img-agent/versions/2.0.0/pull", nil)
	pullReq.Header.Set("Authorization", "Bearer rt_valid_runtime_token")
	pullReq.Header.Set("X-Grant", `{"ops":["agent.pull"]}`)
	pullW := httptest.NewRecorder()
	router.ServeHTTP(pullW, pullReq)
	if pullW.Code != http.StatusOK {
		t.Fatalf("Pull image status = %d, want %d. Body: %s", pullW.Code, http.StatusOK, pullW.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(pullW.Body).Decode(&resp); err != nil {
		t.Fatalf("decode pull response: %v", err)
	}
	if resp["form"] != "image" {
		t.Errorf("pull form = %v, want %q", resp["form"], "image")
	}
	// For image form, content should be the reference string.
	if ref, ok := resp["content"].(string); !ok || ref == "" {
		t.Errorf("pull content = %v, want non-empty reference", resp["content"])
	}
}

func TestHandlers_PullUnauthorized(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	publishVersionHelper(t, router, "code-fixer", "1.0.0", "definition", "content")

	// Pull without a grant header should be denied (403 Forbidden).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/code-fixer/versions/1.0.0/pull", nil)
	req.Header.Set("Authorization", "Bearer rt_valid_runtime_token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("Pull without grant: status = %d, want %d. Body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandlers_PullWithoutAuthn(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	publishVersionHelper(t, router, "code-fixer", "1.0.0", "definition", "content")

	// Pull without any authentication header should return 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/code-fixer/versions/1.0.0/pull", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Pull without auth: status = %d, want %d. Body: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestHandlers_ListAgents(t *testing.T) {
	router, _, _ := setupTestRouter(t)

	// Publish versions for two different agents.
	publishVersionHelper(t, router, "agent-alpha", "1.0.0", "definition", "alpha content")
	publishVersionHelper(t, router, "agent-beta", "1.0.0", "definition", "beta content")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListAgents status = %d, want %d", w.Code, http.StatusOK)
	}

	var agents []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents list: %v", err)
	}
	if len(agents) < 2 {
		t.Fatalf("len(agents) = %d, want at least 2", len(agents))
	}
	names := make(map[string]bool)
	for _, a := range agents {
		if n, ok := a["name"].(string); ok {
			names[n] = true
		}
	}
	if !names["agent-alpha"] {
		t.Error("agent-alpha not found in list")
	}
	if !names["agent-beta"] {
		t.Error("agent-beta not found in list")
	}
}

func TestHandlers_Dispatch(t *testing.T) {
	router, _, broker := setupTestRouter(t)

	// Publish an agent version first.
	publishVersionHelper(t, router, "code-fixer", "1.2.0", "definition", "manifest content...")

	// Subscribe to the target runner's dispatch topic before dispatching.
	sub, err := broker.Subscribe(nil, bus.Identity("test"), bus.Filter("runner.R.dispatch"), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	// Dispatch to runner R.
	dispatchBody := map[string]any{
		"target": "runner-R",
		"grant": map[string]any{
			"ops": []string{"agent.pull"},
		},
	}
	b, _ := json.Marshal(dispatchBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/code-fixer/dispatch", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer pat_operator_token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("Dispatch status = %d, want %d. Body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	// Verify a dispatch message arrived on the runner's topic.
	select {
	case evt := <-sub.Events():
		if evt.Topic != bus.Topic("runner.R.dispatch") {
			t.Errorf("event topic = %q, want %q", evt.Topic, "runner.R.dispatch")
		}
		if len(evt.Message.Payload) == 0 {
			t.Error("dispatch message payload is empty")
		}
	default:
		t.Error("no dispatch message received on runner.R.dispatch topic")
	}
}

func TestHandlers_DispatchCarriesGrant(t *testing.T) {
	router, _, broker := setupTestRouter(t)

	publishVersionHelper(t, router, "code-fixer", "1.2.0", "definition", "manifest content...")

	sub, err := broker.Subscribe(nil, bus.Identity("test"), bus.Filter("runner.R.dispatch"), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Close()

	// Dispatch with a specific grant.
	dispatchBody := map[string]any{
		"target": "runner-R",
		"grant": map[string]any{
			"ops": []string{"agent.pull", "repo.read"},
		},
	}
	b, _ := json.Marshal(dispatchBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/code-fixer/dispatch", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer pat_operator_token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("Dispatch status = %d, want %d. Body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	select {
	case evt := <-sub.Events():
		var msg struct {
			Agent  map[string]string `json:"agent"`
			Grant  json.RawMessage   `json:"grant"`
			Runner string            `json:"runner"`
			Session string           `json:"session,omitempty"`
		}
		if err := json.Unmarshal(evt.Message.Payload, &msg); err != nil {
			t.Fatalf("unmarshal dispatch message: %v", err)
		}
		if msg.Agent == nil {
			t.Error("dispatch message missing agent ref")
		}
		if len(msg.Grant) == 0 {
			t.Error("dispatch message missing grant")
		}
		if msg.Runner == "" {
			t.Error("dispatch message missing runner")
		}
	default:
		t.Error("no dispatch message received")
	}
}
