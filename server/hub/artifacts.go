package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"mework/server/auth"
	"mework/shared/core"
	"mework/shared/ports"
)

// ArtifactHandlers serves artifact-related HTTP endpoints.
type ArtifactHandlers struct {
	store ports.ArtifactStore
}

// NewArtifactHandlers creates handlers backed by the given ArtifactStore.
func NewArtifactHandlers(store ports.ArtifactStore) *ArtifactHandlers {
	return &ArtifactHandlers{store: store}
}

// ListArtifacts responds with all artifacts for a run.
// GET /api/v1/runs/{runID}/artifacts
func (h *ArtifactHandlers) ListArtifacts(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	if runID == "" {
		http.Error(w, "runID is required", http.StatusBadRequest)
		return
	}

	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "authenticated tenant not found", http.StatusUnauthorized)
		return
	}

	infos, err := h.store.List(r.Context(), core.TenantID(tenantID), runID)
	if err != nil {
		http.Error(w, "failed to list artifacts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

// GetArtifact serves a single artifact via presigned URL redirect.
// GET /api/v1/runs/{runID}/artifacts/{name}
func (h *ArtifactHandlers) GetArtifact(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	name := chi.URLParam(r, "name")
	if runID == "" || name == "" {
		http.Error(w, "runID and name are required", http.StatusBadRequest)
		return
	}

	tenantID, ok := auth.GetTenantID(r.Context())
	if !ok {
		http.Error(w, "authenticated tenant not found", http.StatusUnauthorized)
		return
	}

	// Generate a presigned GET URL for the artifact.
	presignedURL, err := h.store.PresignGetURL(
		r.Context(),
		core.TenantID(tenantID),
		core.ArtifactRef{RunID: runID, Name: name},
		15*time.Minute,
	)
	if err != nil {
		http.Error(w, "failed to get artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect to the presigned URL.
	http.Redirect(w, r, presignedURL, http.StatusFound)
}

// ---- Dummy artifact store for wiring until full backing is in place ----

// NewDummyArtifactStore returns a no-op ArtifactStore that returns empty lists
// and errors on data operations. Use it when the real ObjectStore-backed
// ArtifactStore cannot yet be constructed.
func NewDummyArtifactStore() ports.ArtifactStore {
	return &dummyArtifactStore{}
}

type dummyArtifactStore struct{}

func (d *dummyArtifactStore) Put(_ context.Context, _ core.TenantID, _ core.ArtifactRef, _ []byte) error {
	return errors.New("artifact store not yet wired")
}
func (d *dummyArtifactStore) Get(_ context.Context, _ core.TenantID, _ core.ArtifactRef) ([]byte, error) {
	return nil, errors.New("artifact store not yet wired")
}
func (d *dummyArtifactStore) List(_ context.Context, _ core.TenantID, _ string) ([]core.ArtifactInfo, error) {
	return []core.ArtifactInfo{}, nil
}
func (d *dummyArtifactStore) PresignGetURL(_ context.Context, _ core.TenantID, _ core.ArtifactRef, _ time.Duration) (string, error) {
	return "", errors.New("artifact store not yet wired")
}
func (d *dummyArtifactStore) PresignPutURL(_ context.Context, _ core.TenantID, _ core.ArtifactRef, _ time.Duration) (string, error) {
	return "", errors.New("artifact store not yet wired")
}
