package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deepzora/desktop/internal/workspacestate"
	"deepzora/internal/agentpreset"
	"deepzora/internal/control"
	"deepzora/internal/session"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func TestTeamPrivateNavigationUsesMemberIdentityModelAndPermissions(t *testing.T) {
	a := newManualSessionTestApp(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	requests := make(chan string, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	if _, err := a.SaveProviderWithKey(ProviderView{Name: "private-fixture", Kind: "openai", BaseURL: server.URL, Models: []string{"default", "member"}, NoProxy: true}, "disposable-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDefaultModel("private-fixture/default"); err != nil {
		t.Fatal(err)
	}
	foreground, ordinary := manualAgentPresetTab(t, a, "default")
	a.mu.Lock()
	a.activeTabID = foreground.ID
	a.mu.Unlock()
	var member teamroster.Agent
	_, err := desktopTeamStore().Update(ctx, func(r *teamroster.Roster) error {
		p, err := agentpreset.Parse("private", "---\ncomplete: true\ninclude-runtime-context: false\ntools: []\n---\nPRIVATE_MEMBER_PERSONA")
		if err != nil {
			return err
		}
		member = r.AddAgent(teamroster.Agent{Name: "小测", Role: "Verifier", Instructions: "PRIVATE_MEMBER_INSTRUCTIONS", Main: true, Preset: &p, InitialModel: "private-fixture/member", Permission: "read-only"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	ref, err := a.PrepareTeamAgentSession(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.OpenSession(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetSessionSummary(ref); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("Team session exposed as a project summary: %v", err)
	}
	checkSend := func(wantTeam bool, model, input string) {
		t.Helper()
		tab, api := a.tabAndCtrlByID("")
		ctrl, ok := api.(*control.Controller)
		if !ok {
			t.Fatal("navigation did not retain real controller")
		}
		if (ctrl.BoundTeamSeat() != nil) != wantTeam {
			t.Fatalf("target identity inherited from wrong session: team=%v want=%v", ctrl.BoundTeamSeat() != nil, wantTeam)
		}
		if wantTeam {
			_, seat := ctrl.BoundTeamSeat().Team()
			if seat.Agent != member.ID || ctrl.ToolApprovalMode() != "read-only" {
				t.Fatalf("member identity/permission lost: %+v mode=%s", seat, ctrl.ToolApprovalMode())
			}
			permission := ctrl.PermissionSnapshot()
			if _, err := a.SetPermissionPresetForTab(tab.ID, permission.SessionID, "danger-full-access", permission.Revision); err == nil {
				t.Fatal("ordinary permission picker widened team member")
			}
		}
		if err := a.SubmitToTabWithID(tab.ID, input, "private-"+input); err != nil {
			t.Fatal(err)
		}
		select {
		case body := <-requests:
			var request struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal([]byte(body), &request); err != nil {
				t.Fatal(err)
			}
			if request.Model != model {
				t.Fatalf("wrong member model: %s want %s", request.Model, model)
			}
			for _, marker := range []string{"PRIVATE_MEMBER_PERSONA", "PRIVATE_MEMBER_INSTRUCTIONS", "agent-identity", "小测"} {
				if strings.Contains(body, marker) != wantTeam {
					t.Fatalf("provider request team marker %q present=%v want=%v", marker, strings.Contains(body, marker), wantTeam)
				}
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		waitNotRunning(t, ctrl)
	}
	checkSend(true, "member", "warm")
	memberController := a.activeCtrl()
	if _, err := a.OpenSession(ordinary.Ref); err != nil {
		t.Fatal(err)
	}
	checkSend(false, "default", "ordinary")
	// Navigating away retains the idle member runtime, just like CloseTab.
	runtime := &desktopTeamRuntime{app: a, ref: team.SeatRef{Agent: member.ID}, sessionID: ref.SessionID}
	if owner := runtime.owner(); owner == nil || a.controllerForTab(owner) != memberController {
		t.Fatal("navigation discarded idle Team runtime")
	}
	if _, err := a.OpenSession(ref); err != nil {
		t.Fatal(err)
	}
	if a.activeCtrl() != memberController {
		t.Fatal("Team navigation replaced retained controller")
	}
	checkSend(true, "member", "reattached")
	// A member's initial model seeds creation; a later session choice survives
	// idle reclamation and restart instead of being replaced by that seed.
	active, _ := a.tabAndCtrlByID("")
	if err := a.SetModelForTab(active.ID, "private-fixture/default"); err != nil {
		t.Fatal(err)
	}
	memberController = a.activeCtrl()
	if _, err := a.OpenSession(ordinary.Ref); err != nil {
		t.Fatal(err)
	}
	// Explicitly close the detached runtime to simulate an idle reclamation;
	// reopening must rebuild from the target identity, never from the surface.
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.OpenSession(ref); err != nil {
		t.Fatal(err)
	}
	if a.activeCtrl() == memberController {
		t.Fatal("cold reopen retained closed controller")
	}
	checkSend(true, "default", "cold")
	for _, node := range mustListProjectTree(t, a) {
		if strings.Contains(node.Root, member.ID) {
			t.Fatalf("private team workspace leaked into Projects: %s", node.Root)
		}
	}
	for _, workspace := range a.ListWorkspaces() {
		if strings.Contains(workspace.Path, member.ID) {
			t.Fatal("Team sandbox leaked into workspace picker")
		}
	}
	for _, topic := range a.projectTreeRuntimeTopics(a.catalogRuntimeSnapshots()) {
		if topic.Node.Session != nil && topic.Node.Session.SessionID == ref.SessionID {
			t.Fatal("Team session leaked through runtime project overlay")
		}
	}
}

func TestTeamWorkspaceShellFilterPreservesUserProjects(t *testing.T) {
	a := newManualSessionTestApp(t)
	internalRoot := filepath.Join(desktopTeamStore().Dir, "workspaces", "a-0123456789abcdef")
	userRoot := t.TempDir()
	for _, workspace := range []workspacestate.Workspace{
		{ID: "internal", Root: internalRoot, Title: "a-0123456789abcdef", Visible: true},
		{ID: "user", Root: userRoot, Title: "User project", Visible: true},
	} {
		if err := a.workspaceRegistry().EnsureWorkspace(t.Context(), workspace); err != nil {
			t.Fatal(err)
		}
	}
	for _, shells := range [][]ProjectNode{nil, {{Kind: "project", Root: internalRoot}, {Kind: "project", Root: userRoot}}} {
		result := a.mergeCanonicalWorkspaceShells(shells)
		if len(result) != 1 || result[0].Root != userRoot {
			t.Fatalf("wrong project shells: %+v", result)
		}
	}
}

func TestTeamRuntimeProjectRowsStayHiddenInSharedUserProject(t *testing.T) {
	a := newManualSessionTestApp(t)
	root := t.TempDir()
	rows := a.runtimeProjectTopicNodes("project", root, []catalogRuntimeSnapshot{
		{topicID: "ordinary", sessionPath: sessionRoute("ordinary"), workspaceRoot: root},
		{topicID: "member", sessionPath: sessionRoute("desktop-team-a-0123456789abcdef-seat"), workspaceRoot: root},
	})
	if len(rows) != 1 || rows[0].TopicID != "ordinary" {
		t.Fatalf("Team runtime leaked into shared user project: %+v", rows)
	}
}
