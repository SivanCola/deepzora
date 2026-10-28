package main

import "testing"

// A new session's tab starts with the creation default. Once its first user
// turn is saved the open row and topicbar must show the same preview name the
// catalog projects, while a committed title is never replaced.
func TestRefreshAutoSessionTitleAdoptsSavedPreviewOnly(t *testing.T) {
	app, tab, _, _, _ := canonicalWorkspaceOpenFixture(t)

	app.mu.Lock()
	tab.TopicTitle, tab.topicTitleSource = defaultTopicTitle, topicTitleSourceAuto
	app.mu.Unlock()
	app.refreshAutoSessionTitle(tab)
	app.mu.RLock()
	title, source := tab.TopicTitle, tab.topicTitleSource
	app.mu.RUnlock()
	if title != "session-A" || source != topicTitleSourceAuto {
		t.Fatalf("auto title = %q (%s), want the saved first user turn", title, source)
	}

	app.mu.Lock()
	tab.TopicTitle, tab.topicTitleSource = "Kept name", topicTitleSourceManual
	app.mu.Unlock()
	app.refreshAutoSessionTitle(tab)
	app.mu.RLock()
	title = tab.TopicTitle
	app.mu.RUnlock()
	if title != "Kept name" {
		t.Fatalf("manual title replaced by %q", title)
	}
}
