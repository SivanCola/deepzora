package builtin

import (
	"context"
	"deepzora/internal/sandbox"
	"deepzora/internal/tool"
	"strings"
)

type TeamSecretEnvironment func(context.Context, string, string, string) ([]string, error)

func BindTeamSecretEnvironment(t tool.Tool, resolve TeamSecretEnvironment) (tool.Tool, bool) {
	b, ok := t.(bash)
	if !ok {
		return nil, false
	}
	b.teamSecrets = resolve
	return b, true
}
func (b bash) teamCommandEnvironment(ctx context.Context, sh sandbox.Shell, command string) ([]string, error) {
	if b.teamSecrets == nil {
		return nil, nil
	}
	return b.teamSecrets(ctx, sh.Kind.String(), sh.Path, command)
}
func stripTeamSecretEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, assignment := range env {
		key, _, _ := strings.Cut(assignment, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "DEEPZORA_SECRET_") {
			out = append(out, assignment)
		}
	}
	return out
}
