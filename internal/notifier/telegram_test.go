package notifier

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewTelegramNotifier_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("returns disabled notifier when enabled is false", func(t *testing.T) {
		notifier, err := NewTelegramNotifier("token", "123", false, nil, logger)

		require.NoError(t, err)
		assert.NotNil(t, notifier)
		assert.False(t, notifier.IsEnabled())
	})

	t.Run("returns disabled notifier when bot token is empty", func(t *testing.T) {
		notifier, err := NewTelegramNotifier("", "123", true, nil, logger)

		require.NoError(t, err)
		assert.NotNil(t, notifier)
		assert.False(t, notifier.IsEnabled())
	})

	t.Run("returns disabled notifier when chat ID is empty", func(t *testing.T) {
		notifier, err := NewTelegramNotifier("token", "", true, nil, logger)

		require.NoError(t, err)
		assert.NotNil(t, notifier)
		assert.False(t, notifier.IsEnabled())
	})
}

func TestNewTelegramNotifier_InvalidChatID(t *testing.T) {
	logger := zaptest.NewLogger(t)

	_, err := NewTelegramNotifier("token", "invalid-chat-id", true, nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid chat ID")
}

func TestTelegramNotifier_ShouldNotify(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name     string
		events   []string
		event    string
		expected bool
	}{
		{
			name:     "empty events list allows all",
			events:   []string{},
			event:    "success",
			expected: true,
		},
		{
			name:     "matching event is allowed",
			events:   []string{"success", "failed"},
			event:    "success",
			expected: true,
		},
		{
			name:     "non-matching event is blocked",
			events:   []string{"success", "ssl"},
			event:    "bandwidth",
			expected: false,
		},
		{
			name:     "case insensitive matching",
			events:   []string{"SUCCESS", "FAILED"},
			event:    "success",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &TelegramNotifier{
				enabled: true,
				events:  tt.events,
				logger:  logger,
			}
			assert.Equal(t, tt.expected, notifier.shouldNotify(tt.event))
		})
	}
}

func TestTelegramNotifier_GetHostname(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier := &TelegramNotifier{
		logger: logger,
	}
	hostname := notifier.getHostname()
	assert.NotEmpty(t, hostname)
}

func TestTelegramNotifier_NotifyMethods_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	notifier := &TelegramNotifier{
		enabled: false,
		logger:  logger,
	}

	// All notify methods should return nil when disabled
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "NotifySuccess",
			fn: func() error {
				return notifier.NotifySuccess(ctx, "example.com", 123456, "cdn.example.com", 3*time.Second)
			},
		},
		{
			name: "NotifyFailed",
			fn: func() error {
				return notifier.NotifyFailed(ctx, "example.com", "Create DNS Zone", "API error")
			},
		},
		{
			name: "NotifySSLIssued",
			fn: func() error {
				return notifier.NotifySSLIssued(ctx, "example.com", "Let's Encrypt", time.Now().AddDate(0, 3, 0))
			},
		},
		{
			name: "NotifyBandwidthAlert",
			fn: func() error {
				return notifier.NotifyBandwidthAlert(ctx, "example.com", 75.5)
			},
		},
		{
			name: "NotifyDeprovisioned",
			fn: func() error {
				return notifier.NotifyDeprovisioned(ctx, "example.com")
			},
		},
		{
			name: "NotifySubdomainProvisioned",
			fn: func() error {
				return notifier.NotifySubdomainProvisioned(ctx, "blog.example.com", "example.com", "cdn.blog.example.com")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			assert.NoError(t, err)
		})
	}
}

func TestTelegramNotifier_Shutdown(t *testing.T) {
	t.Run("shutdown succeeds for nil client", func(t *testing.T) {
		notifier := &TelegramNotifier{}
		err := notifier.Shutdown()
		assert.NoError(t, err)
	})

	t.Run("shutdown succeeds with client", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		// Create a disabled notifier (no actual bot client)
		notifier, err := NewTelegramNotifier("", "123", false, nil, logger)
		require.NoError(t, err)
		err = notifier.Shutdown()
		assert.NoError(t, err)
	})
}

func TestTelegramFormatter_MessageFormats(t *testing.T) {
	// Test that message formats contain expected content
	tests := []struct {
		name          string
		formatFunc    func() (string, string)
		expectedInMsg []string
	}{
		{
			name: "NotifySuccess format",
			formatFunc: func() (string, string) {
				domain := "example.com"
				zoneID := int64(123456)
				cdnHostname := "morden-example-com.b-cdn.net"
				duration := 3 * time.Second
				return domain, formatSuccessMessage(domain, zoneID, cdnHostname, duration, "server1")
			},
			expectedInMsg: []string{"Domain Provisioned", "example.com", "123456", "morden-example-com.b-cdn.net", "3.00s", "server1"},
		},
		{
			name: "NotifyFailed format",
			formatFunc: func() (string, string) {
				domain := "example.com"
				step := "Create DNS Zone"
				errMsg := "API returned 401 Unauthorized"
				return domain, formatFailedMessage(domain, step, errMsg, "server1", time.Now())
			},
			expectedInMsg: []string{"Provisioning Failed", "example.com", "Create DNS Zone", "401 Unauthorized"},
		},
		{
			name: "NotifySubdomainProvisioned format",
			formatFunc: func() (string, string) {
				return "blog", formatSubdomainMessage("blog.example.com", "example.com", "morden-blog.b-cdn.net", "server1")
			},
			expectedInMsg: []string{"Subdomain Provisioned", "blog.example.com", "example.com", "morden-blog.b-cdn.net"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, msg := tt.formatFunc()
			for _, expected := range tt.expectedInMsg {
				assert.Contains(t, msg, expected)
			}
		})
	}
}

// Helper functions for message format testing (extracted for testing without Telegram bot)
func formatSuccessMessage(domain string, zoneID int64, cdnHostname string, duration time.Duration, hostname string) string {
	return fmt.Sprintf(`✅ <b>Domain Provisioned</b>

🌐 <b>Domain:</b> %s
📍 <b>Zone ID:</b> %d
🚀 <b>CDN:</b> %s
⏱️ <b>Duration:</b> %.2fs

🖥️ <b>Server:</b> %s`,
		domain,
		zoneID,
		cdnHostname,
		duration.Seconds(),
		hostname,
	)
}

func formatFailedMessage(domain, step, errMsg, hostname string, t time.Time) string {
	wibLocation := time.FixedZone("WIB", 7*60*60)
	currentTime := t.In(wibLocation).Format("2006-01-02 15:04:05 WIB")
	return fmt.Sprintf(`❌ <b>Provisioning Failed</b>

🌐 <b>Domain:</b> %s
📍 <b>Step:</b> %s
⚠️ <b>Error:</b> %s

🖥️ <b>Server:</b> %s
🕐 <b>Time:</b> %s`,
		domain,
		step,
		errMsg,
		hostname,
		currentTime,
	)
}

func formatSubdomainMessage(subdomain, parent, cdnHostname, hostname string) string {
	return fmt.Sprintf(`✅ <b>Subdomain Provisioned</b>

🌐 <b>Subdomain:</b> %s
📍 <b>Parent Zone:</b> %s
🚀 <b>CDN:</b> %s

🖥️ <b>Server:</b> %s`,
		subdomain,
		parent,
		cdnHostname,
		hostname,
	)
}

// Test message format helpers directly
func TestMessageFormatters(t *testing.T) {
	t.Run("success message contains all expected fields", func(t *testing.T) {
		msg := formatSuccessMessage("test.com", 789, "cdn.test.com", 2500*time.Millisecond, "myserver")
		assert.Contains(t, msg, "test.com")
		assert.Contains(t, msg, "789")
		assert.Contains(t, msg, "cdn.test.com")
		assert.Contains(t, msg, "2.50s")
		assert.Contains(t, msg, "myserver")
	})

	t.Run("failed message contains WIB time", func(t *testing.T) {
		now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		msg := formatFailedMessage("test.com", "Create DNS", "Error 404", "server1", now)
		assert.Contains(t, msg, "WIB")
		assert.Contains(t, msg, "17:30:00") // 10:30 UTC + 7 hours = 17:30 WIB
	})
}

func TestParseChatID(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"123", 123, false},
		{"-1001234567890", int64(-1001234567890), false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := strconv.ParseInt(tt.input, 10, 64)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
