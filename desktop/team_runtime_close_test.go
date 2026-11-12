package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"deepzora/desktop/internal/teamhost"
	"deepzora/internal/control"
	"deepzora/internal/team"
)

func TestTeamRuntimeCloseReleasesGatesRetainsOwnerAndJoinsRetry(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	entered, release := make(chan bool, 1), make(chan struct{})
	var calls atomic.Int32
	ctrl := newNamedCurrentRebuildController(t, a, "team-close-boundary", control.Options{Cleanup: func() {
		calls.Add(1)
		rebuild := a.runtimeRebuildMu.TryLock()
		if rebuild {
			a.runtimeRebuildMu.Unlock()
		}
		admission := a.runtimeAdmissionMu.TryLock()
		if admission {
			a.runtimeAdmissionMu.Unlock()
		}
		entered <- rebuild && admission
		<-release
	}})
	ref, _ := ctrl.SessionRef()
	tab := &WorkspaceTab{ID: "team-close-boundary", Ctrl: ctrl, Ready: true, SessionID: ref.SessionID}
	a.tabs[tab.ID] = tab
	r := &desktopTeamRuntime{app: a, sessionID: ref.SessionID}
	h, err := teamhost.New(teamhost.Options{Store: desktopTeamStore(), Open: func(context.Context, team.SeatRef, *team.Service) (teamhost.Runtime, error) { return r, nil }})
	if err != nil {
		t.Fatal(err)
	}
	a.teamState.host = h
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Ensure(t.Context(), team.SeatRef{Agent: "closing-member"}); err != nil {
		t.Fatal(err)
	}
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- a.stopTeamHostContext(ctx) }()
	select {
	case free := <-entered:
		if !free {
			t.Error("synchronous cleanup holds a runtime gate")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("close wait: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked Close ignored caller cancellation")
	}
	a.mu.RLock()
	phase := a.sessionRuntimeViewLocked(tab).Phase
	retained := tab.Ctrl == ctrl && a.runtimeForTabLocked(tab) != nil
	a.mu.RUnlock()
	if phase != sessionRuntimeClosing || !retained {
		t.Fatal("unfinished close released its canonical owner")
	}
	if err := a.workspaceRuntimeAdmissionErr(tab, ctrl); err == nil {
		t.Fatal("closing Team runtime admitted a turn")
	}
	if err := a.rebuildSettingTurnLocked("closing test", tab, false, false); err == nil {
		t.Fatal("closing Team runtime allowed a rebuild")
	}
	a.mu.Lock()
	detached := a.detachRuntimeForReplacementLocked(tab)
	a.mu.Unlock()
	if detached {
		t.Fatal("navigation moved an owner still finalizing")
	}
	if err := r.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry did not join original close: %v", err)
	}
	unblock()
	if err := a.stopTeamHostContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("retry ran synchronous cleanup twice")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if tab.Ctrl != nil || a.runtimeForTabLocked(tab) != nil {
		t.Fatal("completed close retained its runtime owner")
	}
}

func TestTeamRuntimeCloseShutdownRetriesCompletedCleanupPanic(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	var failedCalls, successfulCalls atomic.Int32
	ctrl := newNamedCurrentRebuildController(t, a, "team-panic", control.Options{Cleanup: func() {
		if failedCalls.Add(1) == 1 {
			panic("one-time cleanup failure")
		}
	}})
	ref, _ := ctrl.SessionRef()
	tab := &WorkspaceTab{ID: "team-panic", Ctrl: ctrl, Ready: true, SessionID: ref.SessionID}
	a.tabs[tab.ID] = tab
	successful := newNamedCurrentRebuildController(t, a, "team-success", control.Options{Cleanup: func() { successfulCalls.Add(1) }})
	successRef, _ := successful.SessionRef()
	successTab := &WorkspaceTab{ID: "team-success", Ctrl: successful, Ready: true, SessionID: successRef.SessionID}
	a.tabs[successTab.ID] = successTab
	r := &desktopTeamRuntime{app: a, sessionID: ref.SessionID}
	h, err := teamhost.New(teamhost.Options{Store: desktopTeamStore(), Open: func(_ context.Context, seat team.SeatRef, _ *team.Service) (teamhost.Runtime, error) {
		if seat.Agent == "success" {
			return &desktopTeamRuntime{app: a, sessionID: successRef.SessionID}, nil
		}
		return r, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	a.teamState.host = h
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"panic", "success"} {
		if _, err := h.Ensure(t.Context(), team.SeatRef{Agent: agent}); err != nil {
			t.Fatal(err)
		}
	}
	request := shutdownRequest{RequestID: "team-cleanup-panic", Reason: shutdownReasonUserQuit}
	status, err := a.requestShutdown(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), "cleanup panic") || !status.Retryable || status.Completed {
		t.Fatalf("first shutdown: %+v, %v", status, err)
	}
	select {
	case <-ctrl.Closed():
	default:
		t.Fatal("failed synchronous cleanup did not finalize controller")
	}
	if lease, err := ctrl.SessionTemp().Acquire(); err == nil {
		lease.Release()
		t.Fatal("cleanup panic retained the controller's temporary-directory owner")
	}
	if tab.Ctrl != ctrl || a.runtimeForTabLocked(tab) == nil {
		t.Fatal("failed Team close dropped its runtime owner")
	}
	if release, err := desktopTeamStore().AcquireLease(); err == nil {
		release()
		t.Fatal("failed Team close released the writer lease")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled retry: %v", err)
	}
	status, err = a.requestShutdown(t.Context(), request)
	if err != nil || !status.Completed {
		t.Fatalf("retry could not finish shutdown: %+v, %v", status, err)
	}
	if tab.Ctrl != nil || a.runtimeForTabLocked(tab) != nil || successTab.Ctrl != nil {
		t.Fatal("successful retry retained a Team runtime")
	}
	if failedCalls.Load() != 1 || successfulCalls.Load() != 1 {
		t.Fatal("retry reran completed controller cleanup")
	}
	if release, err := desktopTeamStore().AcquireLease(); err != nil {
		t.Fatalf("successful retry retained the writer lease: %v", err)
	} else {
		release()
	}
}
