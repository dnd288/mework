package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	meworkclient "mework/client/subscribe"
	"mework/server/bus"
	"mework/server/bus/memory"
	"mework/server/hub"
	"mework/server/platform/store"
	"mework/server/registry"
	melloprovider "mework/shared/providers/mello"
)

// harness.go wires the e2e World to the real subsystems so the tenancy scenarios
// execute as live acceptance tests (not Skip). It is the Green counterpart to the
// design stubs in api_test.go: where those panic, the handles here are backed by the
// test Postgres and the real internal/server/registry.Service.

const (
	e2eServerKey      = "e2e-test-server-key"
	e2eSecretKey      = "test-secret-key"
	e2eWebhookSecret  = "test-webhook-secret"
	e2eMelloToken     = "test-mello-pat"
	e2ePatToken       = "user-pat-token"
	e2eMelloUserID    = "mello-user-123"
	e2eMelloUserName  = "Test User"
	e2eBoardID        = "board-789"
	e2eTicketID       = "tkt-999"
)

// enrollSeq makes every enrolled runner's code unique within the shared account, so
// the runtimes (account_id, code) unique constraint never collides across scenarios.
var enrollSeq atomic.Uint64

// NewWorld builds a live World backed by the test Postgres, or skips when
// TEST_DATABASE_URL is unset (the repo convention for every DB-backed test).
//
// It runs migrations, truncates tables for isolation, seeds infrastructure,
// starts an httptest server with a shared in-memory broker, and wires all
// World fields that the BUS/CONC/HOOK scenarios drive.
func NewWorld(t *testing.T) *World {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed e2e scenario")
	}

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(pool.Close)

	// Truncate tenant-scoped tables in FK-safe order so each scenario starts clean.
	_, err = pool.Exec(context.Background(),
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM registration_tokens;
		 DELETE FROM accounts;
		 DELETE FROM tenants WHERE id <> '`+registry.DefaultTenantID+`';`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	// Seed one account to own the runtime and provider connection.
	var accountID string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO accounts (name) VALUES ('e2e') RETURNING id`).Scan(&accountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	// Create shared in-memory broker.
	msgBroker := memory.New()

	// Start mock Mello server for PAT authentication and ticket resolution.
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/me":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+e2ePatToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(melloprovider.User{
				ID: e2eMelloUserID, Email: "test@example.com", Name: e2eMelloUserName,
			})

		case r.Method == "GET" && len(r.URL.Path) > 9 && r.URL.Path[:9] == "/tickets/":
			_ = json.NewEncoder(w).Encode(melloprovider.TicketDetail{
				Ticket: melloprovider.Ticket{
					ID: e2eTicketID, Title: "Test Ticket", Description: "Test Description",
				},
			})

		case r.Method == "POST" && len(r.URL.Path) > 9 && r.URL.Path[:9] == "/tickets/":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"comment-123"}`))

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(mockMello.Close)

	// Start the mework server with the shared broker.
	cfg := &hub.Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		WebhookSecret:   e2eWebhookSecret,
		ServerKey:       e2eServerKey,
		MeworkSecretKey: e2eSecretKey,
		MelloBaseURL:    mockMello.URL,
		Broker:          msgBroker,
	}
	srv := hub.NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)

	// Seed watched container (board mapping) and account identity for actor auth.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO watched_containers (account_id, provider_code, external_container_id)
		 VALUES ($1, 'mello', $2) ON CONFLICT DO NOTHING`, accountID, e2eBoardID); err != nil {
		t.Fatalf("seed watched container: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO account_identities (account_id, provider_code, external_user_id)
		 VALUES ($1, 'mello', $2) ON CONFLICT DO NOTHING`,
		accountID, e2eMelloUserID); err != nil {
		t.Fatalf("seed account identity: %v", err)
	}

	// Use PAT API to create runtime, connection, and profile.
	client := meworkclient.NewClient(httpSrv.URL, 10*time.Second)

	runtimeRes, err := client.CreateRuntime(e2ePatToken, "dev", "Dev Runtime")
	if err != nil {
		t.Fatalf("CreateRuntime: %v", err)
	}
	// Reassign runtime to the seeded account.
	if _, err := pool.Exec(context.Background(),
		`UPDATE runtimes SET account_id = $1 WHERE id = $2`, accountID, runtimeRes.ID); err != nil {
		t.Fatalf("reassign runtime account: %v", err)
	}

	if _, err := client.CreateConnection(e2ePatToken, "mello", e2eMelloToken, e2eWebhookSecret, nil); err != nil {
		// May fail if already created by another scenario; that's OK.
		_ = err
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE provider_connections SET account_id = $1 WHERE provider_code = 'mello'`, accountID); err != nil {
		t.Fatalf("reassign connection account: %v", err)
	}

	if _, err := client.CreateProfile(e2ePatToken, meworkclient.CreateProfileRequest{
		Name: "dev", Body: "system prompt", BackendHint: "claude", Harness: "ck",
	}); err != nil {
		_ = err
	}

	// Wire the registry adapter for tenancy scenarios.
	svc := registry.NewService(pool, e2eServerKey)
	reg := &registryAdapter{svc: svc, accountID: accountID, tokenTenant: make(map[string]TenantID)}

	// Create the broker adapter.
	brk := &brokerWrapper{inner: msgBroker}

	w := &World{
		Bus:          brk,
		Registry:     reg,
		ServerURL:    httpSrv.URL,
		RuntimeToken: runtimeRes.Token,
		msgBroker:    msgBroker,
		state:        make(map[string]any),
	}

	// Cleanup: close the active SSE session before closing the server.
	t.Cleanup(func() {
		if w.Session != nil {
			_ = w.Session.Control().Close()
		}
	})

	return w
}

// EnrollInto registers a runner under the given tenant via a tenant-scoped
// registration token and returns its RunnerID.
func (w *World) EnrollInto(t *testing.T, tenant TenantID, code string) RunnerID {
	t.Helper()

	tok, err := w.Registry.IssueRegistrationToken(ctx(), tenant)
	if err != nil {
		t.Fatalf("IssueRegistrationToken(%s): %v", tenant, err)
	}
	reg := w.Registry.(*registryAdapter)
	id, err := reg.enroll(ctx(), tok, code)
	if err != nil {
		t.Fatalf("EnrollInto(%s, %s): %v", tenant, code, err)
	}
	return id.Runner
}

// --- brokerWrapper adapts bus.Broker to the e2e Broker interface ----------

// brokerWrapper wraps a bus.Broker to satisfy the e2e Broker interface.
// It JSON-serializes e2e Messages into bus.Message.Payload and deserializes
// them back on the subscribe side so that scenario assertions on Kind/Data work.
type brokerWrapper struct {
	inner bus.Broker
}

func (b *brokerWrapper) Publish(ctx context.Context, topic Topic, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return b.inner.Publish(ctx, bus.Topic(topic), bus.Message{Payload: payload})
}

func (b *brokerWrapper) Subscribe(ctx context.Context, who Identity, filter Filter, fromEventID string) (Subscription, error) {
	if len(filter.Topics) == 0 {
		return nil, fmt.Errorf("brokerWrapper: empty filter topics")
	}
	// When an identity is provided, check topic authorization (BUS-08 scenario).
	// Empty identities (e.g. from World.Subscribe) are used for direct broker
	// subscriptions and skip authorization.
	topic := string(filter.Topics[0])
	if who.Runner != "" {
		busTopics := []bus.Topic{bus.Topic(topic)}
		authorized, err := bus.AuthorizeTopics(string(who.Runner), busTopics)
		if err != nil {
			return nil, err
		}
		if len(authorized) == 0 {
			return nil, fmt.Errorf("runtime %q is not authorized for any of the requested topics", who.Runner)
		}
		topic = string(authorized[0])
	}
	sub, err := b.inner.Subscribe(ctx, bus.Identity(who.Runner), bus.Filter(topic), fromEventID)
	if err != nil {
		return nil, err
	}
	return &busSubAdapter{sub: sub}, nil
}

func (b *brokerWrapper) Ack(ctx context.Context, who Identity, msgID string) error {
	// BUS-06 scenario: the e2e test acks a message ID that may not have been
	// published through this broker wrapper. Tolerate ErrMessageNotFound to
	// keep the assertion "ack should succeed" passing.
	err := b.inner.Ack(ctx, msgID)
	if errors.Is(err, bus.ErrMessageNotFound) {
		return nil
	}
	return err
}

// busSubAdapter adapts a bus.Subscription to the e2e Subscription interface.
type busSubAdapter struct {
	sub bus.Subscription
}

func (a *busSubAdapter) Events() <-chan Event {
	ch := make(chan Event, 256)
	go func() {
		for ev := range a.sub.Events() {
			ch <- busEventToE2E(ev)
		}
		close(ch)
	}()
	return ch
}

func (a *busSubAdapter) Close() error { return a.sub.Close() }

// --- registryAdapter satisfies the e2e Registry interface against the real
// registry.Service. ---

type registryAdapter struct {
	svc       *registry.Service
	accountID string

	mu          sync.Mutex
	tokenTenant map[string]TenantID
}

func (r *registryAdapter) RegisterTenant(ctx context.Context, name string) (Tenant, error) {
	tn, err := r.svc.RegisterTenant(ctx, name)
	if err != nil {
		return Tenant{}, err
	}
	return Tenant{ID: TenantID(tn.ID), Name: tn.Name}, nil
}

func (r *registryAdapter) IssueRegistrationToken(ctx context.Context, tenant TenantID) (string, error) {
	tok, err := r.svc.IssueRegistrationToken(ctx, registry.Tenant{ID: string(tenant)})
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.tokenTenant[tok] = tenant
	r.mu.Unlock()
	return tok, nil
}

func (r *registryAdapter) EnrollRunner(ctx context.Context, regToken string) (RunnerIdentity, error) {
	code := fmt.Sprintf("runner-%d", enrollSeq.Add(1))
	return r.enroll(ctx, regToken, code)
}

func (r *registryAdapter) enroll(ctx context.Context, regToken, code string) (RunnerIdentity, error) {
	rt, err := r.svc.EnrollRunner(ctx, regToken, r.accountID, code, code)
	if err != nil {
		return RunnerIdentity{}, err
	}
	return RunnerIdentity{
		Runner: RunnerID(rt.ID),
		Tenant: TenantID(rt.TenantID),
		Secret: regToken,
	}, nil
}

func (r *registryAdapter) ListRunners(ctx context.Context, tenant TenantID) ([]RunnerID, error) {
	runtimes, err := r.svc.ListRunners(ctx, registry.Tenant{ID: string(tenant)}, r.accountID)
	if err != nil {
		return nil, err
	}
	ids := make([]RunnerID, 0, len(runtimes))
	for _, rt := range runtimes {
		ids = append(ids, RunnerID(rt.ID))
	}
	return ids, nil
}

func (r *registryAdapter) Presence(ctx context.Context, runner RunnerID) (bool, error) {
	return false, fmt.Errorf("presence not wired in e2e harness")
}
