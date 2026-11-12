package service

import (
	"context"
	"errors"
	"testing"

	"deepzora/internal/control"
)

func TestStoppedDeferredRebuildDoesNotResumeCapturedRequest(t *testing.T) {
	for name, reason := range map[string]deferredRebuildReason{"settings": deferredSettingsReason, "reload": deferredReloadReason, "startup": deferredStartupReason} {
		t.Run(name, func(t *testing.T) {
			isolateDesktopUserDirs(t)
			a := NewApp()
			a.ctx = context.Background()
			a.readyHook = func() {}
			snapshots := 0
			ctrl := &shutdownSnapshotController{SessionAPI: newFixtureController(t, control.Options{}), shutdown: func() error {
				snapshots++
				return errors.New("unexpected rebuild snapshot")
			}}
			tab := &WorkspaceTab{ID: "deferred-close", Scope: "global", Ready: true, Ctrl: ctrl, buildGeneration: 1}
			if reason == deferredStartupReason {
				tab.Ctrl, tab.Ready, tab.StartupErrLeaseHeld = nil, false, true
			}
			a.tabs[tab.ID] = tab
			request := deferredRebuildRequest{reason: reason, target: tab, label: "test-setting", sequence: 1}
			// Model a request copied out of pending before Stop took effect.
			a.stopDeferredRebuildRetry()
			a.retryDeferredRebuild(tab.ID, request)
			if snapshots != 0 || tab.buildGeneration != 1 {
				t.Fatalf("stopped worker resumed: snapshots=%d buildGeneration=%d", snapshots, tab.buildGeneration)
			}
		})
	}
}

func TestShutdownRejectsNewControllerRebuildsAndPublication(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	a.ctx = context.Background()
	tab := &WorkspaceTab{ID: "shutdown-rebuild", StartupErrLeaseHeld: true, buildGeneration: 7}
	a.tabs[tab.ID] = tab
	a.shuttingDown.Store(true)
	if err := a.rebuildSettingTurnLockedWithModel("model", tab, "", false, false); err == nil {
		t.Fatal("shutdown allowed settings to rebuild a controller")
	}
	if err := a.rebuildStartupTabLocked(tab); err == nil || tab.buildGeneration != 7 {
		t.Fatal("shutdown allowed a startup retry to revive a controller")
	}
	unlock, accepted := a.lockTabControllerPublication(a.currentExtensionGeneration(), "global", "")
	if unlock != nil {
		unlock()
	}
	if accepted {
		t.Fatal("shutdown accepted an already-built controller")
	}
}
