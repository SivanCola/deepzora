package agent

import (
	"context"
	"testing"

	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/tool"
	"deepzora/internal/tool/builtin"
)

type ceilingTeamDirectory struct {
	tool.TeamDirectory
	calls   int
	ceiling string
}

func (d *ceilingTeamDirectory) record(ctx context.Context) {
	d.calls++
	d.ceiling = tool.ExecutionCeiling(ctx)
}
func (d *ceilingTeamDirectory) MessageAgent(ctx context.Context, _ tool.TeamMessage) (tool.TeamReceipt, error) {
	d.record(ctx)
	return tool.TeamReceipt{Exchange: "reply", ToName: "Ben"}, nil
}
func (d *ceilingTeamDirectory) PassTurn(ctx context.Context) error { d.record(ctx); return nil }
func (d *ceilingTeamDirectory) HandTurn(ctx context.Context, _ []string) error {
	d.record(ctx)
	return nil
}
func (d *ceilingTeamDirectory) PostToGroup(ctx context.Context, _, _ string, _ []string) error {
	d.record(ctx)
	return nil
}

func TestReadOnlyTeamCeilingAllowsOnlyTopLevelCoordination(t *testing.T) {
	for _, tc := range []struct {
		name, args string
		allowed    bool
	}{
		{"message_agent", `{"to":"ben","message":"read this"}`, true},
		{"pass_turn", `{}`, true},
		{"hand_turn", `{"to":["ben"]}`, true},
		{"post_to_group", `{"group":"launch","message":"read this"}`, true},
		{"create_agent", `{"name":"new"}`, false},
		{"update_agent", `{"agent":"ben","instructions":"change"}`, false},
		{"stop_agent", `{"agent":"ben"}`, false},
	} {
		for _, proxy := range []bool{false, true} {
			for _, identity := range []string{"member", "child", "strict-reader"} {
				t.Run(tc.name+"/"+identity+map[bool]string{false: "/direct", true: "/proxy"}[proxy], func(t *testing.T) {
					d := &ceilingTeamDirectory{}
					link := &tool.TeamLink{}
					link.Bind(d)
					reg := tool.NewRegistry()
					var target tool.Tool
					for _, v := range builtin.TeamTools(link) {
						reg.Add(v)
						if v.Name() == tc.name {
							target = v
						}
					}
					if target == nil {
						t.Fatal("tool missing")
					}
					call := provider.ToolCall{ID: "coordination", Name: tc.name, Arguments: tc.args}
					if proxy {
						reg.Add(readOnlyBoundaryProxy{resolved: tool.ResolvedCall{ProxyAction: "call", Target: target, TargetName: tc.name, Args: []byte(tc.args), ReadOnly: target.ReadOnly()}})
						call.Name = "use_capability"
					}
					a := New(nil, reg, NewSession("sys"), Options{ReadOnlyExecution: identity == "strict-reader"}, event.Discard)
					ctx := tool.WithExecutionCeiling(t.Context(), "read-only")
					if identity == "child" {
						ctx = WithSubagentDepth(ctx, 1)
					}
					out := a.executeOne(ctx, &a.turn, call)
					allowed := identity == "member" && tc.allowed
					if allowed {
						if out.blocked || d.calls != 1 || d.ceiling != "read-only" {
							t.Fatalf("coordination blocked or widened: %+v calls=%d ceiling=%s", out, d.calls, d.ceiling)
						}
					} else if !out.blocked || d.calls != 0 {
						t.Fatalf("restricted tool executed: %+v calls=%d", out, d.calls)
					}
				})
			}
		}
	}
}
