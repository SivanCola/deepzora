package main

import "strings"

// singleSurfaceTabsFile keeps only the persisted active tab (or the first
// one) for the one-conversation layout.
func singleSurfaceTabsFile(f desktopTabsFile) desktopTabsFile {
	if len(f.Tabs) <= 1 {
		return f
	}
	active := strings.TrimSpace(f.ActiveTab)
	chosen := f.Tabs[0]
	for _, entry := range f.Tabs {
		if entry.ID == active {
			chosen = entry
			break
		}
	}
	return desktopTabsFile{Tabs: []desktopTabEntry{chosen}, ActiveTab: chosen.ID, TabOrder: []string{chosen.ID}, extra: cloneDesktopJSONFields(f.extra)}
}

// finishRestoredLocalTabs activates the persisted active tab and builds the
// restored tabs that carry a runtime.
func (a *App) finishRestoredLocalTabs(f desktopTabsFile, toBuild []*WorkspaceTab) {
	a.mu.Lock()
	if _, ok := a.tabs[f.ActiveTab]; ok {
		a.activeTabID = f.ActiveTab
	} else if ordered := a.orderedTabIDsLocked(); len(ordered) > 0 {
		a.activeTabID = ordered[0]
	}
	a.saveTabsLocked()
	a.mu.Unlock()
	for _, tab := range toBuild {
		a.startTabControllerBuild(tab)
	}
}
