package builtin

import (
	"context"
	"deepzora/internal/sandbox"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestBashTeamSecretEnvironmentIsPerInvocationAndBypassesHostTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process fixture; native PowerShell parser has separate coverage")
	}
	t.Setenv("DEEPZORA_SECRET_UNGRANTED", "must-not-inherit")
	calls := 0
	b := bash{shell: sandbox.Shell{Kind: sandbox.ShellBash, Path: "/bin/sh"}, teamSecrets: func(_ context.Context, _, _, command string) ([]string, error) {
		calls++
		if calls == 1 && strings.Contains(command, "$DEEPZORA_SECRET_DEPLOY") {
			return []string{"DEEPZORA_SECRET_DEPLOY=invocation-only"}, nil
		}
		return nil, nil
	}}
	ctx := sandbox.WithPermissionPreset(t.Context(), "danger-full-access")
	terminal := &fakeTerminal{out: "must-not-use-terminal", ok: true}
	b.terminal = terminal
	args, _ := json.Marshal(map[string]string{"command": `test -z "$DEEPZORA_SECRET_UNGRANTED" && test "$DEEPZORA_SECRET_DEPLOY" = invocation-only && printf granted`})
	out, err := b.Execute(ctx, args)
	if err != nil || out != "granted" {
		t.Fatalf("isolated command: %q %v", out, err)
	}
	if len(terminal.called) != 0 {
		t.Fatal("secret reached a host-owned terminal")
	}
	b.terminal = nil
	args, _ = json.Marshal(map[string]string{"command": `test -z "$DEEPZORA_SECRET_DEPLOY" && test -z "$DEEPZORA_SECRET_UNGRANTED" && printf clean`})
	out, err = b.Execute(ctx, args)
	if err != nil || out != "clean" || calls != 2 {
		t.Fatalf("value survived invocation: %q calls=%d err=%v", out, calls, err)
	}
}
