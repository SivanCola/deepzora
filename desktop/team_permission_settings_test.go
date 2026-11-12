package service

import (
	"testing"

	"deepzora/internal/control"
	"deepzora/internal/team"
)

func TestTeamPermissionSettingsSynchronizeSeatsAndRejectStaleEdits(t *testing.T) {
	a, _, private, id, settings := teamAdmissionFixture(t)
	view, err := a.GetTeam()
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.SaveTeamAgent(SaveTeamAgentRequest{ExpectedRevision: view.Roster.Revision, RequestID: "permission-other", Settings: TeamAgentSettings{Name: "Ben", Permission: "read-only"}})
	if err != nil || other.Outcome != "applied" {
		t.Fatalf("other: %+v %v", other, err)
	}
	group, err := a.SaveTeamGroup(SaveTeamGroupRequest{ExpectedRevision: other.View.Roster.Revision, RequestID: "permission-group", Settings: TeamGroupSettings{Name: "Review", Members: []string{id, other.TargetID}, Mode: "everyone", RelayLimit: 20}})
	if err != nil || group.Outcome != "applied" {
		t.Fatalf("group: %+v %v", group, err)
	}
	h, err := a.localTeamHost()
	if err != nil {
		t.Fatal(err)
	}
	seat := func(ref team.SeatRef) *control.Controller {
		t.Helper()
		pane, err := h.Ensure(t.Context(), ref)
		if err != nil {
			t.Fatal(err)
		}
		return a.controllerForTab(pane.(*desktopTeamRuntime).owner()).(*control.Controller)
	}
	groupSeat := seat(team.SeatRef{Agent: id, Group: group.TargetID})
	otherSeat := seat(team.SeatRef{Agent: other.TargetID})
	for _, permission := range []string{"workspace-write", "danger-full-access", "read-only"} {
		view, err := a.GetTeam()
		if err != nil {
			t.Fatal(err)
		}
		settings.Permission = permission
		req := SaveTeamAgentRequest{ID: id, ExpectedRevision: view.Roster.Revision, RequestID: "permission-" + permission, Settings: settings}
		result, err := a.SaveTeamAgent(req)
		if err != nil || result.Outcome != "applied" {
			t.Fatalf("save %s: %+v %v", permission, result, err)
		}
		for _, controller := range []*control.Controller{private, groupSeat} {
			if got := controller.PermissionSnapshot().Preset; got != permission {
				t.Fatalf("seat permission = %q, want %q", got, permission)
			}
		}
		if otherSeat.PermissionSnapshot().Preset != "read-only" {
			t.Fatal("permission edit changed another member")
		}
		req.RequestID += "-stale"
		req.Settings.Name = "must not overwrite"
		stale, err := a.SaveTeamAgent(req)
		if err != nil || stale.Outcome != "rejected" || stale.Reason != "conflict" {
			t.Fatalf("stale edit: %+v %v", stale, err)
		}
		member := stale.View.Roster.Agents[0]
		if member.Name != settings.Name || member.Permission != permission || member.Preset.Body != settings.Preset.Body {
			t.Fatal("permission update lost member configuration")
		}
	}
}
