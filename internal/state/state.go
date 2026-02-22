package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// StatusPending indicates the domain is waiting to be provisioned
	StatusPending = "pending"
	// StatusProvisioning indicates the domain is currently being provisioned
	StatusProvisioning = "provisioning"
	// StatusSuccess indicates the domain was successfully provisioned
	StatusSuccess = "success"
	// StatusFailed indicates the provisioning failed
	StatusFailed = "failed"
)

// Step constants representing each provisioning step
const (
	StepNone       = 0
	StepDNSZone    = 1
	StepDNSRecords = 2
	StepPullZone   = 3
	StepCNAMESync  = 4
)

// ProvisionState tracks the provisioning progress of a domain
type ProvisionState struct {
	ID          string    `json:"id"`           // UUID
	Domain      string    `json:"domain"`       // Domain being provisioned
	Status      string    `json:"status"`       // pending, provisioning, success, failed
	CurrentStep int       `json:"current_step"` // 1-4 (DNS Zone, Records, Pull Zone, CNAME)
	ZoneID      int64     `json:"zone_id,omitempty"`
	PullZoneID  int64     `json:"pull_zone_id,omitempty"`
	CDNHostname string    `json:"cdn_hostname,omitempty"`
	Error       string    `json:"error,omitempty"`
	Retries     int       `json:"retries"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Manager handles state persistence and retrieval
type Manager struct {
	filePath    string
	states      map[string]*ProvisionState
	domainIndex map[string]string // domain -> id mapping
	mu          sync.RWMutex
	logger      *zap.Logger
}

// NewManager creates a new state manager with the specified state file path
func NewManager(filePath string, logger *zap.Logger) (*Manager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	m := &Manager{
		filePath:    filePath,
		states:      make(map[string]*ProvisionState),
		domainIndex: make(map[string]string),
		logger:      logger,
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	// Load existing state if file exists
	if err := m.load(); err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	return m, nil
}

// load reads the state from disk
func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.logger.Info("State file does not exist, starting with empty state",
				zap.String("path", m.filePath))
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	var states []*ProvisionState
	if err := json.Unmarshal(data, &states); err != nil {
		return fmt.Errorf("failed to unmarshal state: %w", err)
	}

	m.states = make(map[string]*ProvisionState)
	m.domainIndex = make(map[string]string)

	for _, state := range states {
		m.states[state.ID] = state
		m.domainIndex[state.Domain] = state.ID
	}

	m.logger.Info("Loaded state from disk",
		zap.Int("count", len(states)),
		zap.String("path", m.filePath))

	return nil
}

// save writes the state to disk
func (m *Manager) save() error {
	states := make([]*ProvisionState, 0, len(m.states))
	for _, state := range m.states {
		states = append(states, state)
	}

	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file first for atomicity
	tmpPath := m.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Rename for atomic update
	if err := os.Rename(tmpPath, m.filePath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// Create creates a new provisioning state for a domain
func (m *Manager) Create(domain string) *ProvisionState {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	state := &ProvisionState{
		ID:          uuid.New().String(),
		Domain:      domain,
		Status:      StatusPending,
		CurrentStep: StepNone,
		Retries:     0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m.states[state.ID] = state
	m.domainIndex[domain] = state.ID

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save state after create",
			zap.String("domain", domain),
			zap.Error(err))
	}

	m.logger.Info("Created provisioning state",
		zap.String("id", state.ID),
		zap.String("domain", domain))

	return state
}

// Get retrieves a state by ID
func (m *Manager) Get(id string) (*ProvisionState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[id]
	if !exists {
		return nil, ErrStateNotFound
	}

	// Return a copy to prevent concurrent modification
	stateCopy := *state
	return &stateCopy, nil
}

// GetByDomain retrieves a state by domain name
func (m *Manager) GetByDomain(domain string) (*ProvisionState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, exists := m.domainIndex[domain]
	if !exists {
		return nil, ErrStateNotFound
	}

	state, exists := m.states[id]
	if !exists {
		return nil, ErrStateNotFound
	}

	// Return a copy to prevent concurrent modification
	stateCopy := *state
	return &stateCopy, nil
}

// Update updates an existing provisioning state
func (m *Manager) Update(state *ProvisionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.states[state.ID]
	if !exists {
		return ErrStateNotFound
	}

	// Preserve creation time
	state.CreatedAt = existing.CreatedAt
	state.UpdatedAt = time.Now()

	m.states[state.ID] = state
	m.domainIndex[state.Domain] = state.ID

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save state after update",
			zap.String("id", state.ID),
			zap.Error(err))
		return fmt.Errorf("failed to save state: %w", err)
	}

	m.logger.Debug("Updated provisioning state",
		zap.String("id", state.ID),
		zap.String("status", state.Status),
		zap.Int("step", state.CurrentStep))

	return nil
}

// Delete removes a state by ID
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return ErrStateNotFound
	}

	delete(m.states, id)
	delete(m.domainIndex, state.Domain)

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save state after delete",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("failed to save state: %w", err)
	}

	m.logger.Info("Deleted provisioning state",
		zap.String("id", id),
		zap.String("domain", state.Domain))

	return nil
}

// ListPending returns all states with pending or provisioning status
func (m *Manager) ListPending() []*ProvisionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ProvisionState
	for _, state := range m.states {
		if state.Status == StatusPending || state.Status == StatusProvisioning {
			stateCopy := *state
			result = append(result, &stateCopy)
		}
	}

	return result
}

// ListFailed returns all states with failed status
func (m *Manager) ListFailed() []*ProvisionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ProvisionState
	for _, state := range m.states {
		if state.Status == StatusFailed {
			stateCopy := *state
			result = append(result, &stateCopy)
		}
	}

	return result
}

// ListAll returns all states
func (m *Manager) ListAll() []*ProvisionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ProvisionState, 0, len(m.states))
	for _, state := range m.states {
		stateCopy := *state
		result = append(result, &stateCopy)
	}

	return result
}

// Recover returns states that need recovery (pending or failed with retries remaining)
func (m *Manager) Recover() []*ProvisionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ProvisionState
	for _, state := range m.states {
		// Include pending states
		if state.Status == StatusPending {
			stateCopy := *state
			result = append(result, &stateCopy)
			continue
		}
		// Include failed states that haven't exceeded retry limit
		if state.Status == StatusFailed && state.Retries < 5 {
			stateCopy := *state
			result = append(result, &stateCopy)
		}
	}

	return result
}

// IncrementStep advances the current step
func (m *Manager) IncrementStep(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return ErrStateNotFound
	}

	state.CurrentStep++
	state.UpdatedAt = time.Now()

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save state after increment",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// SetError sets an error message and marks the state as failed
func (m *Manager) SetError(id, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return ErrStateNotFound
	}

	state.Status = StatusFailed
	state.Error = errMsg
	state.Retries++
	state.UpdatedAt = time.Now()

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save state after error",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("failed to save state: %w", err)
	}

	m.logger.Warn("Marked state as failed",
		zap.String("id", id),
		zap.String("error", errMsg),
		zap.Int("retry", state.Retries))

	return nil
}

// MarkSuccess marks the state as successfully provisioned
func (m *Manager) MarkSuccess(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return ErrStateNotFound
	}

	state.Status = StatusSuccess
	state.CurrentStep = StepCNAMESync
	state.Error = ""
	state.UpdatedAt = time.Now()

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save state after success",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("failed to save state: %w", err)
	}

	m.logger.Info("Marked state as success",
		zap.String("id", id),
		zap.String("domain", state.Domain))

	return nil
}

// MarkProvisioning marks the state as currently being provisioned
func (m *Manager) MarkProvisioning(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[id]
	if !exists {
		return ErrStateNotFound
	}

	state.Status = StatusProvisioning
	state.UpdatedAt = time.Now()

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save state after marking provisioning",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// GetStateFilePath returns the current state file path
func (m *Manager) GetStateFilePath() string {
	return m.filePath
}

// GetCount returns the number of states
func (m *Manager) GetCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.states)
}

// Clear removes all states (use with caution)
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.states = make(map[string]*ProvisionState)
	m.domainIndex = make(map[string]string)

	if err := m.save(); err != nil {
		return fmt.Errorf("failed to save state after clear: %w", err)
	}

	m.logger.Info("Cleared all states")

	return nil
}

// StepName returns the human-readable name for a step number
func StepName(step int) string {
	switch step {
	case StepNone:
		return "none"
	case StepDNSZone:
		return "dns_zone"
	case StepDNSRecords:
		return "dns_records"
	case StepPullZone:
		return "pull_zone"
	case StepCNAMESync:
		return "cname_sync"
	default:
		return "unknown"
	}
}

// State errors
var (
	// ErrStateNotFound is returned when a state is not found
	ErrStateNotFound = fmt.Errorf("state not found")
	// ErrStateConflict is returned when a state already exists for a domain
	ErrStateConflict = fmt.Errorf("state already exists for domain")
)

// BandwidthSnapshot stores bandwidth statistics for historical comparison
type BandwidthSnapshot struct {
	Timestamp   time.Time `json:"timestamp"`
	ZoneID      int64     `json:"zone_id"`
	ZoneName    string    `json:"zone_name"`
	Bandwidth   int64     `json:"bandwidth"`
	Requests    int64     `json:"requests"`
	CacheHits   int64     `json:"cache_hits"`
	CacheMisses int64     `json:"cache_misses"`
}

// SnapshotStore manages bandwidth snapshots
type SnapshotStore struct {
	filePath  string
	snapshots []BandwidthSnapshot
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewSnapshotStore creates a new snapshot store
func NewSnapshotStore(filePath string, logger *zap.Logger) (*SnapshotStore, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &SnapshotStore{
		filePath:  filePath,
		snapshots: make([]BandwidthSnapshot, 0),
		logger:    logger,
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Load existing snapshots if file exists
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load snapshots: %w", err)
	}

	return s, nil
}

// load reads snapshots from disk
func (s *SnapshotStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read snapshot file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &s.snapshots); err != nil {
		return fmt.Errorf("failed to unmarshal snapshots: %w", err)
	}

	return nil
}

// save writes snapshots to disk
func (s *SnapshotStore) save() error {
	data, err := json.MarshalIndent(s.snapshots, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshots: %w", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp snapshot file: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename snapshot file: %w", err)
	}

	return nil
}

// AddSnapshot adds a new bandwidth snapshot
func (s *SnapshotStore) AddSnapshot(snapshot BandwidthSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots = append(s.snapshots, snapshot)

	// Clean up old snapshots (keep last 30 days)
	cutoff := time.Now().AddDate(0, 0, -30)
	filtered := make([]BandwidthSnapshot, 0, len(s.snapshots))
	for _, snap := range s.snapshots {
		if snap.Timestamp.After(cutoff) {
			filtered = append(filtered, snap)
		}
	}
	s.snapshots = filtered

	if err := s.save(); err != nil {
		s.logger.Error("Failed to save snapshots",
			zap.Error(err))
		return fmt.Errorf("failed to save snapshots: %w", err)
	}

	return nil
}

// GetSnapshotsByZone retrieves snapshots for a specific zone
func (s *SnapshotStore) GetSnapshotsByZone(zoneID int64, since time.Time) []BandwidthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []BandwidthSnapshot
	for _, snap := range s.snapshots {
		if snap.ZoneID == zoneID && snap.Timestamp.After(since) {
			result = append(result, snap)
		}
	}

	return result
}

// GetAllSnapshots retrieves all snapshots since a given time
func (s *SnapshotStore) GetAllSnapshots(since time.Time) []BandwidthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []BandwidthSnapshot
	for _, snap := range s.snapshots {
		if snap.Timestamp.After(since) {
			result = append(result, snap)
		}
	}

	return result
}

// GetLatestSnapshotByZone retrieves the latest snapshot for a zone
func (s *SnapshotStore) GetLatestSnapshotByZone(zoneID int64) *BandwidthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *BandwidthSnapshot
	for i := range s.snapshots {
		if s.snapshots[i].ZoneID == zoneID {
			if latest == nil || s.snapshots[i].Timestamp.After(latest.Timestamp) {
				snap := s.snapshots[i]
				latest = &snap
			}
		}
	}

	return latest
}

// Cleanup removes snapshots older than the specified duration
func (s *SnapshotStore) Cleanup(olderThan time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	filtered := make([]BandwidthSnapshot, 0, len(s.snapshots))
	for _, snap := range s.snapshots {
		if snap.Timestamp.After(cutoff) {
			filtered = append(filtered, snap)
		}
	}

	s.snapshots = filtered

	if err := s.save(); err != nil {
		return fmt.Errorf("failed to save snapshots after cleanup: %w", err)
	}

	return nil
}
