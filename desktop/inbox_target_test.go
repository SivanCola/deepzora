package main

import (
	"context"
	"testing"

	"deepzora/internal/agent"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/session"
	"deepzora/internal/tool"
)

func TestCanonicalInboxTargetEnqueuesAndConfirmsReceipt(t *testing.T) {
	isolateDesktopUserDirs(t)
	service, err := session.NewService("local", session.NewFilesystemPersistence(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	executor := agent.New(nil, tool.NewRegistry(), agent.NewSession("system"), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{Executor: executor, SessionService: service, Sink: event.Discard})
	t.Cleanup(func() {
		ctrl.Close()
		<-ctrl.Closed()
		_ = service.CloseAll(context.Background())
	})
	if _, err := ctrl.BindFreshSession(t.Context(), "queue-target"); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	const path = "session-id:queue-target"
	a := &App{tabs: map[string]*WorkspaceTab{"tab": {ID: "tab", Ctrl: ctrl, SessionID: "queue-target", SessionGeneration: 1, Ready: true}}}
	target, err := a.CaptureInboxTarget("tab", path)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := a.EnqueueInboxFollowupForTarget(target, "queued input", "queued input", nil, "request")
	if err != nil || receipt.ItemID == "" {
		t.Fatalf("canonical enqueue: %+v %v", receipt, err)
	}
	queue, err := a.InboxQueueForTarget(target, control.InboxQueueRequest{Kind: "enqueue_steer", TurnID: "finished-turn", Text: "guidance", Display: "guidance", IdempotencyKey: "guidance-request"})
	if err != nil || queue.Receipt == nil || queue.Receipt.ItemID == "" {
		t.Fatalf("canonical guidance: %+v %v", queue, err)
	}
	confirmed, err := a.LookupInboxFollowupForTarget(target, "request")
	if err != nil || confirmed.ItemID != receipt.ItemID {
		t.Fatalf("canonical confirmation: %+v %v", confirmed, err)
	}
	a.tabs["tab"].SessionGeneration++
	if _, err := a.EnqueueInboxFollowupForTarget(target, "stale input", "stale input", nil, "stale"); err == nil {
		t.Fatal("stale generation accepted a new write")
	}
	if confirmed, err := a.LookupInboxFollowupForTarget(target, "request"); err != nil || confirmed.ItemID != receipt.ItemID {
		t.Fatalf("same-session receipt after rebind: %+v %v", confirmed, err)
	}
}

func TestLocalInboxTargetRejectsReplacementAndPreservesReceipt(t *testing.T) {
	a, ctrl, path := currentInboxTestApp(t, control.Options{Sink: event.Discard})
	ref, _ := ctrl.SessionRef()
	defer ctrl.Close()
	if err := ctrl.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	a.tabs = map[string]*WorkspaceTab{"tab": {ID: "tab", Ctrl: ctrl, SessionID: ref.SessionID, SessionGeneration: 1, Ready: true}}
	target, err := a.CaptureInboxTarget("tab", path)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := a.EnqueueInboxFollowupForTarget(target, "input", "input", nil, "local-request")
	if err != nil || receipt.ItemID == "" {
		t.Fatalf("enqueue = %+v, %v", receipt, err)
	}
	confirmed, err := a.LookupInboxFollowupForTarget(target, "local-request")
	if err != nil || confirmed.ItemID != receipt.ItemID {
		t.Fatalf("lookup = %+v, %v", confirmed, err)
	}
	if _, err := a.LookupInboxFollowupForTarget(target, "unknown"); err == nil {
		t.Fatal("missing receipt reported as confirmed")
	}
	a.tabs["tab"].SessionGeneration++
	if _, err := a.EnqueueInboxFollowupForTarget(target, "replacement", "replacement", nil, "replacement"); err == nil {
		t.Fatal("replacement accepted old request")
	}
	if got, err := a.LookupInboxFollowupForTarget(target, "local-request"); err != nil || got.ItemID != receipt.ItemID {
		t.Fatalf("same-session read could not rebind: %+v %v", got, err)
	}
	if len(ctrl.InboxSnapshot().Items) != 1 {
		t.Fatal("replacement created another item")
	}
}
