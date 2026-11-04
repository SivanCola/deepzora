package boot

import (
	"context"
	"deepzora/internal/control"
	"deepzora/internal/tool"
	"deepzora/internal/tool/builtin"
)

func bindTeamSecretEnvironment(reg *tool.Registry, seat control.TeamSeat) {
	source, ok := seat.(interface {
		CommandSecretEnvironment(context.Context, string, string, string) ([]string, error)
	})
	if !ok {
		return
	}
	for _, name := range []string{"bash", "pwsh"} {
		if current, ok := reg.Get(name); ok {
			if bound, ok := builtin.BindTeamSecretEnvironment(current, source.CommandSecretEnvironment); ok {
				reg.Add(bound)
			}
		}
	}
}
