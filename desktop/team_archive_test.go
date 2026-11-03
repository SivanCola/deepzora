package main

import "testing"

func TestDesktopTeamArchiveRestoresStableIdentityAndNeverRestoresDeletedMember(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	a.ctx = t.Context()
	installNoopRuntimeEvents(a)
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	create, err := a.SaveTeamAgent(SaveTeamAgentRequest{RequestID: "archive-member-create", Settings: TeamAgentSettings{Name: "Ada", Permission: "read-only"}})
	if err != nil || create.Outcome != "applied" {
		t.Fatalf("create: %+v %v", create, err)
	}
	view := create.View
	// Keep the required main seat while Ada is archived.
	_, err = a.SaveTeamAgent(SaveTeamAgentRequest{RequestID: "archive-other-create", ExpectedRevision: view.Roster.Revision, Settings: TeamAgentSettings{Name: "Ben", Main: true, Permission: "read-only"}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = a.GetTeam()
	if err != nil {
		t.Fatal(err)
	}
	original := view.Roster.Agents[0]
	req := TeamTargetMutationRequest{AgentID: original.ID, RequestID: "archive-existing-member", ExpectedRevision: view.Roster.Revision}
	archived, err := a.ArchiveTeamAgent(req)
	if err != nil || archived.Outcome != "applied" {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	if len(archived.View.Roster.Agents) != 1 || len(archived.View.Roster.RetiredAgents) != 1 || !archived.View.Roster.RetiredAgents[0].Archived {
		t.Fatalf("archive did not retire execution identity: %+v", archived.View.Roster)
	}
	req.RequestID = "restore-existing-member"
	req.ExpectedRevision = archived.View.Roster.Revision
	restored, err := a.RestoreTeamAgent(req)
	if err != nil || restored.Outcome != "applied" {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	got := restored.View.Roster.Agents[1]
	if got.ID != original.ID || got.SessionPath != original.SessionPath || got.Permission != original.Permission {
		t.Fatalf("restore changed canonical identity: %+v", got)
	}
	req.RequestID = "delete-existing-member"
	req.ExpectedRevision = restored.View.Roster.Revision
	deleted, err := a.DeleteTeamTarget(req)
	if err != nil || deleted.Outcome != "applied" {
		t.Fatalf("delete: %+v %v", deleted, err)
	}
	req.RequestID = "restore-deleted-member"
	req.ExpectedRevision = deleted.View.Roster.Revision
	refused, err := a.RestoreTeamAgent(req)
	if err != nil || refused.Reason != "target_invalid" {
		t.Fatalf("deleted member became restorable: %+v %v", refused, err)
	}
}
