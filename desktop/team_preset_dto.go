package main

import (
	"time"

	"deepzora/internal/agentpreset"
	"deepzora/internal/teamroster"
)

// RestrictTools makes nil vs an explicitly empty allowlist unambiguous on the
// generated Desktop wire. Tools is always an array, including unrestricted.
type TeamPresetDefinition struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	Body                  string   `json:"body"`
	Icon                  string   `json:"icon"`
	IconImage             string   `json:"iconImage"`
	IconColor             string   `json:"iconColor"`
	KeepCoding            bool     `json:"keepCoding"`
	Complete              bool     `json:"complete"`
	IncludeRuntimeContext bool     `json:"includeRuntimeContext"`
	RestrictTools         bool     `json:"restrictTools"`
	Tools                 []string `json:"tools"`
}

func (p TeamPresetDefinition) definition() agentpreset.Preset {
	var tools []string
	if p.RestrictTools {
		tools = append([]string{}, p.Tools...)
	}
	return agentpreset.Preset{ID: p.ID, Name: p.Name, Description: p.Description, Body: p.Body, Icon: p.Icon, IconImage: p.IconImage, IconColor: p.IconColor, KeepCoding: p.KeepCoding, Complete: p.Complete, IncludeRuntimeContext: p.IncludeRuntimeContext, Tools: tools}
}

func teamPresetView(p *agentpreset.Preset) *TeamPresetDefinition {
	if p == nil {
		return nil
	}
	return &TeamPresetDefinition{ID: p.ID, Name: p.Name, Description: p.Description, Body: p.Body, Icon: p.Icon, IconImage: p.IconImage, IconColor: p.IconColor, KeepCoding: p.KeepCoding, Complete: p.Complete, IncludeRuntimeContext: p.IncludeRuntimeContext, RestrictTools: p.Tools != nil, Tools: append([]string{}, p.Tools...)}
}

type TeamAgentView struct {
	TeamAgentSettings
	ID          string    `json:"id"`
	SessionPath string    `json:"sessionPath"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	Archived    bool      `json:"archived,omitempty"`
}

type TeamRosterView struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Revision      uint64                     `json:"revision"`
	Agents        []TeamAgentView            `json:"agents"`
	Groups        []teamroster.Group         `json:"groups"`
	RetiredAgents []TeamAgentView            `json:"retiredAgents"`
	RetiredGroups []teamroster.Group         `json:"retiredGroups"`
	Prefs         teamroster.TeamPreferences `json:"prefs"`
}

func teamRosterView(r teamroster.Roster) TeamRosterView {
	out := TeamRosterView{SchemaVersion: r.SchemaVersion, Revision: r.Revision, Agents: []TeamAgentView{}, Groups: append([]teamroster.Group{}, r.Groups...), RetiredAgents: []TeamAgentView{}, RetiredGroups: append([]teamroster.Group{}, r.RetiredGroups...), Prefs: r.Prefs}
	for _, a := range r.Agents {
		out.Agents = append(out.Agents, teamAgentView(a))
	}
	for _, a := range r.RetiredAgents {
		out.RetiredAgents = append(out.RetiredAgents, teamAgentView(a))
	}
	return out
}

func teamAgentView(a teamroster.Agent) TeamAgentView {
	return TeamAgentView{ID: a.ID, SessionPath: a.SessionPath, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt, Archived: a.Archived, TeamAgentSettings: TeamAgentSettings{Name: a.Name, Role: a.Role, Instructions: a.Instructions, Avatar: a.Avatar, InitialModel: a.InitialModel, Folder: a.Folder, Permission: a.Permission, Main: a.Main, Pinned: a.Pinned, Hidden: a.Hidden, Preset: teamPresetView(a.Preset)}}
}
