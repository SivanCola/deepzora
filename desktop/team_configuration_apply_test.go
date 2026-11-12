package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"deepzora/internal/agentpreset"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/team"
)

func teamAdmissionFixture(t *testing.T) (*App, *WorkspaceTab, *control.Controller, string, TeamAgentSettings) {
	t.Helper()
	a := newManualSessionTestApp(t)
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "team-admission", Kind: "openai", BaseURL: "http://127.0.0.1:1", Models: []string{"test"}, NoProxy: true}, "disposable-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDefaultModel("team-admission/test"); err != nil {
		t.Fatal(err)
	}
	settings := TeamAgentSettings{Name: "Chief", Main: true, Permission: "read-only", Preset: &TeamPresetDefinition{ID: "copied", Name: "Copied", Complete: true, Body: "ORIGINAL_PERSONA", IncludeRuntimeContext: true}}
	created, err := a.SaveTeamAgent(SaveTeamAgentRequest{RequestID: "create-admission-fixture", Settings: settings})
	if err != nil || created.Outcome != "applied" {
		t.Fatalf("create: %+v %v", created, err)
	}
	h, err := a.localTeamHost()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	pane, err := h.Ensure(t.Context(), team.SeatRef{Agent: created.TargetID})
	if err != nil {
		t.Fatal(err)
	}
	owner := pane.(*desktopTeamRuntime).owner()
	return a, owner, a.controllerForTab(owner).(*control.Controller), created.TargetID, settings
}

func saveTeamAdmissionSettings(t *testing.T, a *App, id string, settings TeamAgentSettings) {
	t.Helper()
	view, err := a.GetTeam()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := a.SaveTeamAgent(SaveTeamAgentRequest{ID: id, ExpectedRevision: view.Roster.Revision, RequestID: "update-admission-fixture", Settings: settings})
	if err != nil || updated.Outcome != "applied" {
		t.Fatalf("save: %+v %v", updated, err)
	}
}

func TestTeamTurnAdmissionRechecksConfigurationAfterConcurrentCommit(t *testing.T) {
	for _, entry := range []string{"human", "inbox"} {
		for _, change := range []string{"preset", "directory"} {
			t.Run(entry+"/"+change, func(t *testing.T) {
				a, owner, old, id, settings := teamAdmissionFixture(t)
				if entry == "human" {
					meta, err := a.RevealBackgroundRuntime(owner.ID)
					if err != nil {
						t.Fatal(err)
					}
					var current control.SessionAPI
					owner, current = a.tabAndCtrlByID(meta.ID)
					old = current.(*control.Controller)
				}
				if change == "directory" {
					settings.Folder = t.TempDir()
				} else {
					if len(old.ToolContractEntries()) <= 1 {
						t.Fatal("fixture must start with tools beyond the fixed capability proxy")
					}
					settings.Preset = &TeamPresetDefinition{ID: "copied", Name: "Copied", Complete: true, Body: "REPLACEMENT_PERSONA", IncludeRuntimeContext: false, RestrictTools: true, Tools: []string{}}
				}
				var committed atomic.Bool
				// The save wins the writer boundary after the sender's optimistic
				// check, before that sender can acquire its final admission lock.
				a.lifecycleCheckpointHook = func(stage string) {
					if stage == "team-turn-before-admission" && committed.CompareAndSwap(false, true) {
						saveTeamAdmissionSettings(t, a, id, settings)
					}
				}
				var admitted control.SessionAPI
				var err error
				if entry == "inbox" {
					var release func()
					release, err = a.beforeInboxDispatch(old)
					if release != nil {
						release()
					}
				} else {
					var admission *tabTurnAdmission
					admission, admitted, err = a.beginTabTurn(owner.ID, false)
					if admission != nil {
						admission.abort()
					}
				}
				if !committed.Load() {
					t.Fatalf("test did not cross the intended configuration boundary: %v", err)
				}
				if change == "directory" {
					if err == nil || !strings.Contains(err.Error(), "working directory changed") {
						t.Fatalf("stale directory binding admitted: %v", err)
					}
					return
				}
				if entry == "inbox" {
					if !errors.Is(err, control.ErrInboxRuntimeUnpublished) {
						t.Fatalf("old inbox owner was not fenced after rebuild: %v", err)
					}
				} else if err != nil || admitted == old {
					t.Fatalf("new turn did not use rebuilt controller: %v", err)
				}
				next := a.controllerForTab(owner).(*control.Controller)
				view, err := a.GetTeam()
				if err != nil || next == old || view.Seats[0].SettingsPending || !strings.Contains(next.SystemPrompt(), "REPLACEMENT_PERSONA") {
					t.Fatalf("latest static configuration not applied: %+v %v", view, err)
				}
				// Empty preset tools retain only the existing fixed-schema proxy;
				// its executor still enforces the empty underlying allowlist.
				if entries := next.ToolContractEntries(); len(entries) != 1 || entries[0].Name != "use_capability" {
					t.Fatal("new admission retained tools removed by the saved preset")
				}
			})
		}
	}
}

type pendingTeamAdmissionController struct{ *control.Controller }

func (c pendingTeamAdmissionController) RuntimeStateSnapshot() event.RuntimeStateSnapshot {
	state := c.Controller.RuntimeStateSnapshot()
	state.PendingPrompt = true
	return state
}

func TestTeamTurnAdmissionRejectsPendingConfigurationWhilePromptIsActive(t *testing.T) {
	a, owner, original, id, settings := teamAdmissionFixture(t)
	settings.Preset.Body = "REPLACEMENT_PERSONA"
	saveTeamAdmissionSettings(t, a, id, settings)
	a.mu.Lock()
	owner.Ctrl = pendingTeamAdmissionController{original}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		owner.Ctrl = original
		a.mu.Unlock()
	}()
	var attempts atomic.Int32
	a.lifecycleCheckpointHook = func(stage string) {
		if stage == "team-turn-before-admission" && attempts.Add(1) > 1 {
			t.Fatal("pending prompt caused an admission retry loop")
		}
	}
	admission, _, err := a.beginRuntimeTurn(owner.ID, false, true)
	if admission != nil {
		admission.abort()
	}
	var busy *rebuildBusyError
	if !errors.As(err, &busy) || !busy.work.pendingPrompt {
		t.Fatalf("pending prompt should defer the new static configuration: %v", err)
	}
}

func TestTeamPresetChangeWaitsForIdleAndPreservesCanonicalHistory(t *testing.T) {
	a := newManualSessionTestApp(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	entered, release := make(chan struct{}, 1), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "team-apply", Kind: "openai", BaseURL: server.URL, Models: []string{"test"}, NoProxy: true}, "disposable-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDefaultModel("team-apply/test"); err != nil {
		t.Fatal(err)
	}
	settings := TeamAgentSettings{Name: "Chief", Main: true, Permission: "read-only", Preset: &TeamPresetDefinition{ID: "copied", Name: "Copied", Complete: true, Body: "ORIGINAL_PERSONA", RestrictTools: true, Tools: []string{}}}
	created, err := a.SaveTeamAgent(SaveTeamAgentRequest{RequestID: "create-apply-fixture", Settings: settings})
	if err != nil || created.Outcome != "applied" {
		t.Fatalf("create: %+v %v", created, err)
	}
	h, err := a.localTeamHost()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	pane, err := h.Ensure(ctx, team.SeatRef{Agent: created.TargetID})
	if err != nil {
		t.Fatal(err)
	}
	runtime := pane.(*desktopTeamRuntime)
	owner := runtime.owner()
	old := a.controllerForTab(owner).(*control.Controller)
	ref, _ := old.SessionRef()
	finished := make(chan error, 1)
	go func() { finished <- old.Run(ctx, "keep this history") }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	view, err := a.GetTeam()
	if err != nil {
		t.Fatal(err)
	}
	settings.Preset = &TeamPresetDefinition{ID: "copied", Name: "Copied", Complete: true, Body: "REPLACEMENT_PERSONA", RestrictTools: true, Tools: []string{}}
	updated, err := a.SaveTeamAgent(SaveTeamAgentRequest{ID: created.TargetID, ExpectedRevision: view.Roster.Revision, RequestID: "update-apply-fixture", Settings: settings})
	if err != nil || updated.Outcome != "applied" || !updated.View.Seats[0].SettingsPending {
		t.Fatalf("saved pending: %+v %v", updated, err)
	}
	if applied, err := a.applyTeamPresetIfIdle(ctx, owner); err != nil || applied {
		t.Fatalf("rebuilt busy runtime: %v %v", applied, err)
	}
	if !strings.Contains(old.SystemPrompt(), "ORIGINAL_PERSONA") {
		t.Fatal("active turn's static prompt changed")
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if done, finishing := old.TurnFinishingDone(); finishing {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	history := old.History()
	if applied, err := a.applyTeamPresetIfIdle(ctx, owner); err != nil || !applied {
		t.Fatalf("did not apply at idle: %v %v", applied, err)
	}
	next := a.controllerForTab(owner).(*control.Controller)
	nextRef, _ := next.SessionRef()
	// History is the executable model context, whose first system message is
	// intentionally replaced. Every conversation message and its identity stay.
	if len(history) == 0 || history[0].Role != "system" || len(next.History()) != len(history) {
		t.Fatal("configuration rebuild lost model context")
	}
	history[0].Content = "REPLACEMENT_PERSONA"
	if nextRef != ref || !reflect.DeepEqual(next.History(), history) {
		t.Fatal("configuration rebuild changed canonical identity or conversation history")
	}
	if !strings.Contains(next.SystemPrompt(), "REPLACEMENT_PERSONA") {
		t.Fatal("new runtime did not apply the copied definition")
	}
	view, err = a.GetTeam()
	if err != nil || view.Seats[0].SettingsPending {
		t.Fatalf("still pending after apply: %+v %v", view, err)
	}
	settings.Permission = "workspace-write"
	updated, err = a.SaveTeamAgent(SaveTeamAgentRequest{ID: created.TargetID, ExpectedRevision: view.Roster.Revision, RequestID: "permission-apply-fixture", Settings: settings})
	if err != nil || updated.Outcome != "applied" || next.ToolApprovalMode() != "workspace-write" {
		t.Fatalf("saved permission did not reach the actual owner: %+v %v / %s", updated, err, next.ToolApprovalMode())
	}
	applyTabToolApprovalModeToController(next, "danger-full-access")
	if next.ToolApprovalMode() != "workspace-write" {
		t.Fatal("a session choice widened the member's persistent permission")
	}
	settings.Folder = t.TempDir()
	updated, err = a.SaveTeamAgent(SaveTeamAgentRequest{ID: created.TargetID, ExpectedRevision: updated.View.Roster.Revision, RequestID: "directory-apply-fixture", Settings: settings})
	if err != nil || updated.Outcome != "applied" {
		t.Fatalf("directory migration: %+v %v", updated, err)
	}
	if err := runtime.Deliver(team.Delivery{Key: "stale-binding-message", Kind: team.DeliverMessage, Text: "must not run"}); err == nil {
		t.Fatal("old directory binding accepted new work")
	}
	reopened, err := h.Ensure(ctx, team.SeatRef{Agent: created.TargetID})
	if err != nil {
		t.Fatal(err)
	}
	migrated := a.controllerForTab(reopened.(*desktopTeamRuntime).owner()).(*control.Controller)
	migratedRef, _ := migrated.SessionRef()
	info, err := a.desktopSessionService("").Query().Stat(ctx, migratedRef)
	if err != nil || migratedRef == ref || !sameDesktopPath(info.CWD, settings.Folder) {
		t.Fatalf("directory did not obtain a new canonical workspace: %+v %v", info, err)
	}
	if !reflect.DeepEqual(migrated.History(), next.History()) {
		t.Fatal("directory migration lost conversation history")
	}
}

func TestTeamPresetWireDistinguishesUnrestrictedAndEmptyTools(t *testing.T) {
	for _, tools := range [][]string{nil, {}, {"read_file"}} {
		original := agentpreset.Default()
		original.Tools = tools
		wire := teamPresetView(&original)
		if wire.Tools == nil {
			t.Fatal("wire tools must be an array")
		}
		restored := wire.definition()
		if !reflect.DeepEqual(restored.Tools, tools) {
			t.Fatalf("tool permission changed in DTO round trip: %#v -> %#v", tools, restored.Tools)
		}
	}
}
