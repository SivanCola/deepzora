package agent

import (
	"context"
	"testing"

	"deepzora/internal/capability"
	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/tool"
)

func TestTeamCeilingForcesReadOnlySpawnAcrossEntrances(t *testing.T) {
	for _, entrance := range []string{"direct", "capability", "runner"} {
		t.Run(entrance, func(t *testing.T) {
			p := &spawnProvider{run: func(ctx context.Context, req provider.Request) error {
				if tool.ExecutionCeiling(ctx) != "read-only" {
					t.Error("child lost delegated ceiling")
				}
				for _, schema := range req.Tools {
					if schema.Name == "write_file" {
						t.Error("delegated child received writer schema")
					}
				}
				return nil
			}}
			spawner := spawnFixture(t, p)
			ctx := tool.WithExecutionCeiling(testTaskContext(), "read-only")
			if entrance == "runner" {
				if _, err := spawner.runner.RunProfileSpec(ctx, ProfileExecSpec{Task: TaskSpec{Objective: "inspect"}}); err != nil {
					t.Fatal(err)
				}
			} else {
				reg := tool.NewRegistry()
				reg.Add(spawner)
				call := provider.ToolCall{ID: "team-spawn", Name: SpawnToolName, Arguments: `{"tasks":[{"title":"read","task":"inspect"}]}`}
				if entrance == "capability" {
					reg.Add(NewUseCapabilityTool(context.Background(), nil, nil, reg, capability.NewLedger(), nil, nil))
					call.Name = "use_capability"
					call.Arguments = `{"action":"call","capability_id":"tool:spawn_agents","arguments":` + call.Arguments + `}`
				}
				a := New(nil, reg, NewSession(""), Options{}, event.Discard)
				out := a.executeOne(ctx, &a.turn, call)
				if out.blocked || out.errMsg != "" {
					t.Fatalf("delegated spawn failed: %+v", out)
				}
			}
			if p.calls != 1 {
				t.Fatalf("child calls = %d", p.calls)
			}
		})
	}
}
