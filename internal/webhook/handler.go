package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	signatureHeader       = "X-Whm2bunny-Signature"
	eventAccountCreated   = "account_created"
	eventAddonCreated     = "addon_created"
	eventSubdomainCreated = "subdomain_created"
	eventAccountDeleted   = "account_deleted"
)

// Provisioner interface defines the operations for provisioning and deprovisioning
type Provisioner interface {
	Provision(domain, user string) error
	ProvisionSubdomain(subdomain, parentDomain, user string) error
	Deprovision(domain string) error
}

// WebhookPayload represents the incoming webhook payload from WHM/cPanel
type WebhookPayload struct {
	Event        string `json:"event"`
	Domain       string `json:"domain"`
	Subdomain    string `json:"subdomain,omitempty"`
	ParentDomain string `json:"parent_domain,omitempty"`
	User         string `json:"user"`
}

// Response represents a successful webhook response
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	ID      string `json:"id,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// Handler handles incoming webhooks from WHM/cPanel
type Handler struct {
	provisioner Provisioner
	secret      string
	logger      *zap.Logger
}

// NewHandler creates a new webhook handler
func NewHandler(provisioner Provisioner, secret string, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		provisioner: provisioner,
		secret:      secret,
		logger:      logger,
	}
}

// ServeHTTP implements the http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		h.logger.Warn("invalid method", zap.String("method", r.Method))
		writeJSONResponse(w, http.StatusMethodNotAllowed, ErrorResponse{
			Error: "method not allowed",
		})
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", zap.Error(err))
		writeJSONResponse(w, http.StatusBadRequest, ErrorResponse{
			Error:   "invalid request",
			Details: "failed to read request body",
		})
		return
	}
	defer r.Body.Close()

	// Verify HMAC signature
	signature := r.Header.Get(signatureHeader)
	if !h.verifySignature(body, signature) {
		h.logger.Warn("invalid signature",
			zap.String("signature", signature),
			zap.String("remote_addr", r.RemoteAddr),
		)
		writeJSONResponse(w, http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Details: "invalid signature",
		})
		return
	}

	// Parse JSON payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("failed to parse JSON payload", zap.Error(err))
		writeJSONResponse(w, http.StatusBadRequest, ErrorResponse{
			Error:   "invalid payload",
			Details: err.Error(),
		})
		return
	}

	// Validate payload
	if err := validatePayload(&payload); err != nil {
		h.logger.Warn("payload validation failed", zap.Error(err))
		writeJSONResponse(w, http.StatusBadRequest, ErrorResponse{
			Error:   "validation failed",
			Details: err.Error(),
		})
		return
	}

	// Generate tracking ID
	trackingID := uuid.New().String()

	// Route to appropriate handler based on event type
	switch payload.Event {
	case eventAccountCreated, eventAddonCreated:
		go h.handleProvision(payload, trackingID)
	case eventSubdomainCreated:
		go h.handleSubdomainProvision(payload, trackingID)
	case eventAccountDeleted:
		go h.handleDeprovision(payload, trackingID)
	default:
		h.logger.Warn("unknown event type", zap.String("event", payload.Event))
		writeJSONResponse(w, http.StatusBadRequest, ErrorResponse{
			Error:   "unknown event",
			Details: fmt.Sprintf("event type '%s' is not supported", payload.Event),
		})
		return
	}

	// Return 202 Accepted for async processing
	h.logger.Info("webhook accepted",
		zap.String("event", payload.Event),
		zap.String("tracking_id", trackingID),
		zap.String("domain", payload.Domain),
	)

	writeJSONResponse(w, http.StatusAccepted, Response{
		Success: true,
		Message: "Processing started",
		ID:      trackingID,
	})
}

// verifySignature computes HMAC-SHA256 of payload and compares with provided signature
func (h *Handler) verifySignature(payload []byte, signature string) bool {
	if signature == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// handleProvision handles domain provisioning asynchronously
func (h *Handler) handleProvision(payload WebhookPayload, trackingID string) {
	h.logger.Info("provisioning domain",
		zap.String("tracking_id", trackingID),
		zap.String("domain", payload.Domain),
		zap.String("user", payload.User),
	)

	if err := h.provisioner.Provision(payload.Domain, payload.User); err != nil {
		h.logger.Error("provisioning failed",
			zap.String("tracking_id", trackingID),
			zap.String("domain", payload.Domain),
			zap.Error(err),
		)
		return
	}

	h.logger.Info("provisioning completed",
		zap.String("tracking_id", trackingID),
		zap.String("domain", payload.Domain),
	)
}

// handleSubdomainProvision handles subdomain provisioning asynchronously
func (h *Handler) handleSubdomainProvision(payload WebhookPayload, trackingID string) {
	fullDomain := fmt.Sprintf("%s.%s", payload.Subdomain, payload.ParentDomain)

	h.logger.Info("provisioning subdomain",
		zap.String("tracking_id", trackingID),
		zap.String("subdomain", payload.Subdomain),
		zap.String("parent_domain", payload.ParentDomain),
		zap.String("full_domain", fullDomain),
		zap.String("user", payload.User),
	)

	if err := h.provisioner.ProvisionSubdomain(payload.Subdomain, payload.ParentDomain, payload.User); err != nil {
		h.logger.Error("subdomain provisioning failed",
			zap.String("tracking_id", trackingID),
			zap.String("subdomain", payload.Subdomain),
			zap.String("parent_domain", payload.ParentDomain),
			zap.Error(err),
		)
		return
	}

	h.logger.Info("subdomain provisioning completed",
		zap.String("tracking_id", trackingID),
		zap.String("subdomain", payload.Subdomain),
		zap.String("parent_domain", payload.ParentDomain),
	)
}

// handleDeprovision handles domain deprovisioning asynchronously
func (h *Handler) handleDeprovision(payload WebhookPayload, trackingID string) {
	h.logger.Info("deprovisioning domain",
		zap.String("tracking_id", trackingID),
		zap.String("domain", payload.Domain),
	)

	if err := h.provisioner.Deprovision(payload.Domain); err != nil {
		h.logger.Error("deprovisioning failed",
			zap.String("tracking_id", trackingID),
			zap.String("domain", payload.Domain),
			zap.Error(err),
		)
		return
	}

	h.logger.Info("deprovisioning completed",
		zap.String("tracking_id", trackingID),
		zap.String("domain", payload.Domain),
	)
}

// validatePayload validates the webhook payload based on event type
func validatePayload(payload *WebhookPayload) error {
	// User is always required
	if payload.User == "" {
		return fmt.Errorf("user is required")
	}

	switch payload.Event {
	case eventAccountCreated, eventAddonCreated, eventAccountDeleted:
		if payload.Domain == "" {
			return fmt.Errorf("domain is required for event '%s'", payload.Event)
		}
	case eventSubdomainCreated:
		if payload.Subdomain == "" {
			return fmt.Errorf("subdomain is required for event '%s'", payload.Event)
		}
		if payload.ParentDomain == "" {
			return fmt.Errorf("parent_domain is required for event '%s'", payload.Event)
		}
	default:
		return fmt.Errorf("unknown event type: '%s'", payload.Event)
	}

	return nil
}

// writeJSONResponse writes a JSON response with the given status code
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
