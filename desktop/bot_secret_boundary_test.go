package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/boot"
	"deepzora/internal/config"
	"deepzora/internal/event"
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"deepzora/internal/tool/builtin"
)

func TestBotCredentialsProtectFilesAndProjectProviderAndEventBoundaries(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	store := a.botStore()
	if err := os.MkdirAll(store.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	const active = "bot-boundary-active-disposable-9471"
	const previous = "bot-boundary-backup-disposable-2836"
	entry := botchannels.Saved{Config: botchannels.Config{ID: "boundary", Name: "Bot", Provider: "weixin", Mode: "follow", SessionID: "s", ChatID: "peer", SenderID: "peer"}, Secret: active}
	write := func(name string, entry botchannels.Saved) {
		t.Helper()
		data, _ := json.Marshal([]botchannels.Saved{entry})
		if err := os.WriteFile(filepath.Join(store.Dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("connections.json", entry)
	entry.Secret = previous
	write("connections.pre-members.json", entry)
	write(".connections-interrupted", entry)
	// Simulate a cold boot: no Store.Edit has remembered either credential.
	a.ensureTeamSecretMasking()
	t.Cleanup(a.releaseTeamSecretMasking)
	oldProtection := secrets.ProtectSensitiveFiles()
	secrets.SetProtectSensitiveFiles(false)
	t.Cleanup(func() { secrets.SetProtectSensitiveFiles(oldProtection) })
	roots := boot.RuntimeForbidReadRoots(config.Default(), t.TempDir())
	for _, name := range []string{"connections.json", "connections.pre-members.json", ".connections-interrupted", ".connections-future"} {
		path := filepath.Join(store.Dir, name)
		if !secrets.HostSecretPath(path) {
			t.Fatalf("unprotected Bot credential path %s", name)
		}
		for _, reader := range builtin.ConfineReaders(roots) {
			if reader.Name() != "read_file" {
				continue
			}
			args, _ := json.Marshal(map[string]string{"path": path})
			if _, err := reader.Execute(t.Context(), args); err == nil {
				t.Fatalf("read_file exposed %s", name)
			}
		}
	}
	input := active + " " + previous
	projected := provider.MaskOutbound(provider.Request{Messages: []provider.Message{{Role: provider.RoleTool, Content: input}}}, provider.SignedProjection{})
	for _, value := range []string{active, previous} {
		if strings.Contains(projected.Messages[0].Content, value) {
			t.Fatal("Bot credential reached provider request")
		}
	}
	var observed event.Event
	sink := event.NewSecretSink(event.FuncSink(func(e event.Event) { observed = e }))
	sink.Emit(event.Event{Kind: event.Message, Text: input})
	if strings.Contains(observed.Text, active) || strings.Contains(observed.Text, previous) {
		t.Fatal("Bot credential reached Desktop/Bot event projection")
	}
	entry.Secret = "bot-boundary-rotated-disposable-6182"
	if err := store.Edit(func([]botchannels.Saved) ([]botchannels.Saved, error) { return []botchannels.Saved{entry}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := store.Edit(func([]botchannels.Saved) ([]botchannels.Saved, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(secrets.Masker().String(entry.Secret), entry.Secret) {
		t.Fatal("rotation/deletion unmasked an in-flight credential")
	}
}

func TestBotCredentialSnapshotFailureSealsOutboundAndRecovers(t *testing.T) {
	isolateDesktopUserDirs(t)
	a := NewApp()
	a.ensureTeamSecretMasking()
	t.Cleanup(a.releaseTeamSecretMasking)
	if err := os.MkdirAll(a.botStore().Dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.botStore().Dir, "connections.pre-members.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := provider.CheckSecretSignedReplay(provider.Request{}); err == nil {
		t.Fatal("unreadable credential snapshot allowed outbound work")
	}
	if err := os.WriteFile(path, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := provider.CheckSecretSignedReplay(provider.Request{}); err != nil {
		t.Fatalf("repaired snapshot did not reopen boundary: %v", err)
	}
}
