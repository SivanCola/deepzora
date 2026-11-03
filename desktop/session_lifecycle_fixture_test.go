package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"deepzora/internal/control"
	"deepzora/internal/session"
)

// Service shutdown joins its query workers, but cannot join desktop autosaves
// or controller finalization. Stop those producers before closing their store.
func closeLifecycleFixture(t *testing.T, a *App) {
	t.Helper()
	a.shuttingDown.Store(true)
	a.stopDeferredRebuildRetry()
	a.cancelSessionNavigation()
	a.cancelAllTabBuilds()
	if err := a.stopManualCreations(); err != nil {
		t.Error(err)
	}
	if !a.stopSessionCatalog(time.Second) {
		t.Error("session catalog did not stop")
		a.catalogLifecycleMu.Lock()
		closing := a.catalogClosing
		a.catalogLifecycleMu.Unlock()
		<-closing.done
	}
	a.runtimeRebuildMu.Lock()
	a.mu.RLock()
	tabs := a.runtimeTabsLocked()
	var builds []*tabBuildExecution
	for _, tab := range tabs {
		for build := range tab.buildExecutions {
			builds = append(builds, build)
		}
	}
	a.mu.RUnlock()
	a.runtimeRebuildMu.Unlock()
	for _, build := range builds {
		<-build.done
	}
	for _, tab := range tabs {
		a.quiesceTabAutosave(tab)
	}
	var controllers []control.SessionAPI
	for _, tab := range tabs {
		if ctrl := a.controllerForTab(tab); ctrl != nil {
			ctrl.Close()
			controllers = append(controllers, ctrl)
		}
	}
	for _, ctrl := range controllers {
		if closed, ok := ctrl.(interface{ Closed() <-chan struct{} }); ok {
			<-closed.Closed()
		}
	}
	a.flushRuntimeProjections()
	a.flushTabLayoutWrites()
	if a.sessionUI != nil {
		if err := a.sessionUI.Close(); err != nil {
			t.Error(err)
		}
	}
	if err := a.closeSessionServicesResult(); err != nil {
		t.Error(err)
	}
	a.closeAllSharedHosts()
	a.closeSharedSkillWatchService()
}

type lifecycleClosingController struct {
	control.SessionAPI
	closeOnce      sync.Once
	joinOnce       sync.Once
	closeRequested chan struct{}
	joinRequested  chan struct{}
	closed         chan struct{}
}

func (c *lifecycleClosingController) Close() {
	c.closeOnce.Do(func() { close(c.closeRequested) })
}

func (c *lifecycleClosingController) Closed() <-chan struct{} {
	c.joinOnce.Do(func() { close(c.joinRequested) })
	return c.closed
}

func TestLifecycleFixtureJoinsEveryControllerBeforeServiceShutdown(t *testing.T) {
	a, _ := lifecycleFixture(t)
	service := a.desktopSessionService("")
	visible := &lifecycleClosingController{closeRequested: make(chan struct{}), joinRequested: make(chan struct{}), closed: make(chan struct{})}
	detached := &lifecycleClosingController{closeRequested: make(chan struct{}), joinRequested: make(chan struct{}), closed: make(chan struct{})}
	a.tabs["visible"] = &WorkspaceTab{ID: "visible", Ctrl: visible}
	a.detachedSessions = map[string]*WorkspaceTab{"detached": {ID: "detached", Ctrl: detached}}
	var release sync.Once
	unblock := func() { release.Do(func() { close(visible.closed); close(detached.closed) }) }
	defer unblock()
	done := make(chan struct{})
	go func() { closeLifecycleFixture(t, a); close(done) }()
	for _, closed := range []*lifecycleClosingController{visible, detached} {
		select {
		case <-closed.closeRequested:
		case <-time.After(5 * time.Second):
			t.Fatal("fixture did not close every runtime owner")
		}
	}
	select {
	case <-visible.joinRequested:
	case <-detached.joinRequested:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not join controller finalization")
	}
	select {
	case <-done:
		t.Fatal("fixture returned while controllers still own stores")
	default:
	}
	// Controller cleanup may still write until Closed; the shared service must
	// remain available throughout that interval.
	runtime, err := service.Create(context.Background(), session.CreateOptions{SessionID: "finishing-controller", CWD: globalWorkspaceRoot(), Origin: session.SessionOriginNew})
	if err != nil {
		t.Fatalf("service shut down before controller cleanup: %v", err)
	}
	if err := service.Close(context.Background(), runtime.Ref()); err != nil {
		t.Fatal(err)
	}
	unblock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not finish after all controllers released their stores")
	}
}

func TestLifecycleFixtureQuiescesAutosaveBeforeClosingControllers(t *testing.T) {
	a, _ := lifecycleFixture(t)
	ctrl := &lifecycleClosingController{closeRequested: make(chan struct{}), joinRequested: make(chan struct{}), closed: make(chan struct{})}
	close(ctrl.closed)
	tab := &WorkspaceTab{ID: "saving", Ctrl: ctrl, saving: true}
	tab.saveCond = sync.NewCond(&tab.saveMu)
	a.tabs[tab.ID] = tab
	release := func() {
		tab.saveMu.Lock()
		tab.saving = false
		tab.saveCond.Broadcast()
		tab.saveMu.Unlock()
	}
	defer release()
	done := make(chan struct{})
	go func() { closeLifecycleFixture(t, a); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		tab.saveMu.Lock()
		closing := tab.closing
		tab.saveMu.Unlock()
		if closing {
			break
		}
		select {
		case <-done:
			t.Fatal("fixture did not quiesce its autosave owner")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture did not reach the autosave join")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-ctrl.closeRequested:
		t.Fatal("controller closed while its autosave could still write")
	default:
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not finish after autosave released its write")
	}
}
