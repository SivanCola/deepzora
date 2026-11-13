package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"deepzora/internal/config"
	"deepzora/internal/fileutil"
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

func TestRetiredSlotCleanupRequiresDurableMetadataBeforeAnyRemoval(t *testing.T) {
	store := teamroster.Store{Dir: t.TempDir()}
	slots := config.TeamSecretSlots{Dir: t.TempDir()}
	var pending []string
	for _, value := range []string{"first-retired-value", "second-retired-value"} {
		slot, err := slots.Save(t.Context(), value)
		if err != nil {
			t.Fatal(err)
		}
		pending = append(pending, slot)
	}
	book, err := store.MutateTeamSecrets(t.Context(), 0, "durable-forget-test", "digest", func(book *teamroster.TeamSecretBook) error {
		book.PendingDeletion = pending
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertValuesPresent := func() {
		t.Helper()
		for _, slot := range pending {
			if _, err := slots.Read(slot); err != nil {
				t.Fatalf("removed slot before durable forget metadata: %v", err)
			}
		}
	}
	syncFailure := errors.New("injected metadata directory sync failure")
	calls := 0
	err = cleanupRetiredTeamSecretSlots(t.Context(), store, slots, func(path string) error {
		calls++
		if path != filepath.Join(store.Dir, teamroster.SecretFile) {
			t.Fatalf("sync targeted wrong metadata: %s", path)
		}
		assertValuesPresent()
		return syncFailure
	})
	if !errors.Is(err, syncFailure) || calls != 1 {
		t.Fatalf("metadata durability failure ignored: %v calls=%d", err, calls)
	}
	assertValuesPresent()
	current, err := store.TeamSecrets()
	if err != nil || current.Revision != book.Revision || len(current.PendingDeletion) != len(pending) {
		t.Fatalf("sync failure changed deletion intent: %+v %v", current, err)
	}
	// Retrying after restart re-reads durable metadata and synchronizes it
	// while every slot still exists, before the first destructive operation.
	err = cleanupRetiredTeamSecretSlots(t.Context(), teamroster.Store{Dir: store.Dir}, slots, func(path string) error {
		calls++
		assertValuesPresent()
		return fileutil.SyncParentDir(path)
	})
	if err != nil || calls != 2 {
		t.Fatalf("retry failed: %v calls=%d", err, calls)
	}
	for _, slot := range pending {
		if _, err := os.Stat(filepath.Join(slots.Dir, slot)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retry did not remove retired slot: %v", err)
		}
	}
	current, err = store.TeamSecrets()
	if err != nil || len(current.PendingDeletion) != 0 || current.Revision <= book.Revision {
		t.Fatalf("retry did not settle deletion intent: %+v %v", current, err)
	}
}
