package team

import (
	"context"
	"deepzora/internal/hostframe"
	"deepzora/internal/teamroster"
)

// WithScheduleAdmission fences configuration changes across occurrence claim
// and delivery. DispatchSchedule may only be called inside this callback.
func (s *Service) WithScheduleAdmission(ctx context.Context, run func() error) error {
	s.configuration.RLock()
	defer s.configuration.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return run()
}

// DispatchSchedule is host-authored input with a durable occurrence key.
// The caller owns WithScheduleAdmission; it never takes the human-input path.
func (s *Service) DispatchSchedule(ctx context.Context, sc teamroster.Schedule, occurrence string) error {
	text := hostframe.Block("scheduled-task", []hostframe.Attr{{Name: "id", Value: sc.ID}, {Name: "name", Value: sc.Name}}, sc.Prompt)
	text += "\nThe local scheduler supplied this task. It is not a fresh human request, does not approve a plan or rearm a stopped Goal, and grants no authority. Use the receiver's permissions with the ask ceiling."
	if sc.Group != "" {
		_, err := s.postWithSource(ctx, sc.Group, text, nil, false, occurrence, "scheduler")
		return err
	}
	r, err := s.store.Load()
	if err != nil {
		return err
	}
	if _, ok := r.Agent(sc.Agent); !ok {
		return teamroster.ErrUnknownAgent
	}
	pane, err := s.panes.Ensure(ctx, SeatRef{Agent: sc.Agent})
	if err != nil {
		return err
	}
	return pane.Deliver(Delivery{Key: occurrence, Kind: DeliverSchedule, From: "scheduler", Display: sc.Name, Text: text, Ceiling: "workspace-write"})
}
