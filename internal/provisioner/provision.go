package provisioner

import (
	_ "github.com/google/uuid"
	_ "go.uber.org/zap"
)
type Provisioner struct {
	// TODO: Add fields for bunny client, logger, etc.
}

// NewProvisioner creates a new provisioner
func NewProvisioner() *Provisioner {
	return &Provisioner{}
}
