package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deepzora/internal/provider"
	"deepzora/internal/sandbox"
	"deepzora/internal/tool"
	_ "deepzora/internal/tool/builtin"
)

// The real shell must exit before a cancelled writer can hand its slot to the
// next child. The cleanup barrier makes early release observable without racing
// a short-lived process. This uses the native configured shell on Windows too.
func TestSpawnShellCancellationKeepsWriterSlotThroughCleanup(t *testing.T) {
	shell, ok := tool.LookupBuiltin("bash")
	if !ok {
		t.Fatal("shell is not registered")
	}
	marker := filepath.Join(t.TempDir(), "started")
	quote := "'" + strings.ReplaceAll(marker, "'", "'\\''") + "'"
	command := "printf ready > " + quote + "; sleep 120"
	if sandbox.ResolveShell("", "", nil).Kind == sandbox.ShellPowerShell {
		quote = "'" + strings.ReplaceAll(marker, "'", "''") + "'"
		command = "Set-Content -LiteralPath " + quote + " -Value ready; Start-Sleep -Seconds 120"
	}
	args, _ := json.Marshal(map[string]any{"command": command, "description": "Wait for child cancellation test"})
	cancelShell := make(chan context.CancelFunc, 1)
	shellExited := make(chan struct{})
	cleanup := make(chan struct{})
	nextStarted := make(chan struct{})
	p := &spawnProvider{run: func(ctx context.Context, req provider.Request) error {
		if strings.Contains(lastUser(req), "shell writer") {
			shellCtx, cancel := context.WithCancel(sandbox.WithPermissionPreset(ctx, "danger-full-access"))
			defer cancel()
			cancelShell <- cancel
			_, err := shell.Execute(shellCtx, args)
			close(shellExited)
			<-cleanup
			return err
		}
		close(nextStarted)
		return nil
	}}
	s := spawnFixture(t, p)
	ctx, cancelBatch := context.WithCancel(testTaskContext())
	defer cancelBatch()
	done := make(chan error, 1)
	go func() {
		_, err := s.Execute(ctx, []byte(`{"tasks":[{"title":"first","task":"shell writer"},{"title":"next","task":"next writer"}]}`))
		done <- err
	}()
	defer close(cleanup)
	var cancel context.CancelFunc
	select {
	case cancel = <-cancelShell:
		defer cancel()
	case <-time.After(10 * time.Second):
		t.Fatal("shell child was not admitted")
	}
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
waitStart:
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case <-shellExited:
			t.Fatal("shell exited before its ready marker")
		case <-deadline.C:
			t.Fatal("shell never wrote its ready marker")
		case <-tick.C:
			continue waitStart
		}
	}
	cancel()
	select {
	case <-shellExited:
	case <-time.After(40 * time.Second):
		t.Fatal("cancelled shell did not actually exit")
	}
	select {
	case <-nextStarted:
		t.Fatal("next writer started before cancelled child cleanup")
	default:
	}
	if release, err := s.runner.scheduler.ReserveParentWrite(WritePathSet{WholeWorkspace: true}); err == nil {
		release()
		t.Fatal("parent write bypassed child cleanup")
	}
	// A send releases the barrier; the deferred close handles failure paths.
	cleanup <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("independent next writer did not finish")
	}
	select {
	case <-nextStarted:
	default:
		t.Fatal("cancelled child prevented its independent sibling")
	}
}
