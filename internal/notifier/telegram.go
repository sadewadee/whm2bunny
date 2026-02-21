package notifier

import (
	_ "github.com/mymmrac/telego"
	_ "github.com/google/uuid"
	_ "go.uber.org/zap"
)
type Notifier struct {
	// TODO: Add fields for telegram bot, chat ID, etc.
}

// NewNotifier creates a new notifier
func NewNotifier() *Notifier {
	return &Notifier{}
}

// Notify sends a notification message
func (n *Notifier) Notify(message string) error {
	// TODO: Implement telegram notification
	return nil
}
