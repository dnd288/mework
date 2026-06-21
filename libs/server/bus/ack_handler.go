package bus

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// AckHandler handles delivery acknowledgement requests.
type AckHandler struct {
	broker Broker
}

// NewAckHandler returns a new AckHandler backed by the given broker.
func NewAckHandler(broker Broker) *AckHandler {
	return &AckHandler{broker: broker}
}

// Ack handles POST /messages/{msgID}/ack. It marks a message as acknowledged
// so that it is not redelivered.
func (h *AckHandler) Ack(w http.ResponseWriter, r *http.Request) {
	msgID := chi.URLParam(r, "msgID")
	if msgID == "" {
		http.Error(w, "missing message id", http.StatusBadRequest)
		return
	}

	err := h.broker.Ack(r.Context(), msgID)
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch {
	case errors.Is(err, ErrMessageNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrAlreadyAcked):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
