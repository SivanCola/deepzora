package boot

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/internal/config"
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
)

// A fresh CLI/ACP process must protect Desktop's store without any Desktop
// initializer or an earlier test having registered the path or values.
func TestBuildBotCredentialsInIndependentProcess(t *testing.T) {
	const child = "DEEPZORA_TEST_BOT_CREDENTIAL_CHILD"
	if os.Getenv(child) != "1" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestBuildBotCredentialsInIndependentProcess$", "-test.v")
		cmd.Env = append(os.Environ(), child+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("independent runtime: %v\n%s", err, out)
		}
		return
	}
	opts, _ := presetEffectSetup(t)
	dir := filepath.Join(config.DeepzoraHomeDir(), "bot-channels")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, value string) {
		t.Helper()
		data, _ := json.Marshal([]map[string]string{{"id": "cli-boundary", "secret": value}})
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	active, backup := "cli-bot-active-disposable-9264", "cli-bot-backup-disposable-8263"
	write("connections.json", active)
	write("connections.pre-members.json", backup)
	first := buildPresetEffect(t, opts)
	second := buildPresetEffect(t, opts)
	reader, ok := first.Assembly.Registry.Get("read_file")
	if !ok {
		t.Fatal("runtime did not install read_file")
	}
	for _, protect := range []bool{true, false} {
		secrets.SetProtectSensitiveFiles(protect)
		for _, name := range []string{"connections.json", "connections.pre-members.json", ".connections-interrupted"} {
			args, _ := json.Marshal(map[string]string{"path": filepath.Join(dir, name)})
			if _, err := reader.Execute(t.Context(), args); err == nil {
				t.Fatalf("runtime reader exposed %s (protect=%v)", name, protect)
			}
		}
	}
	assertMasked := func(values ...string) {
		t.Helper()
		input := strings.Join(values, " ")
		projected := provider.MaskOutbound(provider.Request{Messages: []provider.Message{{Role: provider.RoleTool, Content: input}}}, provider.SignedProjection{})
		for _, value := range values {
			if strings.Contains(projected.Messages[0].Content, value) {
				t.Fatal("independent runtime exposed a Bot value at the provider boundary")
			}
		}
	}
	assertMasked(active, backup)
	first.Controller.Close()
	<-first.Controller.Closed()
	// A different process may commit a new value after the first runtime exits.
	rotated := "cli-bot-rotated-disposable-7262"
	write("connections.json", rotated)
	assertMasked(active, backup, rotated)
	if err := os.WriteFile(filepath.Join(dir, "connections.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := provider.CheckSecretSignedReplay(provider.Request{}); err == nil {
		t.Fatal("remaining runtime stopped sealing unreadable credential storage")
	}
	second.Controller.Close()
	<-second.Controller.Closed()
	if err := provider.CheckSecretSignedReplay(provider.Request{}); err != nil {
		t.Fatalf("last runtime retained the credential watcher: %v", err)
	}
	opts.Model = "missing-bot-regression-model"
	if result, err := BuildRuntime(t.Context(), opts); err == nil {
		result.Controller.Close()
		t.Fatal("invalid model unexpectedly built")
	}
	if err := provider.CheckSecretSignedReplay(provider.Request{}); err != nil {
		t.Fatalf("failed runtime build retained the credential watcher: %v", err)
	}
}
