package main

import (
	"fmt"
	"sync"
	"testing"

	"deepzora/internal/event"
)

// runtimeProjectionContractViolation mirrors the frontend's acceptance rule
// (frontend/src/lib/runtimeStateReducer.ts validState). A schema-1 state that
// fails it makes the frontend reject the entire projection and mark every
// session in every workspace as disconnected.
func runtimeProjectionContractViolation(session RuntimeSessionState) string {
	state := session.State
	if state.SchemaVersion != 1 {
		return ""
	}
	switch state.Phase {
	case "idle", "executing", "finishing", "cancelling", "recovery_required", "closed":
	default:
		return fmt.Sprintf("tab %s: phase %q", session.TabID, state.Phase)
	}
	if state.RuntimeEpoch == "" || state.Revision == 0 {
		return fmt.Sprintf("tab %s: schema-1 state without runtime epoch/revision (epoch=%q revision=%d)", session.TabID, state.RuntimeEpoch, state.Revision)
	}
	return ""
}

func TestRuntimeProjectionReportsControllerlessTabAsUnversioned(t *testing.T) {
	building := &WorkspaceTab{ID: "building", Scope: "project", WorkspaceRoot: "/fixture/new-project", TopicID: "new-topic"}
	running := &WorkspaceTab{ID: "running", Scope: "global", TopicID: "topic", Ctrl: &bindingRuntimeReader{state: event.RuntimeStateSnapshot{
		SchemaVersion: 1, RuntimeEpoch: "epoch", Revision: 3, Phase: "executing", Running: true}}}
	app := &App{tabs: map[string]*WorkspaceTab{building.ID: building, running.ID: running}, detachedSessions: map[string]*WorkspaceTab{}}

	projection := app.GetRuntimeStateSnapshot()
	if len(projection.Sessions) != 2 {
		t.Fatalf("sessions=%+v", projection.Sessions)
	}
	for _, session := range projection.Sessions {
		if violation := runtimeProjectionContractViolation(session); violation != "" {
			t.Fatal(violation)
		}
		if session.TabID == building.ID {
			if session.State.SchemaVersion != 0 || session.State.Running || session.State.ActiveWork() {
				t.Fatalf("controllerless tab claimed a committed runtime: %+v", session.State)
			}
			if session.State.Todos == nil || session.State.Interactions == nil {
				t.Fatal("controllerless state must keep the empty wire shape")
			}
		}
	}
}

// Issue #126: creating sessions in a newly opened project publishes the tab
// before its controller. Every projection sampled in that window must remain
// acceptable to the frontend, through the real creation path.
func TestManualCreationAcrossWorkspacesKeepsRuntimeProjectionAcceptable(t *testing.T) {
	a := newManualSessionTestApp(t)
	project := t.TempDir()
	stop := make(chan struct{})
	var (
		mu         sync.Mutex
		violations []string
		samples    int
	)
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		for {
			select {
			case <-stop:
				return
			default:
			}
			projection := a.GetRuntimeStateSnapshot()
			mu.Lock()
			samples++
			for _, session := range projection.Sessions {
				if violation := runtimeProjectionContractViolation(session); violation != "" && len(violations) < 5 {
					violations = append(violations, violation)
				}
			}
			mu.Unlock()
		}
	}()
	for _, request := range []ManualSessionCreationRequest{
		{OperationID: "issue-126-global", Scope: "global"},
		{OperationID: "issue-126-project", Scope: "project", WorkspaceRoot: project},
	} {
		if _, err := a.BeginManualSessionCreation(request); err != nil {
			t.Fatal(err)
		}
		a.manualCreationTasks.Wait()
		if view, err := a.GetManualSessionCreation(request.OperationID); err != nil || view.Phase != "ready" {
			t.Fatalf("%s: %+v %v", request.OperationID, view, err)
		}
	}
	close(stop)
	<-sampled
	mu.Lock()
	defer mu.Unlock()
	if samples == 0 {
		t.Fatal("no projection was sampled")
	}
	if len(violations) > 0 {
		t.Fatalf("frontend would reject the runtime projection: %v", violations)
	}
	for _, session := range a.GetRuntimeStateSnapshot().Sessions {
		if session.State.SchemaVersion != 1 {
			t.Fatalf("ready session %s has no committed runtime state", session.TabID)
		}
	}
}
