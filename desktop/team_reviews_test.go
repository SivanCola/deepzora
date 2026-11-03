package main

import (
	"context"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"errors"
	"testing"
	"time"
)

func TestDisablingTeamReviewCancelsAndJoinsBeforePreferenceCommit(t *testing.T) {
	a, id := newMemoryTeamApp(t)
	roster, err := desktopTeamStore().Update(t.Context(), func(r *teamroster.Roster) error {
		ben := r.AddAgent(teamroster.Agent{Name: "Ben", Permission: "read-only"})
		r.AddGroup(teamroster.Group{Name: "Review", Mode: teamroster.ModeEveryone, Members: []string{id, ben.ID}, Admin: id, MemberSessions: map[string]string{id: sessionRoute("desktop-team-review-fixture")}})
		r.Prefs.MemoryReview = true
		r.Prefs.MemoryReviewStartedAt = time.Now().Add(-time.Minute)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	group := roster.Groups[0].ID
	_, err = desktopTeamStore().AppendLine(t.Context(), group, teamroster.GroupLine{From: teamroster.ByUser, Kind: teamroster.LinePost, Text: "Remember I prefer Chinese", At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	closed := make(chan struct{})
	finished := make(chan error, 1)
	reviews := &teamMemoryReviews{app: a, ctx: ctx, cancel: cancel, done: make(chan struct{}), pending: map[string]teamReviewJob{}}
	reviews.ask = func(context.Context, team.SeatRef, string, string, string) (team.Reviewer, func(), error) {
		return func(ctx context.Context, _, _ string) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		}, func() { close(closed) }, nil
	}
	a.teamState.mu.Lock()
	a.teamState.reviews = reviews
	a.teamState.mu.Unlock()
	go func() { defer close(reviews.done); finished <- reviews.run(team.SeatRef{Agent: id, Group: group}) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("review did not start")
	}
	result, err := a.SaveTeamMemoryPreferences(SaveTeamMemoryPreferencesRequest{ExpectedRevision: roster.Revision, RequestID: "disable-running-review", Enabled: false})
	if err != nil || result.Outcome != "applied" {
		t.Fatalf("disable: %+v %v", result, err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("preference returned before provider cleanup")
	}
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("review cancellation: %v", err)
	}
	if len(team.SharedStore(desktopTeamStore().Dir, team.TierGroup, group).ListAll()) != 0 {
		t.Fatal("canceled review published facts")
	}
	if result.View.Roster.Prefs.MemoryReview {
		t.Fatal("review remained enabled")
	}
}
