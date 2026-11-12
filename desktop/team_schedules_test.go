package service

import (
	"context"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"testing"
	"time"
)

type scheduleTestPanes struct{ delivered []team.Delivery }

func (p *scheduleTestPanes) Ensure(context.Context, team.SeatRef) (team.Pane, error) { return p, nil }
func (p *scheduleTestPanes) Live(team.SeatRef) (team.Pane, bool)                     { return p, true }
func (p *scheduleTestPanes) Deliver(d team.Delivery) error {
	p.delivered = append(p.delivered, d)
	return nil
}
func (p *scheduleTestPanes) Cancel() {}

func TestTeamSchedulerSkipsSleepGapAndClaimsOccurrenceOnce(t *testing.T) {
	a, id := newMemoryTeamApp(t)
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(-3 * time.Minute)
	sc := teamroster.Schedule{ID: teamroster.NewScheduleID(), Name: "Check", Agent: id, Prompt: "Review", Repeat: "every", IntervalMinutes: 10, Timezone: "Asia/Singapore", Anchor: due, Next: due, Enabled: true, Days: []int{}}
	_, err := desktopTeamStore().UpdateSchedules(t.Context(), nil, func(book *teamroster.ScheduleBook) (bool, error) {
		book.Schedules = append(book.Schedules, sc)
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	panes := &scheduleTestPanes{}
	svc := team.New(team.Options{Store: desktopTeamStore(), Panes: panes})
	if err := a.fireTeamSchedule(t.Context(), svc, sc, now); err != nil {
		t.Fatal(err)
	}
	book, err := desktopTeamStore().ScheduleBook()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes.delivered) != 0 || book.Schedules[0].LastOutcome != "skipped_late" {
		t.Fatalf("sleep gap replayed: %+v %+v", panes.delivered, book)
	}
	sc = book.Schedules[0]
	now = sc.Next.Add(time.Second)
	for range 2 {
		if err := a.fireTeamSchedule(t.Context(), svc, sc, now); err != nil {
			t.Fatal(err)
		}
	}
	if len(panes.delivered) != 1 || panes.delivered[0].Ceiling != "workspace-write" || panes.delivered[0].Kind != team.DeliverSchedule {
		t.Fatalf("duplicate or human authority: %+v", panes.delivered)
	}
}

func TestTeamSchedulerBusyTargetWaitsWithinWindowThenSkips(t *testing.T) {
	a, id := newMemoryTeamApp(t)
	now := time.Now().UTC()
	sc := teamroster.Schedule{ID: teamroster.NewScheduleID(), Name: "Check", Agent: id, Prompt: "Review", Repeat: "once", Timezone: "UTC", Anchor: now, Next: now, Enabled: true, Days: []int{}}
	_, err := desktopTeamStore().UpdateSchedules(t.Context(), nil, func(book *teamroster.ScheduleBook) (bool, error) {
		book.Schedules = append(book.Schedules, sc)
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.teamState.mu.Lock()
	a.teamState.reviews = &teamMemoryReviews{running: &team.SeatRef{Agent: id}}
	a.teamState.mu.Unlock()
	// This test owns a synthetic busy marker, not a worker goroutine.
	defer func() { a.teamState.mu.Lock(); a.teamState.reviews = nil; a.teamState.mu.Unlock() }()
	panes := &scheduleTestPanes{}
	svc := team.New(team.Options{Store: desktopTeamStore(), Panes: panes})
	if err := a.fireTeamSchedule(t.Context(), svc, sc, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	book, err := desktopTeamStore().ScheduleBook()
	if err != nil {
		t.Fatal(err)
	}
	if !book.Schedules[0].Next.Equal(sc.Next) || len(panes.delivered) != 0 {
		t.Fatal("busy target did not retain window")
	}
	if err := a.fireTeamSchedule(t.Context(), svc, sc, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	book, err = desktopTeamStore().ScheduleBook()
	if err != nil {
		t.Fatal(err)
	}
	if book.Schedules[0].LastOutcome != "skipped_late" || book.Schedules[0].Enabled {
		t.Fatalf("expired busy once was retried: %+v", book.Schedules[0])
	}
}
