package teamhost

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func TestShutdownBoundsWaitStartsEveryCloseAndRetainsLeaseAcrossRetry(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var calls atomic.Int32
	dir := t.TempDir()
	h := newTestHost(t, dir, func(context.Context, team.SeatRef, *team.Service) (Runtime, error) {
		return &fakeRuntime{closeFn: func(context.Context) error { calls.Add(1); started <- struct{}{}; <-release; return nil }}, nil
	}, nil)
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"Ada", "Ben"} {
		if _, err := h.Ensure(t.Context(), team.SeatRef{Agent: id}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.Shutdown(ctx) }()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("one slow Close prevented another runtime from closing")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown ignored caller cancellation")
	}
	if err := h.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatal("retry started duplicate cleanup")
	}
	if unlock, err := (teamroster.Store{Dir: dir}).AcquireLease(); err == nil {
		unlock()
		t.Fatal("lease released before runtime cleanup")
	}
	if err := h.Claim(t.Context()); err == nil {
		t.Fatal("shutdown permitted new writer admission")
	}
	unblock()
	if err := h.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	unlock, err := (teamroster.Store{Dir: dir}).AcquireLease()
	if err != nil {
		t.Fatalf("successful retry did not release lease: %v", err)
	}
	unlock()
}
