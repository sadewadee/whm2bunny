package webhook

import (
	"net/http"

	_ "github.com/go-chi/chi/v5"
	_ "go.uber.org/zap"
	_ "github.com/google/uuid"
)

// Handler handles incoming webhooks from WHM/cPanel
type Handler struct {
	// TODO: Add fields for provisioner, validator, logger, etc.
}

// NewHandler creates a new webhook handler
func NewHandler() *Handler {
	return &Handler{}
}

// ServeHTTP implements http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement webhook handling
	w.WriteHeader(http.StatusOK)
}
