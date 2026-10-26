package main

import (
	"testing"
	"time"
)

func TestReapStaleTopicActivityStatus(t *testing.T) {
	isolateDesktopUserDirs(t)
	now := time.Now()
	cases := []struct {
		name       string
		status     string
		age        time.Duration
		wantStatus string
	}{
		{"fresh thinking kept", topicStatusThinking, time.Minute, topicStatusThinking},
		{"fresh streaming kept", topicStatusStreaming, 5 * time.Minute, topicStatusStreaming},
		{"stale thinking reaped", topicStatusThinking, topicActivityStatusTTL + time.Minute, ""},
		{"stale streaming reaped", topicStatusStreaming, topicActivityStatusTTL + time.Minute, ""},
		{"waiting confirmation never reaped", topicStatusWaitingConfirmation, topicActivityStatusTTL + time.Minute, topicStatusWaitingConfirmation},
		{"error status never reaped", topicStatusError, topicActivityStatusTTL + time.Minute, topicStatusError},
		{"zero timestamp never reaped", topicStatusThinking, 0, topicStatusThinking},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp()
			tab := &WorkspaceTab{
				ID:             "tab",
				Scope:          "global",
				TopicID:        "topic_ttl",
				Ready:          true,
				ActivityStatus: tc.status,
				disabledMCP:    map[string]ServerView{},
			}
			if tc.age > 0 {
				app.projectTreeRuntime.setActivityAt(tab.ID, now.Add(-tc.age))
			}
			app.tabs[tab.ID] = tab
			app.reapStaleTopicActivityStatus(now)
			if tab.ActivityStatus != tc.wantStatus {
				t.Fatalf("status after reap = %q, want %q", tab.ActivityStatus, tc.wantStatus)
			}
		})
	}
}
