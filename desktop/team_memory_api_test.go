package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func newMemoryTeamApp(t *testing.T) (*App, string) {
	t.Helper()
	isolateDesktopUserDirs(t)
	a := NewApp()
	a.ctx = t.Context()
	installNoopRuntimeEvents(a)
	t.Cleanup(func() {
		if err := a.stopTeamHost(); err != nil {
			t.Error(err)
		}
	})
	result, err := a.SaveTeamAgent(SaveTeamAgentRequest{RequestID: "create-memory-agent", Settings: TeamAgentSettings{Name: "Ada", Permission: "read-only"}})
	if err != nil || result.Outcome != "applied" {
		t.Fatalf("create: %+v %v", result, err)
	}
	return a, result.TargetID
}
func TestTeamMemoryManualRetryAndNamespaceIsolation(t *testing.T) {
	a, id := newMemoryTeamApp(t)
	target := TeamMemoryTarget{Tier: "private", AgentID: id}
	view, err := a.GetTeamMemory(target)
	if err != nil {
		t.Fatal(err)
	}
	req := SaveTeamMemoryRequest{Target: target, ExpectedRosterRevision: view.RosterRevision, RequestID: "create-memory-fact", Fact: MemoryFact{Name: "preference", Description: "language", Body: "Use Chinese"}}
	saved, err := a.SaveTeamMemory(req)
	if err != nil || saved.Outcome != "applied" {
		t.Fatalf("save: %+v %v", saved, err)
	}
	retry, err := a.SaveTeamMemory(req)
	if err != nil || retry.Fact.ID != saved.Fact.ID {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	shared, err := a.GetTeamMemory(TeamMemoryTarget{Tier: "team"})
	if err != nil || len(shared.Facts) != 0 {
		t.Fatalf("private memory leaked: %+v %v", shared, err)
	}
	req.RequestID = "stale-memory-fact"
	req.Fact = saved.Fact
	req.Fact.Revision++
	rejected, err := a.SaveTeamMemory(req)
	if err != nil || rejected.Reason != "conflict" {
		t.Fatalf("stale: %+v %v", rejected, err)
	}
	if _, err := a.GetTeamMemory(TeamMemoryTarget{Tier: "group", GroupID: "../../outside"}); err == nil {
		t.Fatal("accepted arbitrary namespace")
	}
}
func TestTeamGroupMemoryEvidenceConsumesEarliestBatchAndExcludesSchedules(t *testing.T) {
	a, id := newMemoryTeamApp(t)
	r, err := desktopTeamStore().Update(t.Context(), func(r *teamroster.Roster) error {
		ben := r.AddAgent(teamroster.Agent{Name: "Ben", Permission: "read-only"})
		r.AddGroup(teamroster.Group{Name: "Review", Members: []string{id, ben.ID}, Admin: id, Mode: teamroster.ModeEveryone})
		r.Prefs.MemoryReview = true
		r.Prefs.MemoryReviewStartedAt = time.Now().Add(-time.Minute)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	group := r.Groups[0].ID
	for i := range 20 {
		source := ""
		if i == 3 {
			source = "scheduler"
		}
		_, err := desktopTeamStore().AppendLine(t.Context(), group, teamroster.GroupLine{From: teamroster.ByUser, Kind: teamroster.LinePost, Text: fmt.Sprintf("fact-%02d %s", i, strings.Repeat("long evidence ", 500)), Source: source, At: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := loadTeamReviewState(r.Prefs.MemoryReviewStartedAt)
	if err != nil {
		t.Fatal(err)
	}
	reviews := teamMemoryReviews{app: a}
	lines, store, advance, more, err := reviews.evidence(context.Background(), r, team.SeatRef{Agent: id, Group: group}, &state)
	if err != nil || !more || len(lines) != 16 || !strings.HasPrefix(lines[0].Text, "fact-00 ") || !strings.HasPrefix(lines[15].Text, "fact-16 ") {
		t.Fatalf("lost early evidence: %+v more=%v err=%v", lines, more, err)
	}
	if _, err := team.ReviewMemory(t.Context(), store, lines, func(_ context.Context, _, prompt string) (string, error) {
		for i := range 17 {
			if i != 3 && !strings.Contains(prompt, fmt.Sprintf("fact-%02d", i)) {
				t.Fatalf("checkpoint would consume omitted human evidence %d", i)
			}
		}
		if strings.Contains(prompt, "fact-03") {
			t.Fatal("scheduler content entered human evidence")
		}
		return `{"changes":[]}`, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := advance(); err != nil {
		t.Fatal(err)
	}
	state, err = loadTeamReviewState(r.Prefs.MemoryReviewStartedAt)
	if err != nil {
		t.Fatal(err)
	}
	lines, _, _, more, err = reviews.evidence(context.Background(), r, team.SeatRef{Agent: id, Group: group}, &state)
	if err != nil || more || len(lines) != 3 || !strings.HasPrefix(lines[0].Text, "fact-17 ") {
		t.Fatalf("batch cursor lost remaining evidence: %+v %v %v", lines, more, err)
	}
}
