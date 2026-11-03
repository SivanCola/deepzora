package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTeamSecretSlotsNeverExportOrMutatePriorValues(t *testing.T) {
	s := TeamSecretSlots{Dir: filepath.Join(t.TempDir(), "slots")}
	t.Setenv("DEEPZORA_SECRET_TEST", "host-value")
	first, err := s.Save(t.Context(), "first-private-value")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Save(t.Context(), "replacement-private-value")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("slots are mutable")
	}
	if got, err := s.Read(first); err != nil || got != "first-private-value" {
		t.Fatalf("retired value lost: %q %v", got, err)
	}
	if os.Getenv("DEEPZORA_SECRET_TEST") != "host-value" {
		t.Fatal("Team store mutated process environment")
	}
	for _, slot := range []string{"../config.toml", filepath.Join(s.Dir, first), "ts-invalid"} {
		if _, err := s.Read(slot); err == nil {
			t.Fatalf("accepted arbitrary slot %q", slot)
		}
	}
	if err := os.Symlink(filepath.Join(s.Dir, first), filepath.Join(s.Dir, "ts-00000000000000000000000000000000")); err == nil {
		if _, err := s.Read("ts-00000000000000000000000000000000"); err == nil {
			t.Fatal("read symlink slot")
		}
	}
}

func TestPendingTeamSecretRecoveryDoesNotSweepUntrackedSlots(t *testing.T) {
	s := TeamSecretSlots{Dir: t.TempDir()}
	intents := filepath.Join(t.TempDir(), "intents")
	untracked, err := s.Save(t.Context(), "legacy-untracked-value")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := s.SaveWithIntent(t.Context(), "unpublished-value", intents)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after deleting plaintext but before removing its intent.
	if err := os.Remove(filepath.Join(s.Dir, pending)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverPending(t.Context(), intents, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(untracked); err != nil {
		t.Fatalf("deleted an untracked credential: %v", err)
	}
	if _, err := os.Stat(filepath.Join(intents, pending)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("intent not retired after resumed cleanup: %v", err)
	}
}

func TestTeamSecretIntentFailurePreventsPlaintextWrite(t *testing.T) {
	s := TeamSecretSlots{Dir: t.TempDir()}
	intents := filepath.Join(t.TempDir(), "intents")
	if err := os.WriteFile(intents, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveWithIntent(t.Context(), "never-publish-value", intents); err == nil {
		t.Fatal("published without an intent")
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("plaintext written after journal failure: %v %v", entries, err)
	}
}
