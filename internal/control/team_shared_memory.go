package control

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"deepzora/internal/event"
	"deepzora/internal/memory"
	"deepzora/internal/secrets"
	"deepzora/internal/team"
	"deepzora/internal/tool"
)

const TeamMemoryApprovalTool = "team_shared_memory"

func (d teamDirectory) ReadSharedMemory(ctx context.Context, tier, group string, args json.RawMessage) (string, error) {
	svc, caller, err := d.caller(ctx)
	if err != nil {
		return "", err
	}
	store, _, _, err := svc.SharedMemoryTarget(caller, team.SharedTier(tier), group, false)
	if err != nil {
		return "", teamRefusal(err)
	}
	return memory.NewRecallTool(store).Execute(ctx, args)
}

func (d teamDirectory) RememberShared(ctx context.Context, input tool.TeamSharedMemoryRequest) (string, error) {
	svc, caller, err := d.caller(ctx)
	if err != nil {
		return "", err
	}
	if d.c.PlanMode() || tool.ExecutionCeiling(ctx) == ToolApprovalReadOnly || d.c.currentTeamTurn().ceiling == ToolApprovalReadOnly {
		return "", tool.Refusal{Code: "team.memory_read_only", Message: "This work cannot change shared memory under its current execution restriction."}
	}
	if err := validateSharedMemoryRequest(input); err != nil {
		return "", err
	}
	store, group, revision, err := svc.SharedMemoryTarget(caller, team.SharedTier(input.Tier), input.Group, true)
	if err != nil {
		return "", teamRefusal(err)
	}
	if input.ID != "" {
		current, ok := store.Read(input.ID)
		if !ok || current.ID != input.ID || current.Revision != input.ExpectedRevision {
			return "", tool.Refusal{Code: "team.memory_conflict", Message: "This memory changed. Read it again before proposing an update."}
		}
	}
	return d.approveSharedMemory(ctx, svc, caller, input, group, revision)
}

func validateSharedMemoryRequest(input tool.TeamSharedMemoryRequest) error {
	if input.Operation != "save" && input.Operation != "archive" {
		return tool.Refusal{Code: "team.invalid", Message: "operation must be save or archive"}
	}
	if input.ID == "" && (input.Operation == "archive" || input.ExpectedRevision != 0) || input.ID != "" && input.ExpectedRevision <= 0 {
		return tool.Refusal{Code: "team.invalid", Message: "Updates require the exact memory id and revision returned by read_shared_memory."}
	}
	if input.Operation == "save" && (strings.TrimSpace(input.Description) == "" || strings.TrimSpace(input.Body) == "") {
		return tool.Refusal{Code: "team.invalid", Message: "A memory description and body are required."}
	}
	for _, text := range []string{input.Name, input.Description, input.Body} {
		if secrets.RedactCredentials(text) != text {
			return tool.Refusal{Code: "team.credential", Message: "Credentials cannot be stored in shared memory."}
		}
	}
	return nil
}

func (d teamDirectory) approveSharedMemory(ctx context.Context, svc *team.Service, caller team.Caller, input tool.TeamSharedMemoryRequest, group string, revision uint64) (string, error) {
	input.Group = group
	args, _ := json.Marshal(struct {
		Agent          string                       `json:"agent"`
		RosterRevision uint64                       `json:"rosterRevision"`
		Change         tool.TeamSharedMemoryRequest `json:"change"`
	}{caller.Agent, revision, input})
	decision, err := d.c.requestApprovalDecisionWithOptions(ctx, TeamMemoryApprovalTool, input.Tier+":"+group, args, "Apply this exact shared memory change?", approvalDecisionOptions{fresh: true, oneShotHuman: true})
	if err != nil {
		return "", err
	}
	if !decision.allow {
		return "", tool.Refusal{Code: "team.declined", Message: "The user declined this shared memory change."}
	}
	_, currentCaller, callerErr := d.caller(ctx)
	if callerErr != nil {
		return "", callerErr
	}
	if currentCaller.Agent != caller.Agent || currentCaller.Group != caller.Group || currentCaller.Turn != caller.Turn {
		return "", errTeamUnbound
	}
	if d.c.PlanMode() || tool.ExecutionCeiling(ctx) == ToolApprovalReadOnly || d.c.currentTeamTurn().ceiling == ToolApprovalReadOnly {
		return "", tool.Refusal{Code: "team.memory_read_only", Message: "The current execution restriction changed; this shared memory approval is no longer valid."}
	}
	caller.ConfigurationApproved = true
	fact := memory.Memory{ID: input.ID, Name: input.Name, Description: input.Description, Body: input.Body, Type: memory.NormalizeType(input.Type), Activation: memory.ActivationRelevant}
	if fact.Name == "" && fact.ID == "" {
		fact.Name = input.Description
	}
	saved, err := svc.CommitSharedMemory(ctx, caller, team.SharedTier(input.Tier), group, revision, fact, input.ExpectedRevision, input.Operation == "archive")
	if err != nil {
		return "", teamRefusal(err)
	}
	note := "Shared memory updated. Review the entry in Team settings."
	d.c.NotifyTeamMemory(note)
	if err := svc.RecordMemoryChange(ctx, caller.Agent, group, note); err != nil {
		return "", err
	}
	if input.Operation == "archive" {
		return "Shared memory archived.", nil
	}
	return fmt.Sprintf("Saved shared memory %s at revision %d.", saved.ID, saved.Revision), nil
}

// NotifyTeamMemory reports a host-observed result without starting a turn,
// approving a plan or arming a stopped Goal.
func (c *Controller) NotifyTeamMemory(note string) {
	c.mu.Lock()
	sink := c.sink
	c.mu.Unlock()
	if sink != nil {
		sink.Emit(event.Event{Kind: event.Notice, Text: note, Code: "team.memory_updated"})
	}
}
