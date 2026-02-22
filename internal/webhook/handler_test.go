package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestVerifySignature(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "data"}`)

	t.Run("valid signature", func(t *testing.T) {
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(payload)
		signature := hex.EncodeToString(h.Sum(nil))

		handler := &Handler{secret: secret}
		result := handler.verifySignature(payload, signature)
		assert.True(t, result, "valid signature should return true")
	})

	t.Run("invalid signature", func(t *testing.T) {
		invalidSig := "deadbeef"
		handler := &Handler{secret: secret}
		result := handler.verifySignature(payload, invalidSig)
		assert.False(t, result, "invalid signature should return false")
	})

	t.Run("empty signature", func(t *testing.T) {
		handler := &Handler{secret: secret}
		result := handler.verifySignature(payload, "")
		assert.False(t, result, "empty signature should return false")
	})
}

func TestServeHTTP(t *testing.T) {
	secret := "test-webhook-secret"
	logger := zap.NewNop()

	t.Run("invalid method - GET should return 405", func(t *testing.T) {
		handler := NewHandler(nil, secret, logger)
		req := httptest.NewRequest(http.MethodGet, "/hook", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("missing signature header should return 401", func(t *testing.T) {
		handler := NewHandler(nil, secret, logger)
		payload := WebhookPayload{Event: "account_created", Domain: "example.com", User: "testuser"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid signature should return 401", func(t *testing.T) {
		handler := NewHandler(nil, secret, logger)
		payload := WebhookPayload{Event: "account_created", Domain: "example.com", User: "testuser"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Whm2bunny-Signature", "invalid-signature")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid JSON should return 400", func(t *testing.T) {
		handler := NewHandler(nil, secret, logger)
		body := []byte(`{invalid json}`)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))

		// Calculate valid signature
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		signature := hex.EncodeToString(h.Sum(nil))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Whm2bunny-Signature", signature)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("valid account_created request", func(t *testing.T) {
		mockProv := &MockProvisioner{done: make(chan struct{})}
		handler := NewHandler(mockProv, secret, logger)
		payload := WebhookPayload{Event: "account_created", Domain: "example.com", User: "testuser"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))

		// Calculate valid signature
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		signature := hex.EncodeToString(h.Sum(nil))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Whm2bunny-Signature", signature)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		// Wait for async provisioning to complete
		<-mockProv.done
		assert.True(t, mockProv.ProvisionCalled)
		assert.Equal(t, "example.com", mockProv.LastDomain)
	})

	t.Run("valid addon_created request", func(t *testing.T) {
		mockProv := &MockProvisioner{done: make(chan struct{})}
		handler := NewHandler(mockProv, secret, logger)
		payload := WebhookPayload{Event: "addon_created", Domain: "addon.example.com", User: "testuser"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))

		// Calculate valid signature
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		signature := hex.EncodeToString(h.Sum(nil))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Whm2bunny-Signature", signature)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		// Wait for async provisioning to complete
		<-mockProv.done
		assert.True(t, mockProv.ProvisionCalled)
		assert.Equal(t, "addon.example.com", mockProv.LastDomain)
	})

	t.Run("valid subdomain_created request", func(t *testing.T) {
		mockProv := &MockProvisioner{done: make(chan struct{})}
		handler := NewHandler(mockProv, secret, logger)
		payload := WebhookPayload{
			Event:        "subdomain_created",
			Subdomain:    "blog",
			ParentDomain: "example.com",
			User:         "testuser",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))

		// Calculate valid signature
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		signature := hex.EncodeToString(h.Sum(nil))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Whm2bunny-Signature", signature)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		// Wait for async provisioning to complete
		<-mockProv.done
		assert.True(t, mockProv.ProvisionSubdomainCalled)
		assert.Equal(t, "blog", mockProv.LastSubdomain)
		assert.Equal(t, "example.com", mockProv.LastParentDomain)
	})

	t.Run("valid account_deleted request", func(t *testing.T) {
		mockProv := &MockProvisioner{done: make(chan struct{})}
		handler := NewHandler(mockProv, secret, logger)
		payload := WebhookPayload{Event: "account_deleted", Domain: "example.com", User: "testuser"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))

		// Calculate valid signature
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		signature := hex.EncodeToString(h.Sum(nil))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Whm2bunny-Signature", signature)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		// Wait for async deprovisioning to complete
		<-mockProv.done
		assert.True(t, mockProv.DeprovisionCalled)
		assert.Equal(t, "example.com", mockProv.LastDeprovisionDomain)
	})

	t.Run("unknown event should return 400", func(t *testing.T) {
		handler := NewHandler(nil, secret, logger)
		payload := WebhookPayload{Event: "unknown_event", Domain: "example.com", User: "testuser"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))

		// Calculate valid signature
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		signature := hex.EncodeToString(h.Sum(nil))

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Whm2bunny-Signature", signature)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestValidatePayload(t *testing.T) {
	t.Run("valid account_created payload", func(t *testing.T) {
		payload := WebhookPayload{Event: "account_created", Domain: "example.com", User: "testuser"}
		err := validatePayload(&payload)
		assert.NoError(t, err)
	})

	t.Run("valid subdomain_created payload", func(t *testing.T) {
		payload := WebhookPayload{
			Event:        "subdomain_created",
			Subdomain:    "blog",
			ParentDomain: "example.com",
			User:         "testuser",
		}
		err := validatePayload(&payload)
		assert.NoError(t, err)
	})

	t.Run("missing domain for account_created", func(t *testing.T) {
		payload := WebhookPayload{Event: "account_created", User: "testuser"}
		err := validatePayload(&payload)
		assert.Error(t, err)
	})

	t.Run("missing user", func(t *testing.T) {
		payload := WebhookPayload{Event: "account_created", Domain: "example.com"}
		err := validatePayload(&payload)
		assert.Error(t, err)
	})

	t.Run("missing subdomain for subdomain_created", func(t *testing.T) {
		payload := WebhookPayload{Event: "subdomain_created", ParentDomain: "example.com", User: "testuser"}
		err := validatePayload(&payload)
		assert.Error(t, err)
	})

	t.Run("missing parent_domain for subdomain_created", func(t *testing.T) {
		payload := WebhookPayload{Event: "subdomain_created", Subdomain: "blog", User: "testuser"}
		err := validatePayload(&payload)
		assert.Error(t, err)
	})
}

// MockProvisioner is a mock implementation for testing
type MockProvisioner struct {
	ProvisionCalled          bool
	ProvisionSubdomainCalled bool
	DeprovisionCalled        bool
	LastDomain               string
	LastSubdomain            string
	LastParentDomain         string
	LastDeprovisionDomain    string
	LastUser                 string
	done                     chan struct{} // Signal when method is called
}

func (m *MockProvisioner) Provision(domain, user string) error {
	m.ProvisionCalled = true
	m.LastDomain = domain
	m.LastUser = user
	if m.done != nil {
		close(m.done)
	}
	return nil
}

func (m *MockProvisioner) ProvisionSubdomain(subdomain, parentDomain, user string) error {
	m.ProvisionSubdomainCalled = true
	m.LastSubdomain = subdomain
	m.LastParentDomain = parentDomain
	m.LastUser = user
	if m.done != nil {
		close(m.done)
	}
	return nil
}

func (m *MockProvisioner) Deprovision(domain string) error {
	m.DeprovisionCalled = true
	m.LastDeprovisionDomain = domain
	if m.done != nil {
		close(m.done)
	}
	return nil
}

func TestHandlerWriteResponse(t *testing.T) {
	t.Run("success response", func(t *testing.T) {
		w := httptest.NewRecorder()
		resp := Response{Success: true, Message: "Processing started", ID: "test-id"}
		writeJSONResponse(w, http.StatusAccepted, resp)

		assert.Equal(t, http.StatusAccepted, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var decoded Response
		err := json.NewDecoder(w.Body).Decode(&decoded)
		require.NoError(t, err)
		assert.True(t, decoded.Success)
		assert.Equal(t, "Processing started", decoded.Message)
		assert.Equal(t, "test-id", decoded.ID)
	})

	t.Run("error response", func(t *testing.T) {
		w := httptest.NewRecorder()
		resp := ErrorResponse{Error: "Validation failed", Details: "Missing required field"}
		writeJSONResponse(w, http.StatusBadRequest, resp)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var decoded ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&decoded)
		require.NoError(t, err)
		assert.Equal(t, "Validation failed", decoded.Error)
		assert.Equal(t, "Missing required field", decoded.Details)
	})
}

// Test that real HMAC signatures work end-to-end
func TestHMACIntegration(t *testing.T) {
	secret := "integration-test-secret"
	handler := NewHandler(nil, secret, zap.NewNop())

	payload := []byte(`{"event":"account_created","domain":"test.com","user":"test"}`)

	// Create HMAC signature like the WHM hook script would
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expectedSig := hex.EncodeToString(h.Sum(nil))

	result := handler.verifySignature(payload, expectedSig)
	assert.True(t, result, "HMAC signature should verify correctly")
}
