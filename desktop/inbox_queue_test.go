package main

import (
	"testing"

	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/sessioninbox"
)

func TestTargetGuidanceQueuesEndedTurnAndRejectsReplacementSession(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	ctrl := newCurrentRebuildController(t, a, control.Options{Sink: event.Discard})
	ref, _ := ctrl.SessionRef()
	path := sessionRoute(ref.SessionID)
	cleanupExactTurnController(t, ctrl)
	if err := ctrl.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	tab := &WorkspaceTab{ID: "tab", Ctrl: ctrl, SessionID: ref.SessionID, SessionGeneration: 1, Ready: true}
	a.tabs = map[string]*WorkspaceTab{"tab": tab}
	target, err := a.CaptureInboxTarget("tab", path)
	if err != nil {
		t.Fatal(err)
	}
	request := control.InboxQueueRequest{Kind: "enqueue_steer", TurnID: "ended-turn", Text: "preserve guidance", Display: "preserve guidance", IdempotencyKey: "one-guidance"}
	result, err := a.InboxQueueForTarget(target, request)
	if err != nil || result.Receipt == nil || result.Receipt.Disposition != sessioninbox.DispositionQueuedFollowup {
		t.Fatalf("ended turn: %+v %v", result, err)
	}
	duplicate, err := a.InboxQueueForTarget(target, request)
	if err != nil || duplicate.Receipt == nil || duplicate.Receipt.ItemID != result.Receipt.ItemID || len(ctrl.InboxSnapshot().Items) != 1 {
		t.Fatalf("duplicate: %+v %v", duplicate, err)
	}
	confirmed, err := a.LookupInboxFollowupForTarget(target, request.IdempotencyKey)
	if err != nil || confirmed.ItemID != result.Receipt.ItemID {
		t.Fatalf("receipt recovery: %+v %v", confirmed, err)
	}
	tab.SessionGeneration++
	request.IdempotencyKey = "stale-input"
	stale, err := a.InboxQueueForTarget(target, request)
	if err != nil || stale.Reason != "session_changed" || len(ctrl.InboxSnapshot().Items) != 1 {
		t.Fatalf("replacement fence: %+v %v", stale, err)
	}
}
