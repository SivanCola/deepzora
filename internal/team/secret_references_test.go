package team

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func TestPOSIXSecretReferencesRequireDirectASTExpansion(t *testing.T) {
	for _, test := range []struct {
		command string
		names   []string
		invalid bool
	}{
		{`curl -H "Authorization: Bearer $DEEPZORA_SECRET_DEPLOY"`, []string{"DEPLOY"}, false},
		{`echo '${DEEPZORA_SECRET_DEPLOY}'`, []string{}, false},
		{`echo ${!DEEPZORA_SECRET_DEPLOY}`, []string{}, false},
		{`echo ${DEEPZORA_SECRET_DEPLOY:-fallback}`, []string{}, false},
		{`echo $DEEPZORA_SECRET_Z $DEEPZORA_SECRET_A $DEEPZORA_SECRET_Z`, []string{"A", "Z"}, false},
		{`echo "$DEEPZORA_SECRET_DEPLOY`, nil, true},
	} {
		t.Run(test.command, func(t *testing.T) {
			got, err := ReferencedPOSIXSecrets(test.command)
			if (err != nil) != test.invalid || !reflect.DeepEqual(got, test.names) {
				t.Fatalf("references=%v err=%v", got, err)
			}
		})
	}
}

func TestPowerShellSecretParsingPreservesCallerCancellation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	names, err := ReferencedPowerShellSecrets(ctx, executable, `$env:DEEPZORA_SECRET_DEPLOY`)
	if !errors.Is(err, context.Canceled) || len(names) != 0 {
		t.Fatalf("cancelled parser granted names=%v or lost cancellation: %v", names, err)
	}
	expired, stop := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer stop()
	names, err = ReferencedPowerShellSecrets(expired, executable, `$env:DEEPZORA_SECRET_DEPLOY`)
	if !errors.Is(err, context.DeadlineExceeded) || len(names) != 0 {
		t.Fatalf("expired caller deadline granted names=%v or was extended: %v", names, err)
	}
}
func TestPowerShellSecretReferencesUseParserWithoutExecutingInput(t *testing.T) {
	executable, err := exec.LookPath("pwsh")
	if err != nil {
		executable, err = exec.LookPath("powershell.exe")
	}
	if err != nil {
		t.Skip("native PowerShell parser not installed")
	}
	for _, test := range []struct {
		script string
		names  []string
	}{
		{`Write-Output "$env:DEEPZORA_SECRET_DEPLOY"`, []string{"DEPLOY"}},
		{`Write-Output '$env:DEEPZORA_SECRET_DEPLOY'`, []string{}},
		{`Write-Output (Get-Item ('env:' + 'DEEPZORA_SECRET_DEPLOY'))`, []string{}},
		{`throw 'must not execute'; $env:DEEPZORA_SECRET_DEPLOY`, []string{"DEPLOY"}},
	} {
		got, err := ReferencedPowerShellSecrets(t.Context(), executable, test.script)
		if err != nil || !reflect.DeepEqual(got, test.names) {
			t.Fatalf("AST %q=%v %v", test.script, got, err)
		}
	}
}
