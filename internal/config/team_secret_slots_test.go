package config

import (
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
