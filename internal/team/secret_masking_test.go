package team

import (
	"deepzora/internal/config"
	"deepzora/internal/secrets"
	"deepzora/internal/teamroster"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretSnapshotReloadsSameLengthSlotWithRestoredMtimeAndFailsClosed(t *testing.T) {
	store := teamroster.Store{Dir: t.TempDir()}
	slots := config.TeamSecretSlots{Dir: t.TempDir()}
	slot, err := slots.Save(t.Context(), "same-size-value-aaa")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.MutateTeamSecrets(t.Context(), 0, "save-mask-source", "digest", func(book *teamroster.TeamSecretBook) error {
		book.Secrets = append(book.Secrets, teamroster.TeamSecret{Name: "REFRESH_TEST", Slot: slot, Agents: []string{}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	release := InstallSecretValues(store, slots)
	defer release()
	if got := secrets.Masker().String("same-size-value-aaa"); got != "[secret:REFRESH_TEST]" {
		t.Fatal(got)
	}
	path := filepath.Join(slots.Dir, slot)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("same-size-value-bbb"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got := secrets.Masker().String("same-size-value-aaa same-size-value-bbb"); strings.Contains(got, "same-size-value") {
		t.Fatalf("mtime cache leaked rotated values: %s", got)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, teamroster.SecretFile), []byte(`{"schemaVersion":999}`), 0600); err != nil {
		t.Fatal(err)
	}
	_ = secrets.Masker()
	if secrets.BoundaryError() == nil {
		t.Fatal("corrupt authoritative secret source did not seal outbound requests")
	}
}
