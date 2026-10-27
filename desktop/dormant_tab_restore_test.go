package main

import (
	"testing"
	"time"
)

// The dormant tab is resolved before it owns a runtime, so a concurrent
// activation can publish one while the open is still waiting for the rebuild
// lock. Adopting the runtime the tab owns at that point — not the one read
// before the wait — keeps the open from failing with a spurious
// "tab runtime changed" error.
func TestOpenSessionAdoptsRuntimePublishedWhileWaitingForTheRebuildLock(t *testing.T) {
	app, tab, ctrl, _ := auditCurrentTab(t)
	ref, ok := ctrl.SessionRef()
	if !ok {
		t.Fatal("fixture controller has no session ref")
	}
	// Model the caller that resolved the tab while it was still dormant.
	app.mu.Lock()
	tab.Ctrl = nil
	app.mu.Unlock()

	// Holding turnStartMu parks the open after it takes runtimeRebuildMu, so
	// the publication below is ordered strictly after the resolve it races.
	tab.turnStartMu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := app.resumeCanonicalSessionForTranscript(tab, nil, sessionRoute(ref.SessionID), defaultHistoryPageTurns, false)
		done <- err
	}()
	// The open owns the rebuild lock once TryLock fails; it is then parked on
	// turnStartMu, strictly after the resolve this publication races.
	deadline := time.Now().Add(20 * time.Second)
	for app.runtimeRebuildMu.TryLock() {
		app.runtimeRebuildMu.Unlock()
		if time.Now().After(deadline) {
			tab.turnStartMu.Unlock()
			t.Fatal("the open never reached the rebuild lock")
		}
		time.Sleep(time.Millisecond)
	}
	app.mu.Lock()
	tab.Ctrl = ctrl
	app.mu.Unlock()
	tab.turnStartMu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("open adopted a stale nil runtime: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("open did not finish after the runtime was published")
	}
	if got := app.controllerForTab(tab); got != ctrl {
		t.Fatalf("tab controller after open = %p, want the published runtime %p", got, ctrl)
	}
}
