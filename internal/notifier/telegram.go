package notifier

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"go.uber.org/zap"
)

// TelegramNotifier handles Telegram notifications for provisioning events
type TelegramNotifier struct {
	client  *telego.Bot
	chatID  int64
	enabled bool
	events  []string
	logger  *zap.Logger
}

// NewTelegramNotifier creates a new Telegram notifier instance
func NewTelegramNotifier(botToken, chatID string, enabled bool, events []string, logger *zap.Logger) (*TelegramNotifier, error) {
	if !enabled || botToken == "" || chatID == "" {
		return &TelegramNotifier{
			enabled: false,
			logger:  logger,
			events:  events,
		}, nil
	}

	// Parse chat ID
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID: %w", err)
	}

	// Create bot client
	bot, err := telego.NewBot(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	// Test bot connection
	if _, err := bot.GetMe(); err != nil {
		return nil, fmt.Errorf("failed to connect to telegram API: %w", err)
	}

	return &TelegramNotifier{
		client:  bot,
		chatID:  chatIDInt,
		enabled: true,
		events:  events,
		logger:  logger,
	}, nil
}

// IsEnabled returns whether the notifier is enabled
func (t *TelegramNotifier) IsEnabled() bool {
	return t.enabled
}

// Shutdown gracefully shuts down the notifier
func (t *TelegramNotifier) Shutdown() error {
	// No-op for telego bot, cleanup is handled automatically
	return nil
}

// send sends a message to the configured chat
func (t *TelegramNotifier) send(ctx context.Context, message string) error {
	if !t.enabled {
		return nil
	}

	// Create message params - use only ID field for integer chat ID
	params := telego.SendMessageParams{
		ChatID:    telego.ChatID{ID: t.chatID},
		Text:      message,
		ParseMode: "HTML",
	}

	// Send message
	// Note: telego doesn't have SendMessageWithContext, so we use regular SendMessage
	// Context cancellation will be handled at a higher level
	_, err := t.client.SendMessage(&params)
	if err != nil {
		t.logger.Error("failed to send telegram notification",
			zap.Error(err),
			zap.String("message", message),
		)
		return err
	}

	return nil
}

// shouldNotify checks if an event type should be notified
func (t *TelegramNotifier) shouldNotify(event string) bool {
	if len(t.events) == 0 {
		// If no specific events configured, notify for all
		return true
	}
	for _, e := range t.events {
		if strings.EqualFold(e, event) {
			return true
		}
	}
	return false
}

// getHostname returns the server hostname
func (t *TelegramNotifier) getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// NotifySuccess sends a notification on successful domain provisioning
func (t *TelegramNotifier) NotifySuccess(ctx context.Context, domain string, zoneID int64, cdnHostname string, duration time.Duration) error {
	if !t.shouldNotify("success") {
		return nil
	}

	message := fmt.Sprintf(`✅ <b>Domain Provisioned</b>

🌐 <b>Domain:</b> %s
📍 <b>Zone ID:</b> %d
🚀 <b>CDN:</b> %s
⏱️ <b>Duration:</b> %.2fs

🖥️ <b>Server:</b> %s`,
		domain,
		zoneID,
		cdnHostname,
		duration.Seconds(),
		t.getHostname(),
	)

	return t.send(ctx, message)
}

// NotifyFailed sends a notification when provisioning fails
func (t *TelegramNotifier) NotifyFailed(ctx context.Context, domain string, step string, errMsg string) error {
	if !t.shouldNotify("failed") {
		return nil
	}

	// Get current time in WIB (GMT+7)
	wibLocation := time.FixedZone("WIB", 7*60*60)
	currentTime := time.Now().In(wibLocation).Format("2006-01-02 15:04:05 WIB")

	message := fmt.Sprintf(`❌ <b>Provisioning Failed</b>

🌐 <b>Domain:</b> %s
📍 <b>Step:</b> %s
⚠️ <b>Error:</b> %s

🖥️ <b>Server:</b> %s
🕐 <b>Time:</b> %s`,
		domain,
		step,
		errMsg,
		t.getHostname(),
		currentTime,
	)

	return t.send(ctx, message)
}

// NotifySSLIssued sends a notification when an SSL certificate is issued
func (t *TelegramNotifier) NotifySSLIssued(ctx context.Context, domain string, issuer string, expires time.Time) error {
	if !t.shouldNotify("ssl") {
		return nil
	}

	message := fmt.Sprintf(`🔐 <b>SSL Certificate Issued</b>

🌐 <b>Domain:</b> %s
📜 <b>Issuer:</b> %s
📅 <b>Expires:</b> %s

🖥️ <b>Server:</b> %s`,
		domain,
		issuer,
		expires.Format("2006-01-02"),
		t.getHostname(),
	)

	return t.send(ctx, message)
}

// NotifyBandwidthAlert sends a notification for bandwidth usage alerts
func (t *TelegramNotifier) NotifyBandwidthAlert(ctx context.Context, domain string, percentIncrease float64) error {
	if !t.shouldNotify("bandwidth") {
		return nil
	}

	message := fmt.Sprintf(`⚠️ <b>Bandwidth Alert</b>

🌐 <b>Domain:</b> %s
📈 <b>Increase:</b> %.0f%%

🖥️ <b>Server:</b> %s`,
		domain,
		percentIncrease,
		t.getHostname(),
	)

	return t.send(ctx, message)
}

// NotifyDeprovisioned sends a notification when a domain is removed
func (t *TelegramNotifier) NotifyDeprovisioned(ctx context.Context, domain string) error {
	if !t.shouldNotify("deprovisioned") {
		return nil
	}

	message := fmt.Sprintf(`🗑️ <b>Domain Removed</b>

🌐 <b>Domain:</b> %s
📍 <b>DNS Zone:</b> Deleted
🚀 <b>CDN Pull Zone:</b> Deleted

🖥️ <b>Server:</b> %s`,
		domain,
		t.getHostname(),
	)

	return t.send(ctx, message)
}

// NotifySubdomainProvisioned sends a notification when a subdomain is provisioned
func (t *TelegramNotifier) NotifySubdomainProvisioned(ctx context.Context, subdomain string, parent string, cdnHostname string) error {
	if !t.shouldNotify("subdomain") {
		return nil
	}

	message := fmt.Sprintf(`✅ <b>Subdomain Provisioned</b>

🌐 <b>Subdomain:</b> %s
📍 <b>Parent Zone:</b> %s
🚀 <b>CDN:</b> %s

🖥️ <b>Server:</b> %s`,
		subdomain,
		parent,
		cdnHostname,
		t.getHostname(),
	)

	return t.send(ctx, message)
}

// SendRaw sends a raw message to Telegram (used by scheduler for summaries)
func (t *TelegramNotifier) SendRaw(ctx context.Context, message string) error {
	if !t.enabled {
		return nil
	}

	// Ensure the message ends with a newline for proper formatting
	if len(message) > 0 && message[len(message)-1] != '\n' {
		message += "\n"
	}

	return t.send(ctx, message)
}
