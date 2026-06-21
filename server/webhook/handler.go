package webhook

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/server/bus"
	"mework/server/connection"
	"mework/server/orchestrator"
	"mework/server/provider"
	"mework/shared/providers/mello"
)

type Handler struct {
	pool          *pgxpool.Pool
	broker        bus.Broker
	secretKey     string
	melloBaseURL  string
	connectionSvc *connection.Service
}

func NewHandler(pool *pgxpool.Pool, broker bus.Broker, secretKey string, melloBaseURL string) *Handler {
	return &Handler{
		pool:          pool,
		broker:        broker,
		secretKey:     secretKey,
		melloBaseURL:  melloBaseURL,
		connectionSvc: connection.NewService(pool, secretKey),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	providerCode := chi.URLParam(r, "provider")
	prov, ok := provider.Get(providerCode)
	if !ok {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request: failed to read body", http.StatusBadRequest)
		return
	}

	// 1. Pre-parse board_id/container_id from raw body
	containerID, err := prov.ExtractContainerID(bodyBytes)
	if err != nil {
		log.Printf("Webhook pre-parse container ID failed: %v", err)
		w.WriteHeader(http.StatusOK) // Silent ignore
		return
	}

	// 2. Lookup watched container -> account_id
	var accountID string
	err = h.pool.QueryRow(r.Context(), `
		SELECT account_id FROM watched_containers
		WHERE provider_code = $1 AND external_container_id = $2
	`, providerCode, containerID).Scan(&accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Unmapped container %s for provider %s, ignoring webhook", containerID, providerCode)
			w.WriteHeader(http.StatusOK) // Fail closed, but return 200 so provider stops retrying
			return
		}
		log.Printf("Database lookup for container failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 3. Load provider connection -> webhook secret
	var webhookSecret string
	err = h.pool.QueryRow(r.Context(), `
		SELECT webhook_secret FROM provider_connections
		WHERE account_id = $1 AND provider_code = $2
	`, accountID, providerCode).Scan(&webhookSecret)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("No connection secret found for account %s and provider %s", accountID, providerCode)
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("Database lookup for connection secret failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 4. Verify signature & timestamp (replay check)
	var signature, timestamp, deliveryID string
	if providerCode == "mello" {
		signature = r.Header.Get("X-Mello-Signature")
		timestamp = r.Header.Get("X-Mello-Timestamp")
		deliveryID = r.Header.Get("X-Mello-Delivery-Id")
	}

	if signature == "" || timestamp == "" {
		http.Error(w, "Unauthorized: missing signature or timestamp", http.StatusUnauthorized)
		return
	}

	err = prov.VerifyWebhook(bodyBytes, timestamp, signature, webhookSecret)
	if err != nil {
		log.Printf("Webhook signature verification failed: %v", err)
		http.Error(w, "Unauthorized: signature check failed", http.StatusUnauthorized)
		return
	}

	// 5. Parse full event
	ev, err := prov.ParseEvent(bodyBytes)
	if err != nil {
		log.Printf("Webhook event parse failed: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// 6. Check trigger grammar in comment body
	profileName, workflowName, instructions, isTrigger := ParseTrigger(ev.Body)
	if !isTrigger {
		w.WriteHeader(http.StatusOK) // Regular comment, ignore silently
		return
	}

	// 7. Check idempotency: deliveryID must be unique
	if deliveryID != "" {
		var exists bool
		err = h.pool.QueryRow(r.Context(), `
			SELECT EXISTS (SELECT 1 FROM jobs WHERE provider_code = $1 AND external_event_id = $2)
		`, providerCode, deliveryID).Scan(&exists)
		if err == nil && exists {
			log.Printf("Duplicate webhook event %s, skipping", deliveryID)
			w.WriteHeader(http.StatusOK) // Return 200 so provider knows it's handled
			return
		}
	}

	// 8. Actor validation against account identities allowlist
	var actorAuthorized bool
	err = h.pool.QueryRow(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM account_identities
			WHERE account_id = $1 AND provider_code = $2 AND external_user_id = $3
		)
	`, accountID, providerCode, ev.Actor.ID).Scan(&actorAuthorized)
	if err != nil || !actorAuthorized {
		log.Printf("Actor %s (%s) is not authorized for account %s, ignoring", ev.Actor.Name, ev.Actor.ID, accountID)
		w.WriteHeader(http.StatusOK) // Silent ignore
		return
	}

	// 9. Resolve runtime by profile code (profileName == runtimes.code)
	var runtimeID string
	err = h.pool.QueryRow(r.Context(), `
		SELECT id FROM runtimes
		WHERE account_id = $1 AND code = $2
	`, accountID, profileName).Scan(&runtimeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("No registered runtime found for code %s in account %s", profileName, accountID)
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("Runtime resolution query failed: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 10. Enforce 64KB instructions cap (H-size)
	if len(instructions) > 65536 {
		log.Printf("Trigger rejected: instructions length %d exceeds 64KB cap", len(instructions))
		w.WriteHeader(http.StatusOK)
		return
	}

	// 11. Resolve profile to snapshot (M5/M6)
	var profileBodySnapshot *string
	var bodyTemp string
	err = h.pool.QueryRow(r.Context(), `
		SELECT body FROM profiles
		WHERE account_id = $1 AND name = $2
	`, accountID, profileName).Scan(&bodyTemp)
	if err == nil {
		profileBodySnapshot = &bodyTemp
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Profile resolution query failed: %v", err)
	}

	// 12. Decrypt connection token to snapshot task title and description (H3)
	provToken, err := h.connectionSvc.GetDecryptedToken(r.Context(), accountID, providerCode)
	if err != nil {
		log.Printf("Failed to decrypt provider token to fetch ticket: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	client := mello.NewClient(h.melloBaseURL, provToken, 10*time.Second, "mework-server")
	ticket, err := client.GetTicket(ev.ExternalTaskID)
	if err != nil {
		log.Printf("Failed to fetch ticket detail from Mello: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse custom workflow instructions if workflowName was parsed
	finalInstructions := instructions
	if workflowName != "" {
		// Store it in config/workflow if needed. But for instructions,
		// the trigger grammar maps it to the prompt.
		// Wait, how does the prompt snapshot work?
		// We just pass finalInstructions = instructions.
	}

	// 13. Publish dispatch message to the bus
	topic := bus.FormatTopic(bus.TopicRunnerDispatch, profileName)
	msgPayload, err := json.Marshal(map[string]interface{}{
		"runtime_id":          runtimeID,
		"external_task_id":    ev.ExternalTaskID,
		"provider_code":       providerCode,
		"external_actor_id":   ev.Actor.ID,
		"task_title":          ticket.Title,
		"task_description":    ticket.Description,
		"profile_body_snapshot": profileBodySnapshot,
		"workflow":            workflowName,
		"instructions":        finalInstructions,
	})
	if err != nil {
		log.Printf("Failed to marshal message payload: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	err = h.broker.Publish(r.Context(), topic, bus.Message{Payload: msgPayload})
	if err != nil {
		log.Printf("Failed to publish dispatch message: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Insert job row into backing store for state tracking.
	// This is best-effort: the publish is the transport, the store is the record.
	_, _ = orchestrator.Enqueue(r.Context(), h.pool, orchestrator.EnqueueParams{
		AccountID:           accountID,
		RuntimeID:           runtimeID,
		ExternalTaskID:      ev.ExternalTaskID,
		ExternalEventID:     deliveryID,
		ProviderCode:        providerCode,
		ExternalActorID:     ev.Actor.ID,
		TaskTitle:           ticket.Title,
		TaskDescription:     ticket.Description,
		ProfileBodySnapshot: profileBodySnapshot,
		Workflow:            workflowName,
		Instructions:        finalInstructions,
	})

	w.WriteHeader(http.StatusAccepted)
}
