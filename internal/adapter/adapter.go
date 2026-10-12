package adapter

import (
	"context"
	"deploytool/internal/artifact"
	"deploytool/internal/config"
)

type ServiceStatus struct {
	Name       string `json:"name"`
	Installed  bool   `json:"installed"`
	Running    bool   `json:"running"`
	Healthy    bool   `json:"healthy"`
	Skipped    bool   `json:"skipped"`
	Reason     string `json:"reason,omitempty"`
	RolledBack bool   `json:"rolled_back,omitempty"`
}
type ServiceAdapter interface {
	Install(context.Context, *config.Service) error
	Uninstall(context.Context, *config.Service) error
	Start(context.Context, *config.Service) error
	Stop(context.Context, *config.Service) error
	Update(context.Context, *config.Service, *artifact.Diff) error
	Status(context.Context, *config.Service) (*ServiceStatus, error)
}
