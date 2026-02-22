package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// getTestLogger returns a test logger
func getTestLogger() *zap.Logger {
	return zap.NewNop()
}

// getTempDir returns a temporary directory for tests
func getTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "state.json")
}

func TestNewManager(t *testing.T) {
	t.Run("creates new manager with empty state", func(t *testing.T) {
		filePath := getTempDir(t)
		logger := getTestLogger()

		mgr, err := NewManager(filePath, logger)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if mgr == nil {
			t.Fatal("Expected manager to be created")
		}

		if mgr.GetCount() != 0 {
			t.Errorf("Expected empty state, got %d items", mgr.GetCount())
		}
	})

	t.Run("creates directory if not exists", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "subdir", "state.json")
		logger := getTestLogger()

		mgr, err := NewManager(filePath, logger)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if mgr == nil {
			t.Fatal("Expected manager to be created")
		}

		// Check directory was created
		if _, err := os.Stat(filepath.Dir(filePath)); os.IsNotExist(err) {
			t.Error("Expected directory to be created")
		}
	})

	t.Run("loads existing state from disk", func(t *testing.T) {
		filePath := getTempDir(t)
		logger := getTestLogger()

		// Create initial manager and add state
		mgr1, err := NewManager(filePath, logger)
		if err != nil {
			t.Fatalf("Failed to create initial manager: %v", err)
		}

		state := mgr1.Create("example.com")
		if state == nil {
			t.Fatal("Expected state to be created")
		}

		// Create new manager and verify state is loaded
		mgr2, err := NewManager(filePath, logger)
		if err != nil {
			t.Fatalf("Failed to create second manager: %v", err)
		}

		if mgr2.GetCount() != 1 {
			t.Errorf("Expected 1 state, got %d", mgr2.GetCount())
		}

		loaded, err := mgr2.GetByDomain("example.com")
		if err != nil {
			t.Fatalf("Expected to find state, got error: %v", err)
		}

		if loaded.Domain != "example.com" {
			t.Errorf("Expected domain 'example.com', got '%s'", loaded.Domain)
		}

		if loaded.ID != state.ID {
			t.Error("Expected same ID")
		}
	})
}

func TestManager_Create(t *testing.T) {
	t.Run("creates new state with valid fields", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("test.com")

		if state == nil {
			t.Fatal("Expected state to be created")
		}

		if state.Domain != "test.com" {
			t.Errorf("Expected domain 'test.com', got '%s'", state.Domain)
		}

		if state.Status != StatusPending {
			t.Errorf("Expected status '%s', got '%s'", StatusPending, state.Status)
		}

		if state.CurrentStep != StepNone {
			t.Errorf("Expected step %d, got %d", StepNone, state.CurrentStep)
		}

		if state.ID == "" {
			t.Error("Expected non-empty ID")
		}

		if state.Retries != 0 {
			t.Errorf("Expected 0 retries, got %d", state.Retries)
		}

		if state.CreatedAt.IsZero() {
			t.Error("Expected CreatedAt to be set")
		}

		if state.UpdatedAt.IsZero() {
			t.Error("Expected UpdatedAt to be set")
		}
	})

	t.Run("persists state to disk", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr1, _ := NewManager(filePath, getTestLogger())

		state := mgr1.Create("persist.com")

		// Create new manager to load from disk
		mgr2, _ := NewManager(filePath, getTestLogger())
		loaded, err := mgr2.Get(state.ID)

		if err != nil {
			t.Fatalf("Expected to find state, got error: %v", err)
		}

		if loaded.Domain != state.Domain {
			t.Error("Expected domain to be persisted")
		}
	})

	t.Run("creates unique IDs for each state", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state1 := mgr.Create("domain1.com")
		state2 := mgr.Create("domain2.com")

		if state1.ID == state2.ID {
			t.Error("Expected different IDs")
		}
	})
}

func TestManager_Get(t *testing.T) {
	t.Run("retrieves existing state by ID", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		created := mgr.Create("get.com")
		retrieved, err := mgr.Get(created.ID)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if retrieved == nil {
			t.Fatal("Expected state to be retrieved")
		}

		if retrieved.ID != created.ID {
			t.Errorf("Expected ID '%s', got '%s'", created.ID, retrieved.ID)
		}
	})

	t.Run("returns error for non-existent ID", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		_, err := mgr.Get("non-existent-id")

		if err != ErrStateNotFound {
			t.Errorf("Expected ErrStateNotFound, got %v", err)
		}
	})

	t.Run("returns a copy, not the original", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		created := mgr.Create("copy.com")
		retrieved, _ := mgr.Get(created.ID)

		// Modify retrieved state
		retrieved.Status = StatusFailed

		// Get again and verify original is unchanged
		again, _ := mgr.Get(created.ID)

		if again.Status != StatusPending {
			t.Error("Modifying retrieved state should not affect stored state")
		}
	})
}

func TestManager_GetByDomain(t *testing.T) {
	t.Run("retrieves existing state by domain", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		created := mgr.Create("bydomain.com")
		retrieved, err := mgr.GetByDomain("bydomain.com")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if retrieved.ID != created.ID {
			t.Error("Expected same state")
		}
	})

	t.Run("returns error for non-existent domain", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		_, err := mgr.GetByDomain("nonexistent.com")

		if err != ErrStateNotFound {
			t.Errorf("Expected ErrStateNotFound, got %v", err)
		}
	})
}

func TestManager_Update(t *testing.T) {
	t.Run("updates existing state", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("update.com")
		state.Status = StatusProvisioning
		state.CurrentStep = StepDNSZone
		state.ZoneID = 12345

		err := mgr.Update(state)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify update
		retrieved, _ := mgr.Get(state.ID)
		if retrieved.Status != StatusProvisioning {
			t.Errorf("Expected status '%s', got '%s'", StatusProvisioning, retrieved.Status)
		}

		if retrieved.CurrentStep != StepDNSZone {
			t.Errorf("Expected step %d, got %d", StepDNSZone, retrieved.CurrentStep)
		}

		if retrieved.ZoneID != 12345 {
			t.Errorf("Expected ZoneID 12345, got %d", retrieved.ZoneID)
		}
	})

	t.Run("preserves creation time", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("preserve.com")
		originalCreatedAt := state.CreatedAt

		// Wait a bit to ensure time difference
		time.Sleep(10 * time.Millisecond)

		state.Status = StatusProvisioning
		mgr.Update(state)

		retrieved, _ := mgr.Get(state.ID)
		if !retrieved.CreatedAt.Equal(originalCreatedAt) {
			t.Error("CreatedAt should be preserved")
		}

		if !retrieved.UpdatedAt.After(originalCreatedAt) {
			t.Error("UpdatedAt should be updated")
		}
	})

	t.Run("returns error for non-existent state", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := &ProvisionState{
			ID:     "non-existent",
			Domain: "fake.com",
		}

		err := mgr.Update(state)

		if err != ErrStateNotFound {
			t.Errorf("Expected ErrStateNotFound, got %v", err)
		}
	})

	t.Run("persists update to disk", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr1, _ := NewManager(filePath, getTestLogger())

		state := mgr1.Create("persist-update.com")
		state.Status = StatusSuccess
		mgr1.Update(state)

		// Load in new manager
		mgr2, _ := NewManager(filePath, getTestLogger())
		retrieved, _ := mgr2.Get(state.ID)

		if retrieved.Status != StatusSuccess {
			t.Error("Update should be persisted")
		}
	})
}

func TestManager_Delete(t *testing.T) {
	t.Run("deletes existing state", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("delete.com")
		err := mgr.Delete(state.ID)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if mgr.GetCount() != 0 {
			t.Errorf("Expected 0 states, got %d", mgr.GetCount())
		}

		// Verify state is gone
		_, err = mgr.Get(state.ID)
		if err != ErrStateNotFound {
			t.Error("Expected state to be deleted")
		}

		// Verify domain index is cleared
		_, err = mgr.GetByDomain("delete.com")
		if err != ErrStateNotFound {
			t.Error("Expected domain to be removed from index")
		}
	})

	t.Run("returns error for non-existent state", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		err := mgr.Delete("non-existent")

		if err != ErrStateNotFound {
			t.Errorf("Expected ErrStateNotFound, got %v", err)
		}
	})
}

func TestManager_ListPending(t *testing.T) {
	t.Run("returns pending and provisioning states", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		mgr.Create("pending1.com")
		s2 := mgr.Create("pending2.com")
		s2.Status = StatusProvisioning
		mgr.Update(s2)

		s3 := mgr.Create("success.com")
		s3.Status = StatusSuccess
		mgr.Update(s3)

		s4 := mgr.Create("failed.com")
		s4.Status = StatusFailed
		mgr.Update(s4)

		pending := mgr.ListPending()

		if len(pending) != 2 {
			t.Errorf("Expected 2 pending states, got %d", len(pending))
		}
	})

	t.Run("returns empty list when no pending states", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		s := mgr.Create("success.com")
		s.Status = StatusSuccess
		mgr.Update(s)

		pending := mgr.ListPending()

		if len(pending) != 0 {
			t.Errorf("Expected 0 pending states, got %d", len(pending))
		}
	})
}

func TestManager_ListFailed(t *testing.T) {
	t.Run("returns failed states only", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		s1 := mgr.Create("failed1.com")
		s1.Status = StatusFailed
		mgr.Update(s1)

		s2 := mgr.Create("failed2.com")
		s2.Status = StatusFailed
		mgr.Update(s2)

		s3 := mgr.Create("success.com")
		s3.Status = StatusSuccess
		mgr.Update(s3)

		failed := mgr.ListFailed()

		if len(failed) != 2 {
			t.Errorf("Expected 2 failed states, got %d", len(failed))
		}
	})
}

func TestManager_Recover(t *testing.T) {
	t.Run("returns pending and failed states with retries remaining", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		// Pending state
		mgr.Create("pending.com")

		// Failed with retries remaining
		s1 := mgr.Create("failed-low-retries.com")
		s1.Status = StatusFailed
		s1.Retries = 2
		mgr.Update(s1)

		// Failed with max retries
		s2 := mgr.Create("failed-max-retries.com")
		s2.Status = StatusFailed
		s2.Retries = 5
		mgr.Update(s2)

		// Success
		s3 := mgr.Create("success.com")
		s3.Status = StatusSuccess
		mgr.Update(s3)

		recoverable := mgr.Recover()

		if len(recoverable) != 2 {
			t.Errorf("Expected 2 recoverable states, got %d", len(recoverable))
		}
	})
}

func TestManager_IncrementStep(t *testing.T) {
	t.Run("increments step number", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("step.com")
		err := mgr.IncrementStep(state.ID)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		retrieved, _ := mgr.Get(state.ID)
		if retrieved.CurrentStep != 1 {
			t.Errorf("Expected step 1, got %d", retrieved.CurrentStep)
		}
	})

	t.Run("returns error for non-existent state", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		err := mgr.IncrementStep("non-existent")

		if err != ErrStateNotFound {
			t.Errorf("Expected ErrStateNotFound, got %v", err)
		}
	})
}

func TestManager_SetError(t *testing.T) {
	t.Run("sets error and increments retries", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("error.com")
		err := mgr.SetError(state.ID, "API error")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		retrieved, _ := mgr.Get(state.ID)
		if retrieved.Status != StatusFailed {
			t.Errorf("Expected status '%s', got '%s'", StatusFailed, retrieved.Status)
		}

		if retrieved.Error != "API error" {
			t.Errorf("Expected error 'API error', got '%s'", retrieved.Error)
		}

		if retrieved.Retries != 1 {
			t.Errorf("Expected 1 retry, got %d", retrieved.Retries)
		}
	})
}

func TestManager_MarkSuccess(t *testing.T) {
	t.Run("marks state as success", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("mark-success.com")
		err := mgr.MarkSuccess(state.ID)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		retrieved, _ := mgr.Get(state.ID)
		if retrieved.Status != StatusSuccess {
			t.Errorf("Expected status '%s', got '%s'", StatusSuccess, retrieved.Status)
		}

		if retrieved.Error != "" {
			t.Errorf("Expected empty error, got '%s'", retrieved.Error)
		}
	})
}

func TestManager_MarkProvisioning(t *testing.T) {
	t.Run("marks state as provisioning", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		state := mgr.Create("mark-provisioning.com")
		err := mgr.MarkProvisioning(state.ID)

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		retrieved, _ := mgr.Get(state.ID)
		if retrieved.Status != StatusProvisioning {
			t.Errorf("Expected status '%s', got '%s'", StatusProvisioning, retrieved.Status)
		}
	})
}

func TestManager_Clear(t *testing.T) {
	t.Run("clears all states", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		mgr.Create("domain1.com")
		mgr.Create("domain2.com")

		err := mgr.Clear()

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if mgr.GetCount() != 0 {
			t.Errorf("Expected 0 states, got %d", mgr.GetCount())
		}
	})
}

func TestManager_ConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent operations safely", func(t *testing.T) {
		filePath := getTempDir(t)
		mgr, _ := NewManager(filePath, getTestLogger())

		var wg sync.WaitGroup
		numGoroutines := 10
		createsPerGoroutine := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				for j := 0; j < createsPerGoroutine; j++ {
					domain := fmt.Sprintf("concurrent-%d-%d.com", n, j)
					mgr.Create(domain)
				}
			}(i)
		}

		wg.Wait()

		expectedCount := numGoroutines * createsPerGoroutine
		if mgr.GetCount() != expectedCount {
			t.Errorf("Expected %d states, got %d", expectedCount, mgr.GetCount())
		}
	})
}

func TestStepName(t *testing.T) {
	tests := []struct {
		step   int
		noname string
	}{
		{StepNone, "none"},
		{StepDNSZone, "dns_zone"},
		{StepDNSRecords, "dns_records"},
		{StepPullZone, "pull_zone"},
		{StepCNAMESync, "cname_sync"},
		{999, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.noname, func(t *testing.T) {
			name := StepName(tt.step)
			if name != tt.noname {
				t.Errorf("Expected name '%s', got '%s'", tt.noname, name)
			}
		})
	}
}

func TestProvisionState_JSON(t *testing.T) {
	t.Run("serializes and deserializes correctly", func(t *testing.T) {
		state := &ProvisionState{
			ID:          "test-id",
			Domain:      "json.com",
			Status:      StatusProvisioning,
			CurrentStep: StepDNSZone,
			ZoneID:      123,
			PullZoneID:  456,
			CDNHostname: "cdn.example.com",
			Error:       "",
			Retries:     2,
			CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		}

		// Marshal to JSON
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		// Unmarshal from JSON
		var decoded ProvisionState
		err = json.Unmarshal(data, &decoded)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Verify fields
		if decoded.ID != state.ID {
			t.Errorf("Expected ID '%s', got '%s'", state.ID, decoded.ID)
		}

		if decoded.Domain != state.Domain {
			t.Errorf("Expected domain '%s', got '%s'", state.Domain, decoded.Domain)
		}

		if decoded.Status != state.Status {
			t.Errorf("Expected status '%s', got '%s'", state.Status, decoded.Status)
		}

		if decoded.CurrentStep != state.CurrentStep {
			t.Errorf("Expected step %d, got %d", state.CurrentStep, decoded.CurrentStep)
		}

		if decoded.ZoneID != state.ZoneID {
			t.Errorf("Expected ZoneID %d, got %d", state.ZoneID, decoded.ZoneID)
		}

		if decoded.PullZoneID != state.PullZoneID {
			t.Errorf("Expected PullZoneID %d, got %d", state.PullZoneID, decoded.PullZoneID)
		}

		if decoded.CDNHostname != state.CDNHostname {
			t.Errorf("Expected CDNHostname '%s', got '%s'", state.CDNHostname, decoded.CDNHostname)
		}

		if decoded.Retries != state.Retries {
			t.Errorf("Expected Retries %d, got %d", state.Retries, decoded.Retries)
		}
	})
}
