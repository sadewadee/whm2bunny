package scheduler

import (
	"testing"
	"time"

	"github.com/mordenhost/whm2bunny/config"
	"github.com/mordenhost/whm2bunny/internal/bunny"
	"github.com/mordenhost/whm2bunny/internal/notifier"
	"github.com/mordenhost/whm2bunny/internal/state"
	"go.uber.org/zap"
)

func TestNewScheduler(t *testing.T) {
	cfg := &config.Config{
		Telegram: config.TelegramConfig{
			Summary: config.TelegramSummaryConfig{
				Enabled:                 false,
				Schedule:                "0 9 * * *",
				WeeklySchedule:          "0 9 * * 1",
				Timezone:                "Asia/Jakarta",
				IncludeTopBandwidth:     5,
				BandwidthAlertThreshold: 50,
			},
		},
	}

	logger := zap.NewNop()
	bunnyClient := bunny.NewClient("test-key", bunny.WithLogger(logger))
	snapshotStore, _ := state.NewSnapshotStore("/tmp/test-snapshots.json", logger)
	telegramNotifier, _ := notifier.NewTelegramNotifier("", "", false, nil, logger)

	scheduler := NewScheduler(cfg, bunnyClient, telegramNotifier, snapshotStore, logger)

	if scheduler == nil {
		t.Fatal("Expected non-nil scheduler")
	}

	if scheduler.config != cfg {
		t.Error("Expected config to be set")
	}

	if scheduler.bunnyClient != bunnyClient {
		t.Error("Expected bunnyClient to be set")
	}
}

func TestScheduler_Start_WhenDisabled(t *testing.T) {
	cfg := &config.Config{
		Telegram: config.TelegramConfig{
			Summary: config.TelegramSummaryConfig{
				Enabled: false,
			},
		},
	}

	logger := zap.NewNop()
	bunnyClient := bunny.NewClient("test-key")
	snapshotStore, _ := state.NewSnapshotStore("/tmp/test-snapshots.json", logger)
	telegramNotifier, _ := notifier.NewTelegramNotifier("", "", false, nil, logger)

	scheduler := NewScheduler(cfg, bunnyClient, telegramNotifier, snapshotStore, logger)

	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Expected no error when starting disabled scheduler, got %v", err)
	}

	if scheduler.running {
		t.Error("Expected scheduler to not be running when disabled")
	}
}

func TestScheduler_Start_WhenEnabled(t *testing.T) {
	cfg := &config.Config{
		Telegram: config.TelegramConfig{
			Summary: config.TelegramSummaryConfig{
				Enabled:        true,
				Schedule:       "0 9 * * *",
				WeeklySchedule: "0 9 * * 1",
				Timezone:       "UTC",
			},
		},
	}

	logger := zap.NewNop()
	bunnyClient := bunny.NewClient("test-key")
	snapshotStore, _ := state.NewSnapshotStore("/tmp/test-snapshots.json", logger)
	telegramNotifier, _ := notifier.NewTelegramNotifier("", "", false, nil, logger)

	scheduler := NewScheduler(cfg, bunnyClient, telegramNotifier, snapshotStore, logger)

	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Expected no error when starting enabled scheduler, got %v", err)
	}

	if !scheduler.running {
		t.Error("Expected scheduler to be running when enabled")
	}

	// Stop the scheduler
	scheduler.Stop()
}

func TestScheduler_Stop(t *testing.T) {
	cfg := &config.Config{
		Telegram: config.TelegramConfig{
			Summary: config.TelegramSummaryConfig{
				Enabled:  true,
				Schedule: "0 9 * * *",
			},
		},
	}

	logger := zap.NewNop()
	bunnyClient := bunny.NewClient("test-key")
	snapshotStore, _ := state.NewSnapshotStore("/tmp/test-snapshots.json", logger)
	telegramNotifier, _ := notifier.NewTelegramNotifier("", "", false, nil, logger)

	scheduler := NewScheduler(cfg, bunnyClient, telegramNotifier, snapshotStore, logger)

	// Start the scheduler
	_ = scheduler.Start()

	// Stop the scheduler
	scheduler.Stop()

	if scheduler.running {
		t.Error("Expected scheduler to not be running after stop")
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"billions", 1_500_000_000, "1.50B"},
		{"millions", 2_500_000, "2.50M"},
		{"thousands", 12_500, "12.50K"},
		{"hundreds", 500, "500"},
		{"zero", 0, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatNumber(tt.input)
			if result != tt.expected {
				t.Errorf("formatNumber(%d) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetHostname(t *testing.T) {
	s := &Scheduler{}
	hostname := s.getHostname()
	if hostname == "" {
		t.Error("Expected non-empty hostname")
	}
}

func TestGetTimezone(t *testing.T) {
	tests := []struct {
		name        string
		timezone    string
		expectError bool
	}{
		{"UTC", "UTC", false},
		{"Asia/Jakarta", "Asia/Jakarta", false},
		{"Invalid", "Invalid/Timezone", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Scheduler{
				config: &config.Config{
					Telegram: config.TelegramConfig{
						Summary: config.TelegramSummaryConfig{
							Timezone: tt.timezone,
						},
					},
				},
			}
			loc, err := s.getTimezone()
			if tt.expectError && err == nil {
				t.Error("Expected error for invalid timezone")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for valid timezone, got %v", err)
			}
			if !tt.expectError && loc == nil {
				t.Error("Expected non-nil location")
			}
		})
	}
}

func TestFormatDailySummary(t *testing.T) {
	s := &Scheduler{}
	date := time.Date(2024, 2, 22, 0, 0, 0, 0, time.UTC)

	topZones := []bunny.BandwidthEntry{
		{ZoneName: "example.com", Bandwidth: 45 * 1024 * 1024 * 1024},
		{ZoneName: "test.com", Bandwidth: 30 * 1024 * 1024 * 1024},
	}

	message := s.formatDailySummary(date, 75*1024*1024*1024, 1_200_000, 94.5, topZones)

	if message == "" {
		t.Error("Expected non-empty message")
	}

	// Check for key components
	if !contains(message, "Daily Summary") {
		t.Error("Expected 'Daily Summary' in message")
	}
	if !contains(message, "example.com") {
		t.Error("Expected 'example.com' in message")
	}
	if !contains(message, "75.00 GB") {
		t.Error("Expected bandwidth in message")
	}
}

func TestFormatWeeklySummary(t *testing.T) {
	s := &Scheduler{}

	topZones := []bunny.BandwidthEntry{
		{ZoneName: "example.com", Bandwidth: 315 * 1024 * 1024 * 1024},
		{ZoneName: "test.com", Bandwidth: 200 * 1024 * 1024 * 1024},
	}

	message := s.formatWeeklySummary(8, 2024, 875*1024*1024*1024, 8_400_000, 93.2, 15.0, topZones)

	if message == "" {
		t.Error("Expected non-empty message")
	}

	// Check for key components
	if !contains(message, "Weekly Summary") {
		t.Error("Expected 'Weekly Summary' in message")
	}
	if !contains(message, "Week 8, 2024") {
		t.Error("Expected week info in message")
	}
	if !contains(message, "+15%") {
		t.Error("Expected bandwidth change in message")
	}
}

func TestFormatBandwidthAlert(t *testing.T) {
	s := &Scheduler{}

	message := s.formatBandwidthAlert("example.com", 45*1024*1024*1024, 25*1024*1024*1024, 75.0)

	if message == "" {
		t.Error("Expected non-empty message")
	}

	if !contains(message, "Bandwidth Alert") {
		t.Error("Expected 'Bandwidth Alert' in message")
	}
	if !contains(message, "example.com") {
		t.Error("Expected domain in message")
	}
	if !contains(message, "75%") {
		t.Error("Expected percentage in message")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
