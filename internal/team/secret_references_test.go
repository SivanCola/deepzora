package team

import (
	"os/exec"
	"reflect"
	"testing"
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
