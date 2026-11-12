// Package service owns the application services shared by Web and Desktop.
// Agent execution remains in the common internal/control kernel used by CLI,
// ACP, and this service. Transports project commands and events; they do not
// implement session lifecycle, approvals, history, or file operations.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"deepzora/desktop/internal/hostrpc"
	"deepzora/internal/plugin"
	"deepzora/internal/plugincatalog"
	"deepzora/internal/weboptions"
)

type WebOptions = weboptions.Options

// Service owns the common application lifecycle and command surface. Transport
// adapters never construct their own session/controller or approval state.
// App is kept private here as the existing reflected wire-contract target.
type Service struct {
	app      *App
	mu       sync.Mutex
	started  bool
	stopping bool
	cancel   context.CancelFunc
	registry *hostrpc.Registry
}

type ServiceOptions struct {
	// Headless disables native-window recovery and automatic desktop background
	// integrations. Interactive tasks still use exactly the same engine.
	Headless bool
}

func NewService(options ServiceOptions) *Service {
	app := NewApp()
	if options.Headless {
		app.localWeb = true
		app.desktopShell.coordinator = nil
	}
	return &Service{app: app}
}

// Start is called once by the owning transport before admitting commands.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("application service is already started")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.registry == nil {
		registry, err := newDesktopRegistry(s.app)
		if err != nil {
			return err
		}
		s.registry = registry
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.app.startup(serviceCtx)
	s.started = true
	return nil
}

// Contract describes commands/events before startup without opening sessions.
// This is the same contract used by the native preload and the Web handshake.
func (s *Service) Contract() (hostrpc.Contract, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registry == nil {
		registry, err := newDesktopRegistry(s.app)
		if err != nil {
			return hostrpc.Contract{}, err
		}
		s.registry = registry
	}
	return hostrpc.Build(s.registry, hostEventNames), nil
}

// Invoke shares structured errors, secret projection, session identity checks
// and revision/lease enforcement with the desktop host command surface.
func (s *Service) Invoke(ctx context.Context, method string, args []json.RawMessage) (any, error) {
	s.mu.Lock()
	// The native handshake reads build/platform identity before desktop/start.
	identityRead := method == "Version" || method == "Platform"
	if s.registry == nil {
		registry, err := newDesktopRegistry(s.app)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.registry = registry
	}
	registry, started := s.registry, (s.started || identityRead) && !s.stopping
	s.mu.Unlock()
	if !registry.HasCommand(method) {
		return registry.InvokeProjected(ctx, method, args)
	}
	if !started {
		return nil, fmt.Errorf("application service is not started")
	}
	if s.app.localWeb && (method == "ApplyUpdateRequest" || method == "AbandonPendingUpdate") {
		return nil, fmt.Errorf("desktop update operations are unavailable in Web; run deepzora upgrade in the terminal")
	}
	return registry.InvokeProjected(ctx, method, args)
}

// SetEventSink must be called before Start. The common emitter preserves event
// order and keeps transport backpressure outside the execution engine's locks.
func (s *Service) SetEventSink(emit func(context.Context, string, ...any)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("event sink must be installed before service startup")
	}
	s.app.runtimeEvents.emit = emit
	return nil
}

// Shutdown uses the same persistence and controller teardown coordinator for
// every transport. The caller owns its timeout and can retry a failed save.
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	s.stopping = true
	s.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := s.app.requestShutdown(context.Background(), shutdownRequest{RequestID: newDesktopLifecycleRunID(), Reason: shutdownReasonSystemSignal})
		if err == nil {
			s.mu.Lock()
			if s.cancel != nil {
				s.cancel()
			}
			s.mu.Unlock()
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConfigureBuildIdentity must be called once, before creating any service.
// Executable entry points retain their existing main.* linker variables.
func ConfigureBuildIdentity(buildVersion, buildChannel, selfUpdate string) {
	version, channel, macSelfUpdate = buildVersion, buildChannel, selfUpdate
	// Package init runs before an entry point can pass its linked version.
	plugin.SetMCPClientVersion(version)
	plugincatalog.HostVersion = version
}
