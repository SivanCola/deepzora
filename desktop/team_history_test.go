package main

import (
	"os"
	"testing"

	"deepzora/internal/teamroster"
)

func TestTeamGroupHistoryIsBidirectionalBoundedAndRejectsReplacement(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	store := desktopTeamStore()
	group := ""
	_, err := store.Update(t.Context(), func(r *teamroster.Roster) error {
		chief := r.AddAgent(teamroster.Agent{Name: "Chief", Main: true})
		ada := r.AddAgent(teamroster.Agent{Name: "Ada"})
		group = r.AddGroup(teamroster.Group{Name: "Group", Mode: teamroster.ModeEveryone, Members: []string{chief.ID, ada.ID}}).ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if _, err := store.AppendLine(t.Context(), group, teamroster.GroupLine{Kind: teamroster.LinePost, From: teamroster.ByUser, Text: "message"}); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := a.ReadTeamGroupHistory(group, "")
	if err != nil || latest.Status != "ready" || len(latest.Lines) != 32 || latest.Lines[0].Seq != 69 || latest.OlderCursor == "" || latest.NewerCursor != "" {
		t.Fatalf("latest: %+v %v", latest, err)
	}
	older, err := a.ReadTeamGroupHistory(group, latest.OlderCursor)
	if err != nil || older.Status != "ready" || older.Lines[0].Seq != 37 || older.NewerCursor == "" {
		t.Fatalf("older: %+v %v", older, err)
	}
	if _, err := store.AppendLine(t.Context(), group, teamroster.GroupLine{Kind: teamroster.LinePost, From: teamroster.ByUser, Text: "appended"}); err != nil {
		t.Fatal(err)
	}
	newer, err := a.ReadTeamGroupHistory(group, older.NewerCursor)
	if err != nil || newer.Status != "ready" || len(newer.Lines) != 32 || newer.Lines[len(newer.Lines)-1].Seq != 100 {
		t.Fatalf("append invalidated fixed snapshot: %+v %v", newer, err)
	}
	path, err := store.GroupLogPath(group)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	stale, err := a.ReadTeamGroupHistory(group, older.NewerCursor)
	if err != nil || stale.Status != "stale_cursor" {
		t.Fatalf("replacement accepted cursor: %+v %v", stale, err)
	}
	stale, err = a.ReadTeamGroupHistory(group, "unreadable")
	if err != nil || stale.Status != "stale_cursor" {
		t.Fatalf("malformed cursor became transport failure: %+v %v", stale, err)
	}
}
