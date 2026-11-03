package control

import (
	"context"
	"encoding/json"
	"fmt"

	"deepzora/internal/tool"
)

const TeamChangeApprovalTool = "team_change"

func (c *Controller) confirmTeamChange(ctx context.Context, name, folder string) error {
	args, _ := json.Marshal(map[string]string{"agent": name, "field": "folder", "to": folder})
	return c.confirmTeamEdit(ctx, name, args, fmt.Sprintf("Allow %s to use %s? This does not grant workspace trust.", name, folder))
}

func (c *Controller) confirmTeamConfiguration(ctx context.Context, id, name string, spec tool.TeamAgentSpec) error {
	args, _ := json.Marshal(struct {
		Agent   string             `json:"agent"`
		Changes tool.TeamAgentSpec `json:"changes"`
	}{id, spec})
	return c.confirmTeamEdit(ctx, id, args, "Apply these persistent role, instruction, model or folder changes to "+name+"?")
}

func (c *Controller) confirmTeamEdit(ctx context.Context, subject string, args json.RawMessage, reason string) error {
	r, err := c.requestApprovalDecisionWithOptions(ctx, TeamChangeApprovalTool, subject, args, reason,
		approvalDecisionOptions{fresh: true, oneShotHuman: true})
	if err != nil {
		return err
	}
	if !r.allow {
		return tool.Refusal{Code: "team.declined", Message: "The user declined the persistent team change."}
	}
	return nil
}
