package agent

import (
	"context"
	"encoding/json"
	"testing"

	"deepzora/internal/capability"
	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/tool"
)

func TestReadOnlySpawnWorksThroughDirectAndCapabilityEntrances(t *testing.T) {
	for _, proxy := range []bool{false, true} {
		p := &spawnProvider{run: func(_ context.Context, req provider.Request) error {
			for _, schema := range req.Tools {
				if schema.Name == "write_file" {
					t.Error("read-only child received writer")
				}
			}
			return nil
		}}
		spawner := spawnFixture(t, p)
		reg := tool.NewRegistry()
		reg.Add(spawner)
		call := provider.ToolCall{ID: "spawn", Name: SpawnToolName, Arguments: `{"tasks":[{"title":"read","task":"inspect","read_only":true}]}`}
		if proxy {
			reg.Add(NewUseCapabilityTool(context.Background(), nil, nil, reg, capability.NewLedger(), nil, nil))
			call.Name = "use_capability"
			call.Arguments = `{"action":"call","capability_id":"tool:spawn_agents","arguments":` + call.Arguments + `}`
		}
		a := New(nil, reg, NewSession(""), Options{ReadOnlyExecution: true}, event.Discard)
		out := a.executeOne(testTaskContext(), &a.turn, call)
		if out.blocked || out.errMsg != "" || p.calls != 1 {
			t.Fatalf("proxy=%v result=%+v calls=%d", proxy, out, p.calls)
		}
	}
}

func TestReadOnlySpawnResolutionCannotBeWidenedByProfileChange(t *testing.T) {
	p := &spawnProvider{run: func(_ context.Context, req provider.Request) error {
		for _, schema := range req.Tools {
			if schema.Name == "write_file" {
				t.Fatal("profile change widened resolved read-only call")
			}
		}
		return nil
	}}
	s := spawnFixture(t, p)
	readonly := true
	s.runner.profileLookup = func(string) (ProfileDefinition, bool) {
		return ProfileDefinition{Name: "reviewer", Body: "review", ReadOnly: readonly}, true
	}
	args := json.RawMessage(`{"tasks":[{"title":"review","task":"inspect","profile":"reviewer"}]}`)
	resolved := spawnToolForContext(testTaskContext(), s, args)
	if !resolved.ReadOnly() {
		t.Fatal("read-only profile was not classified")
	}
	readonly = false
	if _, err := resolved.Execute(testTaskContext(), args); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyProfileWithToolHooksUsesWriterSlot(t *testing.T) {
	s := spawnFixture(t, &spawnProvider{})
	s.runner.hooksForSession = func(string) ToolHooks { return &stubHooks{} }
	if !s.runner.writableRun(ProfileExecSpec{Grant: CapabilityGrant{ReadOnly: true}}) {
		t.Fatal("unknown hook effects bypassed writer admission")
	}
}
