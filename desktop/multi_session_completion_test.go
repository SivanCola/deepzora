package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"deepzora/internal/agent"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/tool"
)

// A foreground response must settle while another session in the same project
// is still running. Exercise the host admission and shutdown paths as well as
// the real controller's terminal event fan-out.
func TestSameProjectSessionsCompleteAndShutdownIndependently(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()
	root := t.TempDir()
	long := &exactTurnRunner{started: make(chan struct{})}
	makeTab := func(id string, opts control.Options) *WorkspaceTab {
		sink := &tabEventSink{tabID: id}
		opts.WorkspaceRoot, opts.Sink = root, sink
		ctrl := newNamedCurrentRebuildController(t, app, id, opts)
		tab := &WorkspaceTab{ID: id, Scope: "project", WorkspaceRoot: root,
			TopicID: "topic-" + id, Ctrl: ctrl, Ready: true, sink: sink, SessionGeneration: 1}
		setCurrentRebuildTabIdentity(t, app, tab, ctrl)
		app.mu.Lock()
		app.tabs[id] = tab
		app.tabOrder = append(app.tabOrder, id)
		app.activeTabID = id
		app.mu.Unlock()
		sink.setBinding(id, app, 1)
		return tab
	}
	first := makeTab("long-session", control.Options{Runner: long})
	if _, err := app.StartTurnForTab(first.ID, "keep working", "long-submission"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-long.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first session did not start")
	}
	// Switching the single conversation surface leaves its previous owner in
	// the detached registry while a new session is created in the same project.
	if !app.detachSessionRuntime(first) {
		t.Fatal("could not detach the running session")
	}
	app.mu.Lock()
	delete(app.tabs, first.ID)
	app.tabOrder = nil
	app.mu.Unlock()
	executor := agent.New(workspaceContextProvider{}, tool.NewRegistry(), agent.NewSession("system"), agent.Options{}, event.Discard)
	second := makeTab("short-session", control.Options{Runner: executor, Executor: executor})
	ref, _ := second.Ctrl.(*control.Controller).SessionRef()
	for i := range 3 {
		input, err := app.GetSessionComposerState(ref)
		if err != nil {
			t.Fatal(err)
		}
		input, err = app.SaveSessionComposerState(SessionComposerSaveRequest{Ref: ref, ExpectedRevision: input.Revision, ContentJSON: `{"text":"你是谁"}`, ContentVersion: 1, AcknowledgeHistory: true})
		if err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("short-submission-%d", i)
		if _, err := app.BeginSessionComposerSubmission(ref, input.Revision, id, `{"kind":"turn","display":"你是谁","submit":"你是谁"}`); err != nil {
			t.Fatal(err)
		}
		if _, err := app.StartTurnForTab(second.ID, "你是谁", id); err != nil {
			t.Fatal(err)
		}
		if _, err := app.CompleteSessionComposerSubmission(ref, id, "accepted"); err != nil {
			t.Fatal(err)
		}
		ctrl := second.Ctrl.(*control.Controller)
		if done, busy := ctrl.TurnIdleDone(); busy {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("short session stayed in its finishing boundary")
			}
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			projection := app.GetRuntimeStateSnapshot()
			idle := false
			for _, entry := range projection.Sessions {
				if entry.TabID == second.ID && entry.State.Phase == "idle" {
					idle = true
				}
				if entry.TabID == first.ID && !entry.State.Running {
					t.Fatalf("short session interrupted its sibling: %+v", entry.State)
				}
			}
			if idle {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("short session did not publish idle: %+v", projection)
			}
			time.Sleep(time.Millisecond)
		}
	}
	finished := make(chan error, 1)
	go func() {
		status, err := app.requestShutdown(context.Background(), shutdownRequest{RequestID: "multi-session-quit", Reason: shutdownReasonUserQuit})
		if err == nil && !status.Completed {
			err = fmt.Errorf("shutdown incomplete: %+v", status)
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("shutdown blocked: %+v", app.shutdownStatus(""))
	}
}
