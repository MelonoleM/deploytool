package adapter

import (
	"context"
	"deploytool/internal/artifact"
	"deploytool/internal/config"
	"deploytool/internal/process"
	"os"
)

// GenericAdapter makes language/runtime types declarative: commands and artifacts are supplied by config, never hard-coded.
type GenericAdapter struct{}

func NewGeneric() ServiceAdapter                                         { return &GenericAdapter{} }
func (a *GenericAdapter) Install(context.Context, *config.Service) error { return nil }
func (a *GenericAdapter) Uninstall(_ context.Context, s *config.Service) error {
	return os.RemoveAll(s.Workdir)
}
func (a *GenericAdapter) Start(_ context.Context, s *config.Service) error {
	_, e := process.Run(s.Start, s.Workdir)
	return e
}
func (a *GenericAdapter) Stop(_ context.Context, s *config.Service) error               { return nil }
func (a *GenericAdapter) Update(context.Context, *config.Service, *artifact.Diff) error { return nil }
func (a *GenericAdapter) Status(ctx context.Context, s *config.Service) (*ServiceStatus, error) {
	if _, e := os.Stat(s.Workdir); e != nil {
		return &ServiceStatus{Name: s.Name, Reason: "work directory is absent"}, nil
	}
	return &ServiceStatus{Name: s.Name, Installed: true}, nil
}
func RegisterBuiltins(r *Registry) {
	for _, t := range []string{"python", "java", "nodejs", "go", "nginx", "exe"} {
		r.Register(t, func() ServiceAdapter { return NewGeneric() })
	}
}
