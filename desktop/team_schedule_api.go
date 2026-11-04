package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type TeamScheduleView struct {
	Revision  uint64                `json:"revision"`
	Schedules []teamroster.Schedule `json:"schedules"`
}
type SaveTeamScheduleRequest struct {
	ExpectedRevision       uint64              `json:"expectedRevision"`
	ExpectedRosterRevision uint64              `json:"expectedRosterRevision"`
	RequestID              string              `json:"requestId"`
	Schedule               teamroster.Schedule `json:"schedule"`
	Delete                 bool                `json:"delete"`
}
type TeamScheduleResult struct {
	Outcome  string           `json:"outcome"`
	Reason   string           `json:"reason,omitempty"`
	Message  string           `json:"message,omitempty"`
	TargetID string           `json:"targetId,omitempty"`
	View     TeamScheduleView `json:"view"`
}

func (a *App) GetTeamSchedules() (TeamScheduleView, error) {
	book, err := desktopTeamStore().ScheduleBook()
	return TeamScheduleView{Revision: book.Revision, Schedules: book.Schedules}, err
}
func validTeamScheduleTarget(r teamroster.Roster, sc teamroster.Schedule) error {
	if sc.Agent != "" {
		if _, ok := r.Agent(sc.Agent); !ok {
			return teamroster.ErrUnknownAgent
		}
	} else {
		g, ok := r.Group(sc.Group)
		if !ok {
			return teamroster.ErrUnknownGroup
		}
		if g.Archived {
			return team.ErrArchived
		}
	}
	return nil
}
func (a *App) SaveTeamSchedule(req SaveTeamScheduleRequest) (TeamScheduleResult, error) {
	result := TeamScheduleResult{Outcome: "rejected"}
	err := a.withTeamWriter(func(ctx context.Context, svc *team.Service) error {
		return svc.Configure(ctx, func() error {
			if len(req.RequestID) < 8 || len(req.RequestID) > 128 {
				return teamroster.ErrInvalid
			}
			_, err := desktopTeamStore().UpdateSchedules(ctx, nil, func(book *teamroster.ScheduleBook) (bool, error) {
				digest := teamMutationDigest(req)
				for _, receipt := range book.Receipts {
					if receipt.Key == req.RequestID {
						if receipt.Digest != digest {
							return false, teamroster.ErrRequestReused
						}
						result.TargetID = receipt.Target
						return false, nil
					}
				}
				if book.Revision != req.ExpectedRevision {
					return false, teamroster.ErrConflict
				}
				return true, desktopTeamStore().InspectAt(ctx, req.ExpectedRosterRevision, func(r teamroster.Roster) error {
					sc := req.Schedule
					index := slices.IndexFunc(book.Schedules, func(old teamroster.Schedule) bool { return old.ID == sc.ID })
					if sc.ID != "" && index < 0 {
						return teamroster.ErrInvalid
					}
					if req.Delete {
						if index < 0 {
							return teamroster.ErrInvalid
						}
						book.Schedules = slices.Delete(book.Schedules, index, index+1)
					} else {
						if err := validTeamScheduleTarget(r, sc); err != nil {
							return err
						}
						if err := sc.Validate(); err != nil {
							return err
						}
						sc.LastAt, sc.LastOccurrence, sc.LastOutcome = time.Time{}, "", ""
						sc.Next = time.Time{}
						if sc.Enabled {
							sc.Next = sc.NextAfter(time.Now())
						}
						if sc.ID == "" {
							sc.ID = teamroster.NewScheduleID()
							book.Schedules = append(book.Schedules, sc)
						} else {
							book.Schedules[index] = sc
						}
					}
					result.TargetID = sc.ID
					book.Receipts = append(book.Receipts, teamroster.ScheduleReceipt{Key: req.RequestID, Digest: digest, Target: sc.ID})
					return nil
				})
			})
			return err
		})
	})
	view, readErr := a.GetTeamSchedules()
	if readErr != nil {
		return result, readErr
	}
	result.View = view
	if err != nil {
		result.Reason, result.Message = teamMutationReason(err), err.Error()
		return result, nil
	}
	result.Outcome = "applied"
	a.emitTeamChanged()
	return result, nil
}

type RunTeamScheduleRequest struct {
	ID               string `json:"id"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	RequestID        string `json:"requestId"`
}

// Immediate execution is still scheduler input; clicking Run does not approve
// any future sensitive tool, mutate member permissions or resume a Goal.
func (a *App) RunTeamSchedule(req RunTeamScheduleRequest) (TeamScheduleResult, error) {
	result := TeamScheduleResult{Outcome: "rejected"}
	err := a.withTeamWriter(func(ctx context.Context, svc *team.Service) error {
		return svc.WithScheduleAdmission(ctx, func() error {
			if len(req.RequestID) < 8 || len(req.RequestID) > 128 {
				return teamroster.ErrInvalid
			}
			var selected teamroster.Schedule
			admitted := false
			_, err := desktopTeamStore().UpdateSchedules(ctx, nil, func(book *teamroster.ScheduleBook) (bool, error) {
				digest := teamMutationDigest(req)
				for _, receipt := range book.Receipts {
					if receipt.Key == req.RequestID {
						if receipt.Digest != digest {
							return false, teamroster.ErrRequestReused
						}
						if receipt.Outcome != "delivered" {
							return false, fmt.Errorf("previous scheduled input is %s; inspect the target conversation before running a new occurrence", receipt.Outcome)
						}
						return false, nil
					}
				}
				if book.Revision != req.ExpectedRevision {
					return false, teamroster.ErrConflict
				}
				index := slices.IndexFunc(book.Schedules, func(sc teamroster.Schedule) bool { return sc.ID == req.ID })
				if index < 0 {
					return false, teamroster.ErrInvalid
				}
				selected = book.Schedules[index]
				if err := a.requireTeamTargetIdle(selected.Agent, selected.Group); err != nil {
					return false, err
				}
				book.Receipts = append(book.Receipts, teamroster.ScheduleReceipt{Key: req.RequestID, Digest: digest, Target: req.ID, Outcome: "dispatching"})
				admitted = true
				return true, nil
			})
			if err != nil || !admitted {
				return err
			}
			dispatchErr := svc.DispatchSchedule(ctx, selected, "manual-"+req.RequestID)
			_, saveErr := desktopTeamStore().UpdateSchedules(ctx, nil, func(book *teamroster.ScheduleBook) (bool, error) {
				for i := range book.Receipts {
					if book.Receipts[i].Key == req.RequestID {
						book.Receipts[i].Outcome = "delivered"
						if dispatchErr != nil {
							book.Receipts[i].Outcome = "failed"
						}
						return true, nil
					}
				}
				return false, teamroster.ErrInvalid
			})
			if dispatchErr != nil {
				return dispatchErr
			}
			return saveErr
		})
	})
	result.View, _ = a.GetTeamSchedules()
	result.TargetID = req.ID
	if err != nil {
		result.Reason, result.Message = teamMutationReason(err), err.Error()
		return result, nil
	}
	result.Outcome = "accepted"
	a.emitTeamChanged()
	return result, nil
}
