package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"deepzora/internal/agentpreset"
	"deepzora/internal/boot"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type TeamView struct {
	Roster   teamroster.Roster `json:"roster"`
	Writable bool              `json:"writable"`
	Seats    []TeamSeatView    `json:"seats"`
}

type TeamSeatView struct {
	AgentID    string         `json:"agentId"`
	GroupID    string         `json:"groupId,omitempty"`
	TabID      string         `json:"tabId"`
	SessionID  string         `json:"sessionId"`
	Generation uint64         `json:"generation"`
	Work       ActiveWorkView `json:"work"`
}

type TeamAgentSettings struct {
	Name         string              `json:"name"`
	Role         string              `json:"role"`
	Instructions string              `json:"instructions"`
	Avatar       teamroster.Avatar   `json:"avatar"`
	InitialModel string              `json:"initialModel"`
	Folder       string              `json:"folder"`
	Permission   string              `json:"permission"`
	Main         bool                `json:"main"`
	Pinned       bool                `json:"pinned"`
	Hidden       bool                `json:"hidden"`
	Preset       *agentpreset.Preset `json:"preset,omitempty"`
}

type SaveTeamAgentRequest struct {
	ID               string            `json:"id"`
	ExpectedRevision uint64            `json:"expectedRevision"`
	RequestID        string            `json:"requestId"`
	PresetID         *string           `json:"presetId,omitempty"`
	Settings         TeamAgentSettings `json:"settings"`
}

type TeamGroupSettings struct {
	Name       string          `json:"name"`
	Notice     string          `json:"notice"`
	Members    []string        `json:"members"`
	Admin      string          `json:"admin"`
	Mode       teamroster.Mode `json:"mode"`
	RelayLimit int             `json:"relayLimit"`
	Folder     string          `json:"folder"`
	Pinned     bool            `json:"pinned"`
	Hidden     bool            `json:"hidden"`
	Archived   bool            `json:"archived"`
}

type SaveTeamGroupRequest struct {
	ID               string            `json:"id"`
	ExpectedRevision uint64            `json:"expectedRevision"`
	RequestID        string            `json:"requestId"`
	Settings         TeamGroupSettings `json:"settings"`
}

type TeamMutationResult struct {
	Outcome  string   `json:"outcome"`
	Reason   string   `json:"reason,omitempty"`
	Message  string   `json:"message,omitempty"`
	TargetID string   `json:"targetId,omitempty"`
	View     TeamView `json:"view"`
}

func (a *App) GetTeam() (TeamView, error) {
	r, err := desktopTeamStore().Load()
	if err != nil {
		return TeamView{}, err
	}
	h, err := a.localTeamHost()
	if err != nil {
		return TeamView{}, err
	}
	view := TeamView{Roster: r, Writable: h.Ready(), Seats: []TeamSeatView{}}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.runtimeTabsLocked() {
		if tab.teamSeat == nil || tab.Ctrl == nil {
			continue
		}
		_, ref := tab.teamSeat.Team()
		view.Seats = append(view.Seats, TeamSeatView{AgentID: ref.Agent, GroupID: ref.Group, TabID: tab.ID, SessionID: tab.SessionID, Generation: tab.SessionGeneration, Work: activeWorkForController(tab.Ctrl)})
	}
	return view, nil
}

func teamMutationDigest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func teamMutationReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, teamroster.ErrConflict):
		return "conflict"
	case errors.Is(err, teamroster.ErrReadOnly):
		return "permission_denied"
	case errors.Is(err, teamroster.ErrUnknownAgent), errors.Is(err, teamroster.ErrUnknownGroup), errors.Is(err, teamroster.ErrNotMember):
		return "target_invalid"
	case errors.Is(err, errTeamBusy):
		return "busy"
	case errors.Is(err, teamroster.ErrRequestReused):
		return "request_reused"
	default:
		var field *teamroster.FieldError
		if errors.As(err, &field) || errors.Is(err, teamroster.ErrLastMain) || errors.Is(err, teamroster.ErrAgentCapacity) || errors.Is(err, teamroster.ErrGroupCapacity) || errors.Is(err, teamroster.ErrGroupSize) || errors.Is(err, teamroster.ErrInvalid) || errors.Is(err, agentpreset.ErrNotFound) {
			return "invalid"
		}
		return "storage_failed"
	}
}

var errTeamBusy = errors.New("team target has active work or unconsumed exchanges")

func (a *App) mutateTeam(revision uint64, key string, request any, mutate func(context.Context, *teamroster.Roster) (string, error)) (TeamMutationResult, error) {
	var resultTarget string
	h, err := a.localTeamHost()
	if err != nil {
		return TeamMutationResult{}, err
	}
	if err = h.Claim(a.bootContext()); err == nil {
		err = h.WithWriter(a.bootContext(), func(ctx context.Context, _ *team.Service) error {
			_, target, err := desktopTeamStore().Mutate(ctx, revision, key, teamMutationDigest(request), func(r *teamroster.Roster) (string, error) { return mutate(ctx, r) })
			resultTarget = target
			return err
		})
	}
	view, readErr := a.GetTeam()
	if readErr != nil {
		return TeamMutationResult{}, readErr
	}
	result := TeamMutationResult{Outcome: "applied", TargetID: resultTarget, View: view}
	if err != nil {
		result.Outcome = "rejected"
		result.Reason = teamMutationReason(err)
		result.Message = err.Error()
	}
	if err == nil {
		a.emitTeamChanged()
	}
	return result, nil
}

// SaveTeamAgent accepts user-owned settings only; session bindings, creation
// attribution and source-preset paths are always retained by the host.
func (a *App) SaveTeamAgent(req SaveTeamAgentRequest) (TeamMutationResult, error) {
	return a.mutateTeam(req.ExpectedRevision, req.RequestID, req, func(ctx context.Context, r *teamroster.Roster) (string, error) {
		var member *teamroster.Agent
		if req.ID != "" {
			var ok bool
			member, ok = r.Agent(req.ID)
			if !ok {
				return "", teamroster.ErrUnknownAgent
			}
		} else {
			created := r.AddAgent(teamroster.Agent{CreatedBy: teamroster.ByUser, CreatedAt: time.Now().UTC()})
			member, _ = r.Agent(created.ID)
		}
		settings := req.Settings
		preset := settings.Preset
		if req.PresetID != nil {
			resolved, err := agentpreset.Resolve(boot.AgentPresetDirs(settings.Folder), *req.PresetID)
			if err != nil {
				return "", err
			}
			preset = &resolved
		}
		if preset != nil {
			// Round-trip through the existing authoring validator. Source paths
			// and scopes are not executable authority and are not copied.
			parsed, err := agentpreset.Parse(preset.ID, agentpreset.Render(*preset))
			if preset.IsDefault() {
				parsed, err = agentpreset.Default(), nil
			}
			if err != nil {
				return "", err
			}
			member.Preset = &parsed
		}
		member.Name, member.Role, member.Instructions = settings.Name, settings.Role, settings.Instructions
		member.Avatar, member.InitialModel = settings.Avatar, settings.InitialModel
		member.Folder, member.Permission = settings.Folder, settings.Permission
		member.Main, member.Pinned, member.Hidden = settings.Main, settings.Pinned, settings.Hidden
		if req.ID == "" && len(r.Agents) == 1 {
			member.Main = true
		}
		return member.ID, nil
	})
}

func (a *App) SaveTeamGroup(req SaveTeamGroupRequest) (TeamMutationResult, error) {
	return a.mutateTeam(req.ExpectedRevision, req.RequestID, req, func(ctx context.Context, r *teamroster.Roster) (string, error) {
		var group *teamroster.Group
		if req.ID != "" {
			var ok bool
			group, ok = r.Group(req.ID)
			if !ok {
				return "", teamroster.ErrUnknownGroup
			}
		} else {
			created := r.AddGroup(teamroster.Group{CreatedBy: teamroster.ByUser, CreatedAt: time.Now().UTC()})
			group, _ = r.Group(created.ID)
		}
		s := req.Settings
		group.Name, group.Notice, group.Members = s.Name, s.Notice, slices.Clone(s.Members)
		group.Admin, group.Mode, group.RelayLimit = s.Admin, s.Mode, s.RelayLimit
		group.Folder, group.Pinned, group.Hidden, group.Archived = s.Folder, s.Pinned, s.Hidden, s.Archived
		for id := range group.MemberSessions {
			if !group.IsMember(id) {
				delete(group.MemberSessions, id)
			}
		}
		return group.ID, nil
	})
}

func (a *App) OpenTeamAgent(agentID, groupID string) (TabMeta, error) {
	h, err := a.localTeamHost()
	if err != nil {
		return TabMeta{}, err
	}
	if err := h.Claim(a.bootContext()); err != nil {
		return TabMeta{}, err
	}
	ref := team.SeatRef{Agent: agentID, Group: groupID}
	if _, err := h.Ensure(a.bootContext(), ref); err != nil {
		return TabMeta{}, err
	}
	r, err := desktopTeamStore().Load()
	if err != nil {
		return TabMeta{}, err
	}
	path, err := teamSessionPath(&r, ref)
	if err != nil {
		return TabMeta{}, err
	}
	meta, err := a.activateTopic("", "", "", path)
	if err == nil {
		h.SetVisible(ref, true)
	}
	return meta, err
}
