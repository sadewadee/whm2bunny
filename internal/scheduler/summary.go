package scheduler

import (
	_ "github.com/robfig/cron/v3"
	_ "github.com/google/uuid"
	_ "go.uber.org/zap"
)
type Scheduler struct {
	// TODO: Add fields for cron, notifier, logger, etc.
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	// TODO: Implement scheduler
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	// TODO: Implement stop
}
