package main

import (
	"os"
	"path/filepath"
	"testing"

	"deepzora/internal/config"
	"deepzora/internal/teamroster"
)

func TestRetiredSlotCleanupPersistsIntentAcrossFailureAndProtectsActiveSlots(t *testing.T) {
	a, _ := newMemoryTeamApp(t)
	slots := config.DefaultTeamSecretSlots()
	retired, err := slots.Save(t.Context(), "retired-cleanup-value")
	if err != nil {
		t.Fatal(err)
	}
	active, err := slots.Save(t.Context(), "active-cleanup-value")
	if err != nil {
		t.Fatal(err)
	}
	store := desktopTeamStore()
	book, err := store.MutateTeamSecrets(t.Context(), 0, "pending-cleanup-test", "digest", func(b *teamroster.TeamSecretBook) error {
		b.Secrets = []teamroster.TeamSecret{{Name: "ACTIVE", Slot: active, Agents: []string{}}}
		b.PendingDeletion = []string{retired}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A non-regular replacement simulates a removal that cannot be completed.
	path := filepath.Join(slots.Dir, retired)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.cleanupTeamSecretSlots(t.Context()); err == nil {
		t.Fatal("cleanup silently discarded a failed removal")
	}
	reloaded, err := store.TeamSecrets()
	if err != nil || len(reloaded.PendingDeletion) != 1 {
		t.Fatalf("cleanup intent lost: %+v %v", reloaded, err)
	}
	// The durable record is the restart owner; no in-memory deletion list is
	// needed to finish an interrupted cleanup, and missing files are retryable.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := a.cleanupTeamSecretSlots(t.Context()); err != nil {
		t.Fatal(err)
	}
	reloaded, err = store.TeamSecrets()
	if err != nil || len(reloaded.PendingDeletion) != 0 {
		t.Fatalf("cleanup did not settle: %+v %v", reloaded, err)
	}
	if value, err := slots.Read(active); err != nil || value != "active-cleanup-value" {
		t.Fatal("cleanup removed an active credential")
	}
	_, err = store.MutateTeamSecrets(t.Context(), reloaded.Revision, "reject-active-cleanup", "digest", func(b *teamroster.TeamSecretBook) error { b.PendingDeletion = []string{active}; return nil })
	if err == nil {
		t.Fatal("cleanup accepted an active credential reference")
	}
	if book.Revision >= reloaded.Revision {
		t.Fatal("cleanup completion did not commit its own revision")
	}
}
