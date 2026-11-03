// Package teamhost owns the local Desktop team's lease and background seats.
// Runtime implementations adapt the existing Desktop session service; this
// package does not create another controller or persistence engine.
package teamhost

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

const (
	IdleTimeout     = 10 * time.Minute
	BackgroundLimit = 6
)

type Runtime interface {
	team.Pane
	Idle() bool
	Close(context.Context) error
	// CloseIdle rechecks admission atomically at the existing runtime owner.
	CloseIdle(context.Context) (bool, error)
}

type Options struct {
	Store teamroster.Store
	// Open's context bounds admission only. The existing Desktop owner supplies
	// the returned runtime's lifetime, independently of this operation context.
	Open                      func(context.Context, team.SeatRef, *team.Service) (Runtime, error)
	Changed                   func()
	Now                       func() time.Time
	Validate                  func(team.SeatRef) error
	Binding                   func(team.SeatRef) (string, error)
	AgentConfigurationChanged func(context.Context, *teamroster.Roster, teamroster.Agent, teamroster.Agent) error
}

type entry struct {
	runtime Runtime
	used    time.Time
	visible bool
	binding string
}

type Host struct {
	opts    Options
	mu      sync.Mutex
	seats   map[team.SeatRef]*entry
	visible map[team.SeatRef]bool
	locks   sync.Map
	ops     sync.WaitGroup
	service *team.Service
	release func()
	closing bool
	ready   bool
	ctx     context.Context
	cancel  context.CancelFunc
	claimMu sync.Mutex
}

func New(opts Options) (*Host, error) {
	if opts.Store.Dir == "" || opts.Open == nil {
		return nil, errors.New("team host requires a store and session runtime adapter")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Host{opts: opts, seats: map[team.SeatRef]*entry{}, visible: map[team.SeatRef]bool{}, ctx: ctx, cancel: cancel}, nil
}

func (h *Host) operationContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(h.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}

// Claim recovers interrupted work before allowing ordinary host commands.
// Another live host leaves this host read-only; a later tick can claim it.
func (h *Host) Claim(ctx context.Context) error {
	ctx, done := h.operationContext(ctx)
	defer done()
	h.claimMu.Lock()
	defer h.claimMu.Unlock()
	h.mu.Lock()
	if h.closing {
		h.mu.Unlock()
		return errors.New("team host is closing")
	}
	if h.service != nil {
		svc, ready := h.service, h.ready
		h.mu.Unlock()
		if ready {
			return nil
		}
		return h.recover(ctx, svc)
	}
	h.mu.Unlock()
	release, err := h.opts.Store.AcquireLease()
	if err != nil {
		return err
	}
	svc := team.New(team.Options{Store: h.opts.Store, Panes: h, Now: h.opts.Now, OnChange: h.opts.Changed, AgentConfigurationChanged: h.opts.AgentConfigurationChanged})
	h.mu.Lock()
	h.service, h.release = svc, release
	h.mu.Unlock()
	return h.recover(ctx, svc)
}

type recoveryContextKey struct{}

func (h *Host) recover(ctx context.Context, svc *team.Service) error {
	if err := svc.Recover(context.WithValue(ctx, recoveryContextKey{}, h)); err != nil {
		// Keep the lease: partial recovery must not enable writes or hand the
		// state to another writer while recovered runtimes may still exist.
		return fmt.Errorf("recover team: %w", err)
	}
	h.mu.Lock()
	h.ready = true
	h.mu.Unlock()
	if h.opts.Changed != nil {
		h.opts.Changed()
	}
	return nil
}

// WithWriter holds shutdown admission while a trusted Desktop command runs.
// The callback must not expose the Service to a renderer or model directly.
func (h *Host) WithWriter(ctx context.Context, fn func(context.Context, *team.Service) error) error {
	ctx, done := h.operationContext(ctx)
	defer done()
	h.claimMu.Lock()
	h.mu.Lock()
	if h.closing || h.service == nil || !h.ready {
		h.mu.Unlock()
		h.claimMu.Unlock()
		return teamroster.ErrReadOnly
	}
	svc := h.service
	h.ops.Add(1)
	h.mu.Unlock()
	h.claimMu.Unlock()
	defer h.ops.Done()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(ctx, svc)
}

func (h *Host) seatLock(ref team.SeatRef) *sync.Mutex {
	lock, _ := h.locks.LoadOrStore(ref, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (h *Host) Ensure(ctx context.Context, ref team.SeatRef) (team.Pane, error) {
	ctx, done := h.operationContext(ctx)
	defer done()
	lock := h.seatLock(ref)
	lock.Lock()
	defer lock.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if h.opts.Validate != nil {
		if err := h.opts.Validate(ref); err != nil {
			return nil, err
		}
	}
	binding := ""
	if h.opts.Binding != nil {
		var err error
		binding, err = h.opts.Binding(ref)
		if err != nil {
			return nil, err
		}
	}
	h.mu.Lock()
	if h.closing || h.service == nil || (!h.ready && ctx.Value(recoveryContextKey{}) != h) {
		h.mu.Unlock()
		return nil, teamroster.ErrReadOnly
	}
	previous := h.seats[ref]
	if e := previous; e != nil && (h.opts.Binding == nil || e.binding == binding) {
		e.used = h.opts.Now()
		rt := e.runtime
		h.mu.Unlock()
		return rt, nil
	}
	svc := h.service
	h.ops.Add(1)
	h.mu.Unlock()
	defer h.ops.Done()
	if previous != nil {
		closer, ok := previous.runtime.(interface {
			CloseRebound(context.Context) (bool, error)
		})
		if !ok {
			return nil, errors.New("team runtime cannot replace a changed binding")
		}
		closed, err := closer.CloseRebound(ctx)
		if err != nil {
			return nil, err
		}
		if !closed {
			return nil, errors.New("team runtime is busy while its binding changed")
		}
		h.mu.Lock()
		delete(h.seats, ref)
		h.mu.Unlock()
	}
	rt, err := h.opts.Open(ctx, ref, svc)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, errors.New("team runtime adapter returned no runtime")
	}
	if owner, ok := rt.(interface{ BindingKey() string }); ok {
		binding = owner.BindingKey()
	}
	h.mu.Lock()
	if h.closing {
		h.seats[ref] = &entry{runtime: rt, used: h.opts.Now()}
		h.mu.Unlock()
		return nil, errors.New("team host closed during seat creation")
	}
	h.seats[ref] = &entry{runtime: rt, used: h.opts.Now(), visible: h.visible[ref], binding: binding}
	h.mu.Unlock()
	return rt, nil
}

// Ready is the admission fence an adapter checks before autonomous inbox work.
func (h *Host) Ready() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ready && !h.closing
}

// BindingService is for trusted runtime assembly, including recovery. It does
// not confer write admission; commands still go through WithWriter.
func (h *Host) BindingService() *team.Service {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.service
}

func (h *Host) Live(ref team.SeatRef) (team.Pane, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.seats[ref]
	if e == nil {
		return nil, false
	}
	return e.runtime, true
}

// SetVisible changes only the display reference. Closing a panel does not
// cancel work, discard pending approvals or close a session's durable writer.
func (h *Host) SetVisible(ref team.SeatRef, visible bool) {
	lock := h.seatLock(ref)
	lock.Lock()
	defer lock.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closing {
		return
	}
	h.visible[ref] = visible
	if e := h.seats[ref]; e != nil {
		e.visible = visible
		e.used = h.opts.Now()
	}
}

// Tick is driven by Desktop's owned clock loop. It never creates an OS daemon.
func (h *Host) Tick(ctx context.Context) error {
	if err := h.Claim(ctx); err != nil {
		if errors.Is(err, teamroster.ErrReadOnly) {
			return nil
		}
		return err
	}
	return h.WithWriter(ctx, func(ctx context.Context, s *team.Service) error {
		if err := s.Tick(ctx); err != nil {
			return err
		}
		return h.evict(ctx)
	})
}

func (h *Host) Shutdown(ctx context.Context) error {
	h.cancel()
	h.claimMu.Lock()
	defer h.claimMu.Unlock()
	h.mu.Lock()
	h.closing = true
	h.mu.Unlock()
	h.ops.Wait()
	h.mu.Lock()
	seats := maps.Clone(h.seats)
	h.mu.Unlock()
	for _, e := range seats {
		e.runtime.Cancel()
	}
	var failures error
	for ref, e := range seats {
		if err := e.runtime.Close(ctx); err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		h.mu.Lock()
		delete(h.seats, ref)
		h.mu.Unlock()
	}
	if failures != nil {
		return failures
	}
	h.mu.Lock()
	release := h.release
	h.release, h.service = nil, nil
	h.mu.Unlock()
	if release != nil {
		release()
	}
	return nil
}
