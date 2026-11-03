package main

import (
	"context"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type TeamGroupPostRequest struct {
	GroupID   string   `json:"groupId"`
	RequestID string   `json:"requestId"`
	Text      string   `json:"text"`
	Mentions  []string `json:"mentions"`
	Everyone  bool     `json:"everyone"`
}
type TeamGroupPostResult struct {
	Outcome string                `json:"outcome"`
	Reason  string                `json:"reason,omitempty"`
	Message string                `json:"message,omitempty"`
	Line    *teamroster.GroupLine `json:"line,omitempty"`
}

func (a *App) SendTeamGroupMessage(req TeamGroupPostRequest) (TeamGroupPostResult, error) {
	result := TeamGroupPostResult{Outcome: "rejected"}
	if len(req.RequestID) < 8 || len(req.RequestID) > 128 {
		result.Reason = "invalid"
		result.Message = "A stable submission request identity is required"
		return result, nil
	}
	err := a.withTeamWriter(func(ctx context.Context, s *team.Service) error {
		r, err := desktopTeamStore().Load()
		if err != nil {
			return err
		}
		group, ok := r.Group(req.GroupID)
		if !ok {
			return teamroster.ErrUnknownGroup
		}
		for _, id := range req.Mentions {
			if _, ok := r.Agent(id); !ok {
				return teamroster.ErrUnknownAgent
			}
			if !group.IsMember(id) {
				return teamroster.ErrNotMember
			}
		}
		line, err := s.UserPostWithID(ctx, req.GroupID, req.Text, req.Mentions, req.Everyone, req.RequestID)
		if line.Seq > 0 {
			result.Outcome = "accepted"
			result.Line = &line
		}
		return err
	})
	if err != nil {
		result.Reason = teamMutationReason(err)
		result.Message = err.Error()
	}
	return result, nil
}

func (a *App) withTeamWriter(fn func(context.Context, *team.Service) error) error {
	h, err := a.localTeamHost()
	if err != nil {
		return err
	}
	if err := h.Claim(a.bootContext()); err != nil {
		return err
	}
	return h.WithWriter(a.bootContext(), fn)
}

func (a *App) ContinueTeamGroup(groupID string) error {
	return a.withTeamWriter(func(ctx context.Context, s *team.Service) error { return s.Continue(ctx, groupID) })
}

// StopTeamSeat affects execution only, never persistent member permissions.
func (a *App) StopTeamSeat(agentID, groupID string) error {
	return a.withTeamWriter(func(ctx context.Context, s *team.Service) error {
		if groupID != "" {
			return s.StopMember(ctx, groupID, agentID)
		}
		r, err := desktopTeamStore().Load()
		if err != nil {
			return err
		}
		if _, ok := r.Agent(agentID); !ok {
			return teamroster.ErrUnknownAgent
		}
		s.Stop(agentID)
		return nil
	})
}

func (a *App) StopTeamGroup(groupID string) error {
	return a.withTeamWriter(func(ctx context.Context, s *team.Service) error {
		r, err := desktopTeamStore().Load()
		if err != nil {
			return err
		}
		g, ok := r.Group(groupID)
		if !ok {
			return teamroster.ErrUnknownGroup
		}
		for _, id := range g.Members {
			if err := s.StopMember(ctx, groupID, id); err != nil {
				return err
			}
		}
		return nil
	})
}
