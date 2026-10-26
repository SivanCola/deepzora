package main

import (
	"fmt"

	"deepzora/internal/control"
)

type localReceiptOwner struct {
	tab        *WorkspaceTab
	ctrl       control.SessionAPI
	path       string
	generation uint64
}

func (a *App) localReceiptTarget(path string) (localReceiptOwner, error) {
	key := sessionRuntimeKey(path)
	if key == "" {
		return localReceiptOwner{}, fmt.Errorf("inbox receipt session unavailable")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	var owner localReceiptOwner
	for _, tab := range a.runtimeTabsLocked() {
		if tab.Ctrl == nil || tab.ReadOnly || sessionRuntimeKey(inboxTabIdentity(tab)) != key {
			continue
		}
		if owner.ctrl != nil && owner.ctrl != tab.Ctrl {
			return localReceiptOwner{}, fmt.Errorf("inbox receipt owner is ambiguous")
		}
		owner = localReceiptOwner{tab: tab, ctrl: tab.Ctrl, path: inboxTabIdentity(tab), generation: tab.SessionGeneration}
	}
	if owner.ctrl == nil {
		return owner, fmt.Errorf("inbox receipt session unavailable")
	}
	return owner, nil
}

func (a *App) localReceiptOwnerCurrent(owner localReceiptOwner) bool {
	path := owner.ctrl.SessionRoute()
	if lifecycle, ok := owner.ctrl.(control.IdentityLifecycle); ok && lifecycle.UsesExclusiveSession() {
		ref, bound := lifecycle.SessionRef()
		if !bound {
			return false
		}
		path = "session-id:" + ref.SessionID
	}
	if sessionRuntimeKey(path) != sessionRuntimeKey(owner.path) {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.runtimeTabsLocked() {
		if tab == owner.tab && tab.Ctrl == owner.ctrl && !tab.ReadOnly && tab.SessionGeneration == owner.generation &&
			sessionRuntimeKey(inboxTabIdentity(tab)) == sessionRuntimeKey(owner.path) {
			return true
		}
	}
	return false
}
