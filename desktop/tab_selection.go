package main

import (
	"fmt"

	"deepzora/internal/control"
)

// activeOrSingleLocalTab resolves the workspace tab a local command should
// target: the active tab when there is one, otherwise the first dormant tab. Both resolve through tabAndCtrlByID so a tab
// whose startup was blocked by a session lease gets the same recovery attempt
// every other tab-scoped command performs before reporting a missing runtime.
func (a *App) activeOrSingleLocalTab() (*WorkspaceTab, control.SessionAPI) {
	if tab, ctrl := a.tabAndCtrlByID(""); tab != nil {
		return tab, ctrl
	}
	return a.firstResolvableLocalTab()
}

// firstResolvableLocalTab returns the first visible tab that resolves through
// tabAndCtrlByID, so a startup blocked by a session lease is retried here too.
func (a *App) firstResolvableLocalTab() (*WorkspaceTab, control.SessionAPI) {
	a.mu.RLock()
	ordered, _ := a.orderedTabIDsSnapshotLocked()
	a.mu.RUnlock()
	for _, id := range ordered {
		if tab, ctrl := a.tabAndCtrlByID(id); tab != nil {
			return tab, ctrl
		}
	}
	return nil, nil
}

// SetActiveTab switches the frontend's active tab.
func (a *App) SetActiveTab(tabID string) error {
	// Even selecting the already-visible tab cancels a slower source adoption.
	a.desktopSessions.navigationSeq.Add(1)
	a.tabSelectionMu.Lock()
	defer a.tabSelectionMu.Unlock()

	a.mu.RLock()
	_, ok := a.tabs[tabID]
	alreadyActive := a.activeTabID == tabID
	a.mu.RUnlock()
	if !ok {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if alreadyActive {
		a.queueCurrentTabLayout()
		return nil
	}
	a.mu.RLock()
	active := a.tabs[a.activeTabID]
	a.mu.RUnlock()
	if err := a.snapshotTabForAction(active, "switching tabs"); err != nil {
		return err
	}

	a.mu.Lock()
	if _, ok := a.tabs[tabID]; !ok {
		a.mu.Unlock()
		return fmt.Errorf("tab %q not found", tabID)
	}
	if a.activeTabID == tabID {
		a.mu.Unlock()
		a.queueCurrentTabLayout()
		return nil
	}
	a.activeTabID = tabID
	next := a.tabs[tabID]
	// A tab restored without a runtime builds one on first activation.
	dormant := next != nil && next.Ctrl == nil
	// A direct click supersedes pending publication without cancelling its
	// build: the tab stays open, and selecting that same tab keeps it alive.
	supersededReq, supersededTab := a.supersedePendingTopicActivationLocked(tabID, false)
	dir, entries, activeID, version := a.saveTabsCollectLocked()
	a.mu.Unlock()

	// I/O outside the lock — disk writes can block for hundreds of ms on
	// Windows when antivirus or the search indexer briefly locks the file.
	a.queueTabLayoutSave(dir, entries, activeID, version)
	if dormant {
		a.startTabControllerBuild(next)
	}
	if active != nil {
		active.clearRuntimeDisplayCurrency()
	}
	if next != nil {
		next.clearRuntimeDisplayCurrency()
	}
	if supersededReq != "" {
		a.emitTopicActivation(TopicActivationEvent{RequestID: supersededReq, TabID: supersededTab, Phase: topicActivationPhaseCancelled})
	}
	a.kickDeferredRebuildRetry()
	return nil
}
