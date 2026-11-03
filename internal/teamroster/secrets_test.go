package teamroster

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTeamSecretMetadataCASRevocationAndRetirement(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	sc := TeamSecret{Name: "DEPLOY", Slot: "ts-original", Agents: []string{"a-0000000000000001"}}
	book, err := store.MutateTeamSecrets(t.Context(), 0, "create", "digest", func(b *TeamSecretBook) error { b.Secrets = append(b.Secrets, sc); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MutateTeamSecrets(t.Context(), 0, "create", "digest", func(*TeamSecretBook) error { t.Fatal("retry mutated twice"); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MutateTeamSecrets(t.Context(), 0, "different", "digest", func(*TeamSecretBook) error { return nil }); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale overwrite: %v", err)
	}
	book, err = store.MutateTeamSecrets(t.Context(), book.Revision, "rotate", "digest2", func(b *TeamSecretBook) error {
		b.Retire(b.Secrets[0], time.Now())
		b.Secrets[0].Slot = "ts-replacement"
		b.Secrets[0].Agents = []string{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Secrets[0].Agents) != 0 || book.Retired[0].Slot != "ts-original" {
		t.Fatalf("rotation lost explicit revoke/retirement: %+v", book)
	}
	raw, err := os.ReadFile(filepath.Join(store.Dir, SecretFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"value"`) {
		t.Fatal("metadata can contain plaintext")
	}
	if err := os.WriteFile(filepath.Join(store.Dir, SecretFile), []byte(`{"schemaVersion":999}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MutateTeamSecrets(t.Context(), 0, "future", "digest", func(*TeamSecretBook) error { t.Fatal("future schema overwritten"); return nil }); !errors.Is(err, ErrNewerSchema) {
		t.Fatal(err)
	}
}
