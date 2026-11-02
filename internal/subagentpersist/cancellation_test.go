package subagentpersist

import (
	"context"
	"deepzora/internal/agent"
	"deepzora/internal/event"
	"deepzora/internal/jobs"
	"deepzora/internal/session"
	"io"
	"testing"
)

func TestChildRuntimeCancellationDoesNotCancelParentOrSibling(t *testing.T) {
	store, parent := testStore(t, t.TempDir())
	children, err := parent.Subagents()
	if err != nil {
		t.Fatal(err)
	}
	parentContext := t.Context()
	handles := make([]agent.SubagentTranscript, 2)
	contexts := make([]context.Context, 2)
	refs := []string{"sa_cancel_one", "sa_cancel_two"}
	for i, ref := range refs {
		handles[i], err = store.Create(t.Context(), agent.SubagentMeta{Ref: ref}, nil)
		if err != nil {
			t.Fatal(err)
		}
		run := &agent.SubagentRun{Persistence: handles[i]}
		var release func()
		contexts[i], release = run.CancellableContext(parentContext)
		defer release()
		defer handles[i].Close(context.Background())
	}
	runtime, ok := children.Runtime(session.SessionRef{HostID: children.HostID(), SessionID: refs[0]})
	if !ok || !runtime.Cancel() {
		t.Fatal("child runtime rejected cancellation")
	}
	if contexts[0].Err() != context.Canceled {
		t.Fatal("selected child was not cancelled")
	}
	if contexts[1].Err() != nil || parentContext.Err() != nil {
		t.Fatal("cancellation escaped the selected child")
	}
	if err := handles[0].Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if runtime.Cancel() {
		t.Fatal("closed child acknowledged a stale cancellation")
	}
}

func TestBackgroundChildRuntimeCancellationMarksOnlyItsJobKilled(t *testing.T) {
	store, parent := testStore(t, t.TempDir())
	children, err := parent.Subagents()
	if err != nil {
		t.Fatal(err)
	}
	m := jobs.NewManager(event.Discard)
	defer m.Close()
	ready := make(chan struct{})
	proceed := make(chan struct{})
	defer close(proceed)
	handle, err := store.Create(t.Context(), agent.SubagentMeta{Ref: "sa_background_cancel"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close(context.Background())
	run := &agent.SubagentRun{Ref: "sa_background_cancel", Persistence: handle}
	j := m.StartForSession("parent", "task", "child", func(ctx context.Context, _ io.Writer) (string, error) {
		ctx, release := run.CancellableContext(ctx)
		defer release()
		close(ready)
		<-ctx.Done()
		return "", agent.NewSubagentRunError(run, ctx.Err())
	})
	sibling := m.StartForSession("parent", "task", "sibling", func(ctx context.Context, _ io.Writer) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-proceed:
			return "sibling completed", nil
		}
	})
	<-ready
	runtime, ok := children.Runtime(session.SessionRef{HostID: children.HostID(), SessionID: run.Ref})
	if !ok || !runtime.Cancel() {
		t.Fatal("background child cancellation rejected")
	}
	results := m.WaitForSession(t.Context(), "parent", []string{j.ID}, 5)
	if len(results) != 1 || results[0].Status != jobs.Killed {
		t.Fatalf("cancelled child job: %+v", results)
	}
	result, ok := m.InspectForSession("parent", sibling.ID)
	if !ok || result.Status != jobs.Running {
		t.Fatal("selected child cancellation escaped to sibling job")
	}
	// The manager/parent remains usable for subsequent work.
	next := m.StartForSession("parent", "task", "next", func(context.Context, io.Writer) (string, error) { return "ok", nil })
	results = m.WaitForSession(t.Context(), "parent", []string{next.ID}, 5)
	if len(results) != 1 || results[0].Status != jobs.Done {
		t.Fatalf("parent manager was cancelled: %+v", results)
	}
}
