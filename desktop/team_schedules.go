package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func (a *App) tickTeamSchedules(ctx context.Context, now time.Time) error {
	a.teamState.scheduleMu.Lock()
	defer a.teamState.scheduleMu.Unlock()
	h, err := a.localTeamHost()
	if err != nil {
		return err
	}
	return h.WithWriter(ctx, func(ctx context.Context, svc *team.Service) error {
		if !a.teamState.schedulesRecovered {
			if err := recoverTeamScheduleDispatches(ctx, now); err != nil {
				return err
			}
			a.teamState.schedulesRecovered = true
		}
		book, err := desktopTeamStore().ScheduleBook()
		if err != nil {
			return err
		}
		var errs []error
		for _, sc := range book.Schedules {
			if !sc.Enabled || sc.Next.IsZero() || sc.Next.After(now) {
				continue
			}
			errs = append(errs, a.fireTeamSchedule(ctx, svc, sc, now))
		}
		return errors.Join(errs...)
	})
}

// A crash leaves dispatching as an interrupted occurrence. It is not replayed:
// the durable inbox may already have settled it before the crash.
func recoverTeamScheduleDispatches(ctx context.Context, now time.Time) error {
	_, err := desktopTeamStore().UpdateSchedules(ctx, nil, func(book *teamroster.ScheduleBook) (bool, error) {
		changed := false
		for i := range book.Receipts {
			if book.Receipts[i].Outcome == "dispatching" {
				book.Receipts[i].Outcome = "interrupted"
				changed = true
			}
		}
		for i := range book.Schedules {
			sc := &book.Schedules[i]
			if sc.LastOutcome == "dispatching" {
				sc.LastOutcome = "interrupted"
				sc.LastAt = now.UTC()
				changed = true
			}
		}
		return changed, nil
	})
	return err
}

func (a *App) fireTeamSchedule(ctx context.Context, svc *team.Service, observed teamroster.Schedule, now time.Time) error {
	return svc.WithScheduleAdmission(ctx, func() error { return a.fireTeamScheduleAdmitted(ctx, svc, observed, now) })
}

func (a *App) fireTeamScheduleAdmitted(ctx context.Context, svc *team.Service, observed teamroster.Schedule, now time.Time) error {
	due := observed.Next
	key := fmt.Sprintf("schedule-%s-%d", observed.ID, due.Unix())
	admitted := false
	_, err := desktopTeamStore().UpdateSchedules(ctx, nil, func(book *teamroster.ScheduleBook) (bool, error) {
		i := slices.IndexFunc(book.Schedules, func(sc teamroster.Schedule) bool { return sc.ID == observed.ID })
		if i < 0 {
			return false, nil
		}
		sc := &book.Schedules[i]
		if !sc.Enabled || !sc.Next.Equal(due) || sc.LastOccurrence == key {
			return false, nil
		}
		outcome := "skipped_late"
		if now.Sub(due) <= teamroster.ScheduleGrace {
			if err := a.requireTeamTargetIdle(sc.Agent, sc.Group); err != nil {
				if errors.Is(err, errTeamBusy) {
					return false, nil
				}
				return false, err
			}
			outcome = "dispatching"
			admitted = true
		}
		sc.LastOccurrence, sc.LastOutcome, sc.LastAt = key, outcome, now.UTC()
		sc.Next = sc.NextAfter(now)
		sc.Enabled = !sc.Next.IsZero()
		return true, nil
	})
	if err != nil || !admitted {
		return err
	}
	dispatchErr := svc.DispatchSchedule(ctx, observed, key)
	_, saveErr := desktopTeamStore().UpdateSchedules(ctx, nil, func(book *teamroster.ScheduleBook) (bool, error) {
		for i := range book.Schedules {
			sc := &book.Schedules[i]
			if sc.ID == observed.ID && sc.LastOccurrence == key {
				sc.LastOutcome = "delivered"
				if dispatchErr != nil {
					sc.LastOutcome = "failed"
				}
				return true, nil
			}
		}
		return false, nil
	})
	a.emitTeamChanged()
	return errors.Join(dispatchErr, saveErr)
}
