package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"deepzora/internal/provider"
	"deepzora/internal/tool"
)

func TestSubagentStoreTerminalSaveKeepsStartTime(t *testing.T) {
	store := newTestSubagentStore(t, t.TempDir())
	run, err := store.PrepareFresh(testSubagentSpec(t, "explore"))
	if err != nil {
		t.Fatal(err)
	}
	defer run.Release()
	created := run.Meta.CreatedAt
	appendSubagentMessages(t, run, provider.Message{Role: provider.RoleAssistant, Content: "done"})
	before := time.Now().UTC()
	if err := store.SaveCompleted(run); err != nil {
		t.Fatal(err)
	}
	meta, err := store.LoadMeta(run.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.CreatedAt.Equal(created) || meta.UpdatedAt.Before(before) {
		t.Fatalf("terminal times = %+v", meta)
	}
}

func TestSubagentStoreSaveFailedPersistsTerminalMetaWhenFlushFails(t *testing.T) {
	store := newTestSubagentStore(t, t.TempDir())
	run, err := store.PrepareFresh(testSubagentSpec(t, "explore"))
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	defer run.Release()
	if err := store.MarkRunning(run); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	run.Persistence.(*testSubagentTranscript).flushErr = errors.New("flush failed")

	if err := store.SaveFailed(run); err == nil {
		t.Fatal("SaveFailed unexpectedly succeeded despite flush failure")
	}
	meta, err := store.LoadMeta(run.Ref)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Status != SubagentFailed {
		t.Fatalf("persisted status = %q, want %q", meta.Status, SubagentFailed)
	}
	if run.terminalPersisted {
		t.Fatal("flush failure recorded successful terminal durability")
	}
	if err := store.SaveFailed(run); err == nil {
		t.Fatal("retry skipped the failed flush")
	}
	run.Persistence.(*testSubagentTranscript).flushErr = nil
	if err := store.SaveFailed(run); err != nil {
		t.Fatal(err)
	}
	if !run.terminalPersisted {
		t.Fatal("successful retry did not confirm durability")
	}
}

func TestSubagentStoreCleanupStaleRunningSkipsMissingParentProof(t *testing.T) {
	store := newTestSubagentStore(t, t.TempDir())
	spec := testSubagentSpec(t, "review")
	run, err := store.PrepareFresh(spec)
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	if err := store.MarkRunning(run); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	ref := run.Ref
	run.Release()
	meta, err := store.LoadMeta(ref)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	meta.ParentSession = ""
	if err := store.saveMeta(meta); err != nil {
		t.Fatalf("saveMeta without parent: %v", err)
	}

	cleaned, err := store.CleanupStaleRunning()
	if err != nil {
		t.Fatalf("CleanupStaleRunning: %v", err)
	}
	if cleaned != 0 {
		t.Fatalf("cleaned = %d without parent proof, want 0", cleaned)
	}
	meta, err = store.LoadMeta(ref)
	if err != nil {
		t.Fatalf("LoadMeta after cleanup: %v", err)
	}
	if meta.Status != SubagentRunning {
		t.Fatalf("status = %q without parent proof, want running", meta.Status)
	}
}

func TestSubagentStoreCleanupStaleRunningSkipsCorruptMeta(t *testing.T) {
	store := newTestSubagentStore(t, t.TempDir())
	spec := testSubagentSpec(t, "review")
	run, err := store.PrepareFresh(spec)
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	if err := store.MarkRunning(run); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	ref := run.Ref
	run.Release()

	// Corrupt metadata files (truncated JSON, empty, and invalid custom field
	// values) must be skipped, not abort the whole startup cleanup.
	for i, corrupt := range []string{
		`{"status":"running"`,
		"",
		`{"createdAt":"not-a-time"}`,
	} {
		corruptRef := fmt.Sprintf("sa_corrupt_%d", i)
		if err := os.WriteFile(filepath.Join(store.dir, corruptRef+".meta.json"), []byte(corrupt), 0o600); err != nil {
			t.Fatalf("write corrupt meta %d: %v", i, err)
		}
	}

	cleaned, err := store.CleanupStaleRunning()
	if err != nil {
		t.Fatalf("CleanupStaleRunning should skip corrupt meta: %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("cleaned = %d, want 1 (corrupt metas skipped, running meta interrupted)", cleaned)
	}
	meta, err := store.LoadMeta(ref)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Status != SubagentInterrupted {
		t.Fatalf("status = %q, want interrupted", meta.Status)
	}
}

func TestSubagentStoreCleanupStaleRunningKeepsParentGuardAfterCorruptReread(t *testing.T) {
	sessionDir := t.TempDir()
	store := newTestSubagentStore(t, filepath.Join(sessionDir, "subagents"))
	spec := testSubagentSpec(t, "review")
	spec.ParentSession = "lease-parent"

	refs := make([]string, 0, 2)
	for range 2 {
		run, err := store.PrepareFresh(spec)
		if err != nil {
			t.Fatalf("PrepareFresh: %v", err)
		}
		if err := store.MarkRunning(run); err != nil {
			t.Fatalf("MarkRunning: %v", err)
		}
		refs = append(refs, run.Ref)
		run.Release()
	}
	sort.Strings(refs)

	held := false
	store.WithParentIdleGuard(func(_ context.Context, id string, work func() error) (bool, error) {
		held = true
		defer func() { held = false }()
		return true, work()
	})
	var guardedReread bool
	store.cleanupBeforeReread = func(parentSession, ref string) {
		switch ref {
		case refs[0]:
			if err := os.WriteFile(store.metaPath(ref), []byte(`{"createdAt":"not-a-time"}`), 0o600); err != nil {
				t.Fatalf("corrupt first metadata reread: %v", err)
			}
		case refs[1]:
			guardedReread = held
		}
	}

	cleaned, err := store.CleanupStaleRunning()
	if err != nil {
		t.Fatalf("CleanupStaleRunning: %v", err)
	}
	if !guardedReread || held {
		t.Fatal("guard must span every reread and release after cleanup")
	}
	if cleaned != 1 {
		t.Fatalf("cleaned = %d, want 1", cleaned)
	}
	meta, err := store.LoadMeta(refs[1])
	if err != nil {
		t.Fatalf("LoadMeta second ref: %v", err)
	}
	if meta.Status != SubagentInterrupted {
		t.Fatalf("second ref status = %q, want interrupted", meta.Status)
	}
}

func TestSubagentStoreRefusesDestroyedParent(t *testing.T) {
	store := newTestSubagentStore(t, t.TempDir()).WithDestroyedChecker(func(string) bool { return true })
	if _, err := store.PrepareFresh(testSubagentSpec(t, "review")); err == nil {
		t.Fatal("created a child for destroyed parent")
	}
	if len(store.persistence.(*testSubagentPersistence).records) != 0 {
		t.Fatal("destroyed parent created artifacts")
	}
}

func testSubagentSpec(t *testing.T, name string) SubagentSpec {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	return SubagentSpec{
		Kind:          "skill",
		Name:          name,
		WorkspaceRoot: t.TempDir(),
		ParentSession: "parent-session",
		SystemPrompt:  "review persona",
		Registry:      reg,
		Model:         "deepseek",
		Effort:        "max",
	}
}
