package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"deepzora/internal/control"
)

func TestShutdownJoinsControllersOutsideRuntimeGates(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	ctrl := newNamedCurrentRebuildController(t, app, "shutdown-join", control.Options{})
	ref, _ := ctrl.SessionRef()
	gate := &lifecycleClosingController{SessionAPI: ctrl, closeRequested: make(chan struct{}),
		joinRequested: make(chan struct{}), closed: make(chan struct{})}
	tab := &WorkspaceTab{ID: "shutdown-join", Ctrl: gate, Ready: true, SessionID: ref.SessionID}
	app.tabs[tab.ID] = tab
	release := sync.OnceFunc(func() { close(gate.closed) })
	defer release()
	finished := make(chan error, 1)
	go func() {
		_, err := app.requestShutdown(context.Background(), shutdownRequest{RequestID: "join", Reason: shutdownReasonUserQuit})
		finished <- err
	}()
	select {
	case <-gate.joinRequested:
	case err := <-finished:
		t.Fatalf("shutdown closed persistence before joining its controller: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not join controller finalization")
	}
	select {
	case <-gate.closeRequested:
	default:
		t.Fatal("joined before requesting cancellation")
	}
	if _, exists := ctrl.SessionService().Runtime(ref); !exists {
		t.Fatal("service released a controller still finalizing")
	}
	if err := app.workspaceRuntimeAdmissionErr(tab, gate); err == nil {
		t.Fatal("shutdown still admits new turns")
	}
	// A final callback may need the normal host mutation gate. The shutdown
	// join must not own that gate while waiting for the callback to return.
	if !app.runtimeRebuildMu.TryLock() {
		t.Fatal("shutdown retained the rebuild gate during controller finalization")
	}
	app.runtimeRebuildMu.Unlock()
	if !app.runtimeAdmissionMu.TryLock() {
		t.Fatal("shutdown retained the admission gate during controller finalization")
	}
	app.runtimeAdmissionMu.Unlock()
	ctrl.Close()
	<-ctrl.Closed()
	release()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish after controller release")
	}
}

func TestShutdownControllerWaitCanTimeOutAndRetry(t *testing.T) {
	gate := &lifecycleClosingController{closeRequested: make(chan struct{}), joinRequested: make(chan struct{}), closed: make(chan struct{})}
	items := []desktopShutdownItem{{tab: &WorkspaceTab{ID: "stalled"}, ctrl: gate}}
	coordinator := &desktopShutdownCoordinator{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := coordinator.closeControllers(ctx, items); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("wait must identify the unfinished session and retain cancellation: %v", err)
	}
	close(gate.closed)
	if err := coordinator.closeControllers(context.Background(), items); err != nil {
		t.Fatalf("finalization could not be retried: %v", err)
	}
}

func TestShutdownClosesIdleControllerOutsideRuntimeGates(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	blocked := make(chan string, 2)
	ctrl := newNamedCurrentRebuildController(t, a, "idle-close", control.Options{Cleanup: func() {
		if a.runtimeRebuildMu.TryLock() {
			a.runtimeRebuildMu.Unlock()
		} else {
			blocked <- "rebuild"
		}
		if a.runtimeAdmissionMu.TryLock() {
			a.runtimeAdmissionMu.Unlock()
		} else {
			blocked <- "admission"
		}
	}})
	a.tabs["idle-close"] = &WorkspaceTab{ID: "idle-close", Ctrl: ctrl}
	status, err := a.requestShutdown(t.Context(), shutdownRequest{RequestID: "idle-close", Reason: shutdownReasonUserQuit})
	if err != nil || !status.Completed {
		t.Fatalf("shutdown: %+v, %v", status, err)
	}
	select {
	case gate := <-blocked:
		t.Fatalf("synchronous controller cleanup ran under the %s gate", gate)
	default:
	}
}

type blockingShutdownClose struct {
	stubSessionAPI
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (c *blockingShutdownClose) Close() {
	if c.calls.Add(1) == 1 {
		close(c.started)
	}
	<-c.release
}

func TestShutdownBoundsSynchronousCloseAndReusesItOnRetry(t *testing.T) {
	first := &blockingShutdownClose{started: make(chan struct{}), release: make(chan struct{})}
	second := &blockingShutdownClose{started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(first.release); close(second.release) })
	defer release()
	items := []desktopShutdownItem{{tab: &WorkspaceTab{ID: "first"}, ctrl: first}, {tab: &WorkspaceTab{ID: "second"}, ctrl: second}}
	c := &desktopShutdownCoordinator{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- c.closeControllers(ctx, items) }()
	for _, ctrl := range []*blockingShutdownClose{first, second} {
		select {
		case <-ctrl.started:
		case <-time.After(5 * time.Second):
			t.Fatal("one blocked Close prevented another session's cancellation")
		}
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked synchronous close did not respect the wait deadline: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("synchronous Close bypassed the shutdown wait deadline")
	}
	if err := c.closeControllers(ctx, items); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry should still be waiting for the original close: %v", err)
	}
	release()
	if err := c.closeControllers(t.Context(), items); err != nil {
		t.Fatal(err)
	}
	if first.calls.Load() != 1 || second.calls.Load() != 1 {
		t.Fatal("retry started duplicate controller cleanup")
	}
}
