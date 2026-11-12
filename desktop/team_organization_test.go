package service

import (
	"errors"
	"os"
	"strings"
	"testing"

	"deepzora/internal/teamroster"
)

func teamOrganizationFixture(t *testing.T) (*App, teamroster.Roster, string) {
	t.Helper()
	a := newManualSessionTestApp(t)
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	group := ""
	r, err := desktopTeamStore().Update(t.Context(), func(r *teamroster.Roster) error {
		chief := r.AddAgent(teamroster.Agent{Name: "Chief", Main: true})
		ada := r.AddAgent(teamroster.Agent{Name: "Ada With Spaces", Avatar: teamroster.Avatar{Glyph: "bot", Color: "#123456"}})
		group = r.AddGroup(teamroster.Group{Name: "Project", Members: []string{chief.ID, ada.ID}, Mode: teamroster.ModeEveryone}).ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := a.localTeamHost()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	return a, r, group
}

func TestTeamDeletionAndReorderingKeepHistoricalIdentities(t *testing.T) {
	a, roster, group := teamOrganizationFixture(t)
	store := desktopTeamStore()
	ada := roster.Agents[1]
	if _, err := store.AppendLine(t.Context(), group, teamroster.GroupLine{From: ada.ID, Kind: teamroster.LinePost, Text: "keep my history"}); err != nil {
		t.Fatal(err)
	}
	ordered, err := a.ReorderTeam(ReorderTeamRequest{ExpectedRevision: roster.Revision, RequestID: "reorder-team-fixture", AgentIDs: []string{ada.ID, roster.Agents[0].ID}, GroupIDs: []string{group}})
	if err != nil || ordered.Outcome != "applied" || ordered.View.Roster.Agents[0].ID != ada.ID {
		t.Fatalf("order: %+v %v", ordered, err)
	}
	x, _, err := store.OpenExchange(t.Context(), teamroster.Exchange{From: roster.Agents[0].ID, To: ada.ID, ReplyTo: teamroster.ReplyToUser}, "pending delivery")
	if err != nil {
		t.Fatal(err)
	}
	req := TeamTargetMutationRequest{AgentID: ada.ID, ExpectedRevision: ordered.View.Roster.Revision, RequestID: "retire-team-fixture"}
	busy, err := a.DeleteTeamTarget(req)
	if err != nil || busy.Reason != "busy" {
		t.Fatalf("deleted pending exchange: %+v %v", busy, err)
	}
	if _, err := store.SetExchangeState(t.Context(), x.ID, teamroster.ExchangeInterrupted, x.At); err != nil {
		t.Fatal(err)
	}
	deleted, err := a.DeleteTeamTarget(req)
	if err != nil || deleted.Outcome != "applied" {
		t.Fatalf("retire: %+v %v", deleted, err)
	}
	if len(deleted.View.Roster.Agents) != 1 || len(deleted.View.Roster.Groups) != 0 || len(deleted.View.Roster.RetiredGroups) != 1 || len(deleted.View.Roster.RetiredAgents) != 1 {
		t.Fatalf("identity history lost: %+v", deleted.View.Roster)
	}
	if got := deleted.View.Roster.RetiredAgents[0]; got.Name != ada.Name || got.Avatar != ada.Avatar {
		t.Fatal("retirement changed the speaker")
	}
	page, err := a.ReadTeamGroupHistory(group, "")
	if err != nil || len(page.Lines) != 1 || page.Lines[0].From != ada.ID {
		t.Fatalf("retired history unreadable: %+v %v", page, err)
	}
	stored, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored.Agent(ada.ID); ok {
		t.Fatal("retired identity retained execution authority")
	}
	retry, err := a.DeleteTeamTarget(req)
	if err != nil || retry.Outcome != "applied" || len(retry.View.Roster.RetiredAgents) != 1 {
		t.Fatalf("retry duplicated retirement: %+v %v", retry, err)
	}
}

func TestTeamDraftCASAndSnapshotReadWatermarkSurviveRestart(t *testing.T) {
	a, roster, group := teamOrganizationFixture(t)
	request := SaveTeamGroupDraftRequest{GroupID: group, Draft: TeamGroupDraft{RequestID: "durable-submission-id", Text: "retained input", Mentions: []string{roster.Agents[1].ID}}}
	saved, err := a.SaveTeamGroupDraft(request)
	if err != nil || saved.Outcome != "applied" {
		t.Fatalf("draft: %+v %v", saved, err)
	}
	retry, err := a.SaveTeamGroupDraft(request)
	if err != nil || retry.View.Revision != saved.View.Revision {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	request.Draft.Text = "stale writer"
	conflict, err := a.SaveTeamGroupDraft(request)
	if err != nil || conflict.Reason != "conflict" {
		t.Fatalf("CAS lost: %+v %v", conflict, err)
	}
	restarted := NewApp()
	draft, err := restarted.ReadTeamGroupDraft(group)
	if err != nil || draft.Draft.Text != "retained input" || draft.Draft.RequestID != "durable-submission-id" {
		t.Fatalf("restart lost request identity: %+v %v", draft, err)
	}
	store := desktopTeamStore()
	for range 3 {
		if _, err := store.AppendLine(t.Context(), group, teamroster.GroupLine{Kind: teamroster.LinePost, From: roster.Agents[1].ID, Text: "reply"}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := a.ReadTeamGroupHistory(group, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.MarkTeamGroupRead(group, page.Snapshot, 2); err != nil {
		t.Fatal(err)
	}
	if err := a.MarkTeamGroupRead(group, page.Snapshot, 1); err != nil {
		t.Fatal(err)
	}
	status, err := teamGroupStatus(store, group)
	if err != nil || status.ReadSequence != 2 || status.Unread != 1 {
		t.Fatalf("watermark regressed: %+v %v", status, err)
	}
	path, err := store.GroupLogPath(group)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), "reply", "other")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.MarkTeamGroupRead(group, page.Snapshot, 3); !errors.Is(err, errTeamHistoryStale) {
		t.Fatalf("stale view marked replacement as read: %v", err)
	}
	status, err = teamGroupStatus(store, group)
	if err != nil || status.ReadSequence != 0 || status.Unread != 3 {
		t.Fatalf("replacement inherited old read state: %+v %v", status, err)
	}
}
