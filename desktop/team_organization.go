package main

import (
	"context"
	"slices"

	"deepzora/internal/teamroster"
)

type TeamTargetMutationRequest struct {
	AgentID          string `json:"agentId"`
	GroupID          string `json:"groupId"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	RequestID        string `json:"requestId"`
}

// DeleteTeamTarget retires identities instead of destroying their history.
// A member's departure retires a group that could no longer have two members.
func (a *App) DeleteTeamTarget(req TeamTargetMutationRequest) (TeamMutationResult, error) {
	return a.mutateTeam(req.ExpectedRevision, req.RequestID, req, func(_ context.Context, r *teamroster.Roster) (string, error) {
		if (req.AgentID == "") == (req.GroupID == "") {
			return "", teamroster.ErrInvalid
		}
		if err := a.requireTeamTargetIdle(req.AgentID, req.GroupID); err != nil {
			return "", err
		}
		if req.GroupID != "" {
			g, ok := r.Group(req.GroupID)
			if !ok {
				return "", teamroster.ErrUnknownGroup
			}
			retired := *g
			retired.Archived = true
			r.RetiredGroups = append(r.RetiredGroups, retired)
			r.Groups = slices.DeleteFunc(r.Groups, func(g teamroster.Group) bool { return g.ID == req.GroupID })
			return req.GroupID, nil
		}
		member, ok := r.Agent(req.AgentID)
		if !ok {
			return "", teamroster.ErrUnknownAgent
		}
		for _, group := range r.Groups {
			if group.IsMember(member.ID) {
				if err := a.requireTeamTargetIdle("", group.ID); err != nil {
					return "", err
				}
			}
		}
		r.RetiredAgents = append(r.RetiredAgents, *member)
		for i := range r.Groups {
			g := &r.Groups[i]
			if !g.IsMember(req.AgentID) {
				continue
			}
			if len(g.Members) <= teamroster.MinMembers {
				retired := *g
				retired.Archived = true
				r.RetiredGroups = append(r.RetiredGroups, retired)
				continue
			}
			g.Members = slices.DeleteFunc(g.Members, func(id string) bool { return id == req.AgentID })
			delete(g.MemberSessions, req.AgentID)
			if g.Admin == req.AgentID {
				g.Admin, g.Mode = "", teamroster.ModeEveryone
			}
		}
		r.Groups = slices.DeleteFunc(r.Groups, func(g teamroster.Group) bool {
			return slices.ContainsFunc(r.RetiredGroups, func(retired teamroster.Group) bool { return retired.ID == g.ID })
		})
		r.Agents = slices.DeleteFunc(r.Agents, func(member teamroster.Agent) bool { return member.ID == req.AgentID })
		return req.AgentID, nil
	})
}

type ReorderTeamRequest struct {
	AgentIDs         []string `json:"agentIds"`
	GroupIDs         []string `json:"groupIds"`
	ExpectedRevision uint64   `json:"expectedRevision"`
	RequestID        string   `json:"requestId"`
}

func (a *App) ReorderTeam(req ReorderTeamRequest) (TeamMutationResult, error) {
	return a.mutateTeam(req.ExpectedRevision, req.RequestID, req, func(_ context.Context, r *teamroster.Roster) (string, error) {
		if len(req.AgentIDs) != len(r.Agents) || len(req.GroupIDs) != len(r.Groups) {
			return "", teamroster.ErrConflict
		}
		agents, groups := make([]teamroster.Agent, 0, len(r.Agents)), make([]teamroster.Group, 0, len(r.Groups))
		seen := map[string]bool{}
		for _, id := range req.AgentIDs {
			member, ok := r.Agent(id)
			if !ok || seen[id] {
				return "", teamroster.ErrInvalid
			}
			seen[id] = true
			agents = append(agents, *member)
		}
		for _, id := range req.GroupIDs {
			group, ok := r.Group(id)
			if !ok || seen[id] {
				return "", teamroster.ErrInvalid
			}
			seen[id] = true
			groups = append(groups, *group)
		}
		r.Agents, r.Groups = agents, groups
		return "", nil
	})
}
