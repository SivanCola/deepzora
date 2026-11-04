package agent

import (
	"context"
	"encoding/json"

	"deepzora/internal/planmode"
	"deepzora/internal/tool"
)

type forceReadOnlySpawnKey struct{}

func forceReadOnlySpawn(ctx context.Context) bool {
	forced, _ := ctx.Value(forceReadOnlySpawnKey{}).(bool)
	return forced || PlanModeFromContext(ctx) || planmode.Active(ctx) || tool.ExecutionCeiling(ctx) == "read-only"
}

// A resolved read-only invocation carries its own restriction through Execute.
// A profile changed after classification can never widen this invocation.
type readOnlySpawner struct{ *Spawner }

func (readOnlySpawner) ReadOnly() bool { return true }
func (s readOnlySpawner) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return s.Spawner.Execute(context.WithValue(ctx, forceReadOnlySpawnKey{}, true), args)
}

func spawnToolForContext(ctx context.Context, target tool.Tool, args json.RawMessage) tool.Tool {
	s, ok := target.(*Spawner)
	if !ok || s.runner == nil {
		return target
	}
	if forceReadOnlySpawn(ctx) {
		return readOnlySpawner{s}
	}
	var p struct {
		Tasks []spawnTask `json:"tasks"`
	}
	if json.Unmarshal(args, &p) != nil || len(p.Tasks) == 0 || len(p.Tasks) > 8 {
		return target
	}
	for _, task := range p.Tasks {
		if task.ReadOnly {
			continue
		}
		profile, err := ResolveProfileDefinition(s.runner.profileLookup, task.Profile)
		if err != nil || !profile.ReadOnly {
			return target
		}
	}
	return readOnlySpawner{s}
}
