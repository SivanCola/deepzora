package main

import (
	"context"
	"errors"
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
