package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"deepzora/internal/evidence"
	"deepzora/internal/tool"
)

// parseToolCall resolves the canonical tool and rejects ambiguity/unknown tools.
func (a *Agent) parseToolCall(ctx context.Context, turn *turnRuntime, plan *toolCallPlan) (toolOutcome, bool) {
	t, canonicalName, ambiguous := a.svc.tools.ResolveCall(plan.call.Name)
	if len(ambiguous) > 0 {
		msg := fmt.Sprintf("ambiguous MCP tool reference %q; use one of: %s", plan.call.Name, strings.Join(ambiguous, ", "))
		return toolOutcome{
			output: "error: " + msg,
			errMsg: msg,
		}, true
	}
	if t == nil {
		switch plan.call.Name {
		case "task", "read_only_task", "parallel_tasks", "fleet":
			return toolOutcome{output: "error: this delegation tool is retired; use spawn_agents with independent tasks", errMsg: "retired delegation tool"}, true
		case "complete_subtask":
			return toolOutcome{output: "error: complete_subtask is retired; return a final text answer", errMsg: "retired completion tool"}, true
		}
		if server, ok := completedMCPConnect(a.svc.tools, plan.call.Name); ok {
			return toolOutcome{
				output: fmt.Sprintf("MCP server %q is connected; its real tools are now available", server),
			}, true
		}
		return toolOutcome{
			output: fmt.Sprintf("error: unknown tool %q", plan.call.Name),
			errMsg: fmt.Sprintf("unknown tool %q", plan.call.Name),
		}, true
	}
	t = spawnToolForContext(ctx, t, json.RawMessage(plan.call.Arguments))
	plan.tool = t
	plan.canonicalName = canonicalName
	plan.permName = canonicalName
	plan.permArgs = json.RawMessage(plan.call.Arguments)
	plan.execTool = t
	plan.execArgs = json.RawMessage(plan.call.Arguments)
	plan.evidenceName = canonicalName
	plan.evidenceArgs = json.RawMessage(plan.call.Arguments)
	plan.readOnly = t.ReadOnly()
	if canonicalName == "read_file" {
		if out, blocked := a.resolveReadCursor(plan); blocked {
			return out, true
		}
	}
	if tool.IsShellToolName(canonicalName) {
		var permissionReader bool
		plan.effects, permissionReader = evidence.ClassifyBashToolCall(plan.execArgs)
		if permissionReader {
			// Carry the resolved read-only classification without changing the schema.
			plan.readOnly = true
			plan.resolvedMeta = &tool.ResolvedCall{TargetName: canonicalName, ReadOnly: true}
		}
	} else {
		plan.effects = evidence.ClassifyToolCall(plan.evidenceName, plan.evidenceArgs, plan.readOnly)
	}
	return toolOutcome{}, false
}
