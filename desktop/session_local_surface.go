package main

// selectLocalSurfaceAfterOpen republishes the tab layout once a bound session
// opens. It serializes with direct tab clicks, so a newer selection is never
// cleared by this completion.
func (a *App) selectLocalSurfaceAfterOpen(navigationSequence uint64) {
	a.tabSelectionMu.Lock()
	defer a.tabSelectionMu.Unlock()
	if a.desktopSessions.navigationSeq.Load() != navigationSequence {
		return
	}
	a.queueCurrentTabLayout()
}
