package agent

import (
	"encoding/json"
	"testing"

	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/tool"
)

type escalatingTeamWriter struct{ readOnlyBoundaryTarget }

func (escalatingTeamWriter) DeclareWriteAccess(json.RawMessage) (tool.WriteAccessDeclaration, error) {
	return tool.WriteAccessDeclaration{RequestedPreset: "danger-full-access"}, nil
}

func TestDelegatedWorkspaceCeilingBlocksChildShellEscalation(t *testing.T) {
	reg := tool.NewRegistry()
	calls := 0
	reg.Add(escalatingTeamWriter{readOnlyBoundaryTarget{name: "writer", calls: &calls}})
	a := New(nil, reg, NewSession("sys"), Options{}, event.Discard)
	out := a.executeOne(tool.WithExecutionCeiling(t.Context(), "workspace-write"), &a.turn, provider.ToolCall{ID: "escalation", Name: "writer", Arguments: `{}`})
	if !out.blocked || calls != 0 {
		t.Fatalf("headless child escalated its preset: %+v calls=%d", out, calls)
	}
}

func TestDelegatedReadOnlyCeilingBlocksDirectAndDeferredWriters(t *testing.T) {
	for _, proxy := range []bool{false, true} {
		reg := tool.NewRegistry()
		calls := 0
		writer := readOnlyBoundaryTarget{name: "writer", calls: &calls}
		reg.Add(writer)
		call := provider.ToolCall{ID: "team-write", Name: "writer", Arguments: `{}`}
		if proxy {
			reg.Add(readOnlyBoundaryProxy{resolved: tool.ResolvedCall{ProxyAction: "call", Target: writer, TargetName: "writer"}})
			call.Name = "use_capability"
		}
		a := New(nil, reg, NewSession("sys"), Options{}, event.Discard)
		ctx := tool.WithExecutionCeiling(t.Context(), "read-only")
		out := a.executeOne(ctx, &a.turn, call)
		if !out.blocked || calls != 0 {
			t.Fatalf("proxy=%v: inherited ceiling escaped: %+v, calls=%d", proxy, out, calls)
		}
		out = a.executeOne(t.Context(), &a.turn, call)
		if out.blocked || calls != 1 {
			t.Fatalf("proxy=%v: cap leaked into ordinary execution: %+v calls=%d", proxy, out, calls)
		}
	}
}
