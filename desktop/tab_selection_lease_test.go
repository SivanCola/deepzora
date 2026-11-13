package service

import (
	"context"
	"testing"
)

// A tab whose controller build was blocked by a held session lease keeps
// Ctrl == nil and StartupErrLeaseHeld set. Every tab-scoped command resolves
// through tabAndCtrlByID, which retries that blocked startup before reporting
// a missing runtime; the local-tab resolver used by OpenSession must do the
// same, or opening a session on the only tab fails until the user finds
// another way to trigger a rebuild.
func TestActiveOrSingleLocalTabRecoversLeaseBlockedStartup(t *testing.T) {
	app, ref := lifecycleFixture(t)
	app.readyHook = func() {}

	release, err := app.beginProjectRuntimeAdmission("global", globalTabWorkspaceRoot())
	if err != nil {
		t.Fatal(err)
	}
	tab := app.createTabEntry("global", globalTabWorkspaceRoot(), "")
	tab.sink = &tabEventSink{tabID: tab.ID, app: app}
	installNoopRuntimeEvents(app, tab.sink)
	// The startup that would have built this tab's runtime was refused by a
	// lease the holder has since released.
	tab.StartupErr = (&sessionLeaseBusyError{}).Error()
	tab.StartupErrLeaseHeld = true
	tab.Ready = false
	app.publishRestoredTab(tab, release)
	resolved, ctrl := app.activeOrSingleLocalTab()
	if resolved != tab {
		t.Fatalf("resolved tab = %+v, want the lease-blocked tab", resolved)
	}
	if ctrl == nil {
		t.Fatal("lease-blocked startup was not recovered; the tab stayed without a runtime")
	}
	// Navigation can retire the recovered controller and create a new owner.
	// Seal callbacks first, join the retired owner, then use the complete App
	// shutdown boundary for the replacement and its background services.
	t.Cleanup(func() {
		app.shuttingDown.Store(true)
		app.quiesceTabAutosave(tab)
		ctrl.Close()
		if closed, ok := ctrl.(interface{ Closed() <-chan struct{} }); ok {
			<-closed.Closed()
		}
		if _, err := app.requestShutdown(context.Background(), shutdownRequest{RequestID: "lease-recovery-cleanup", Reason: shutdownReasonUserQuit}); err != nil {
			t.Errorf("shutdown recovered tab: %v", err)
		}
	})
	if _, err := app.OpenSession(ref); err != nil {
		t.Fatalf("open session on the recovered tab: %v", err)
	}
	app.mu.RLock()
	boundSessionID := app.tabs[tab.ID].SessionID
	app.mu.RUnlock()
	if boundSessionID != ref.SessionID {
		t.Fatalf("tab session after open = %q, want %q", boundSessionID, ref.SessionID)
	}
}
