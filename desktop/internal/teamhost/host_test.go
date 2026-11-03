package teamhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type fakeRuntime struct {
	busy        atomic.Bool
	closed      atomic.Int32
	cancelled   atomic.Int32
	closeFn     func(context.Context) error
	closeIdleFn func(context.Context) (bool, error)
}

func (*fakeRuntime) Deliver(team.Delivery) error { return nil }
func (r *fakeRuntime) Cancel()                   { r.cancelled.Add(1) }
func (r *fakeRuntime) Idle() bool                { return !r.busy.Load() }
func (r *fakeRuntime) Close(ctx context.Context) error {
	if r.closeFn != nil {
		if err := r.closeFn(ctx); err != nil {
			return err
		}
	}
	r.closed.Add(1)
	return nil
}
func (r *fakeRuntime) CloseIdle(ctx context.Context) (bool, error) {
	if r.closeIdleFn != nil {
		return r.closeIdleFn(ctx)
	}
	if !r.Idle() {
		return false, nil
	}
	return true, r.Close(ctx)
}

func newTestHost(t *testing.T, dir string, open func(context.Context, team.SeatRef, *team.Service) (Runtime, error), now func() time.Time) *Host {
	t.Helper()
	h, err := New(Options{Store: teamroster.Store{Dir: dir}, Open: open, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return h
}

func TestOnlyOneDesktopHostOwnsTeamWrites(t *testing.T) {
	dir := t.TempDir()
	open := func(context.Context, team.SeatRef, *team.Service) (Runtime, error) { return &fakeRuntime{}, nil }
	first := newTestHost(t, dir, open, nil)
	second := newTestHost(t, dir, open, nil)
	if err := first.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := second.Claim(t.Context()); !errors.Is(err, teamroster.ErrReadOnly) {
		t.Fatalf("second writer: %v", err)
	}
	if err := second.WithWriter(t.Context(), func(context.Context, *team.Service) error { t.Fatal("read-only host ran a mutation"); return nil }); !errors.Is(err, teamroster.ErrReadOnly) {
		t.Fatal(err)
	}
	if err := first.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := second.Claim(t.Context()); err != nil {
		t.Fatalf("lease not released after shutdown: %v", err)
	}
}

func TestConcurrentOpenCreatesOneSeatRuntime(t *testing.T) {
	var opens atomic.Int32
	runtime := &fakeRuntime{}
	h := newTestHost(t, t.TempDir(), func(context.Context, team.SeatRef, *team.Service) (Runtime, error) { opens.Add(1); return runtime, nil }, nil)
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Go(func() {
			pane, err := h.Ensure(t.Context(), team.SeatRef{Agent: "Ada"})
			if err == nil && pane != runtime {
				err = errors.New("wrong runtime")
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if opens.Load() != 1 {
		t.Fatalf("opened %d duplicate runtimes", opens.Load())
	}
}

func TestEvictionPreservesVisibleBusyAndNewlyAdmittedSeats(t *testing.T) {
	now := time.Now()
	runtimes := map[team.SeatRef]*fakeRuntime{}
	h := newTestHost(t, t.TempDir(), func(_ context.Context, ref team.SeatRef, _ *team.Service) (Runtime, error) {
		rt := &fakeRuntime{}
		runtimes[ref] = rt
		return rt, nil
	}, func() time.Time { return now })
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	for i := range 8 {
		ref := team.SeatRef{Agent: fmt.Sprint(i)}
		if _, err := h.Ensure(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
	}
	visible, busy, raced := team.SeatRef{Agent: "0"}, team.SeatRef{Agent: "1"}, team.SeatRef{Agent: "2"}
	h.SetVisible(visible, true)
	runtimes[busy].busy.Store(true)
	runtimes[raced].closeIdleFn = func(context.Context) (bool, error) { return false, nil }
	if err := h.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if runtimes[visible].closed.Load() != 0 || runtimes[busy].closed.Load() != 0 || runtimes[raced].closed.Load() != 0 {
		t.Fatal("evicted protected runtime")
	}
	closed := 0
	for _, rt := range runtimes {
		closed += int(rt.closed.Load())
	}
	if closed != 1 {
		t.Fatalf("soft-cap eviction closed %d, want one idle runtime", closed)
	}
	now = now.Add(IdleTimeout)
	if err := h.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if runtimes[visible].closed.Load() != 0 || runtimes[busy].closed.Load() != 0 {
		t.Fatal("idle timer discarded live or visible work")
	}
}

func TestShutdownRetainsLeaseUntilLateOpenIsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	entered, closing, allowClose := make(chan struct{}), make(chan struct{}), make(chan struct{})
	rt := &fakeRuntime{closeFn: func(context.Context) error { close(closing); <-allowClose; return nil }}
	dir := t.TempDir()
	h := newTestHost(t, dir, func(ctx context.Context, _ team.SeatRef, _ *team.Service) (Runtime, error) {
		close(entered)
		<-ctx.Done()
		return rt, nil
	}, nil)
	if err := h.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	opened := make(chan error, 1)
	go func() { _, err := h.Ensure(ctx, team.SeatRef{Agent: "Ada"}); opened <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stopped := make(chan error, 1)
	go func() { stopped <- h.Shutdown(ctx) }()
	select {
	case <-closing:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if release, err := (teamroster.Store{Dir: dir}).AcquireLease(); err == nil {
		release()
		t.Fatal("released lease before runtime closed")
	}
	close(allowClose)
	if err := <-opened; err == nil {
		t.Fatal("late open was published during shutdown")
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if rt.closed.Load() != 1 || rt.cancelled.Load() == 0 {
		t.Fatal("late runtime was not cancelled and closed exactly once")
	}
}

func TestFailedRecoveryCannotEnableMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, teamroster.ExchangeFile)
	if err := os.WriteFile(path, []byte("broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h := newTestHost(t, dir, func(context.Context, team.SeatRef, *team.Service) (Runtime, error) { return &fakeRuntime{}, nil }, nil)
	if err := h.Claim(t.Context()); err == nil {
		t.Fatal("corrupt recovery succeeded")
	}
	if _, err := h.Ensure(t.Context(), team.SeatRef{Agent: "Ada"}); !errors.Is(err, teamroster.ErrReadOnly) {
		t.Fatalf("partial recovery opened a seat: %v", err)
	}
	if err := h.WithWriter(t.Context(), func(context.Context, *team.Service) error {
		t.Fatal("partially recovered host mutated state")
		return nil
	}); !errors.Is(err, teamroster.ErrReadOnly) {
		t.Fatal(err)
	}
	if release, err := (teamroster.Store{Dir: dir}).AcquireLease(); err == nil {
		release()
		t.Fatal("partial recovery relinquished authority")
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := h.WithWriter(t.Context(), func(context.Context, *team.Service) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestLongRunningSeatGetsAFullIdleWindowAfterFinishing(t *testing.T) {
	now := time.Now()
	rt := &fakeRuntime{}
	h := newTestHost(t, t.TempDir(), func(context.Context, team.SeatRef, *team.Service) (Runtime, error) { return rt, nil }, func() time.Time { return now })
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Ensure(t.Context(), team.SeatRef{Agent: "Ada"}); err != nil {
		t.Fatal(err)
	}
	rt.busy.Store(true)
	now = now.Add(2 * IdleTimeout)
	if err := h.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	rt.busy.Store(false)
	now = now.Add(time.Second)
	if err := h.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rt.closed.Load() != 0 {
		t.Fatal("long turn consumed the seat's idle retention window")
	}
	now = now.Add(IdleTimeout)
	if err := h.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rt.closed.Load() != 1 {
		t.Fatal("finished seat was never reclaimed")
	}
}

func TestRecoveryPublishesReadyAfterOpeningTheAdmissionFence(t *testing.T) {
	var h *Host
	observedReady := false
	var err error
	h, err = New(Options{Store: teamroster.Store{Dir: t.TempDir()}, Open: func(context.Context, team.SeatRef, *team.Service) (Runtime, error) { return &fakeRuntime{}, nil }, Changed: func() {
		if h != nil && h.Ready() {
			observedReady = true
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown(context.Background())
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !observedReady {
		t.Fatal("UI was notified only while recovered host was still read-only")
	}
}
