package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepzora/internal/fileutil"
)

func TestMemoryBatchRejectsEveryChangeWhenAnyRevisionConflicts(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "memory")}
	initial, err := s.SaveWithOptions(Memory{Name: "launch", Description: "launch", Body: "Friday"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := memoryBatchSnapshot(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	err = s.ApplyBatch(t.Context(), []BatchChange{
		{Memory: Memory{Name: "new-fact", Description: "new", Body: "must not be partially saved"}},
		{Memory: Memory{ID: initial.Memory.ID, Body: "Saturday"}, ExpectedRevision: initial.Memory.Revision + 1},
	})
	if err == nil {
		t.Fatal("accepted an inconsistent batch")
	}
	after, err := memoryBatchSnapshot(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sameBatchSnapshot(before, after) {
		t.Fatal("a revision conflict partially published the batch")
	}
}

func TestMemoryBatchCrashRecoversBeforeImagesAndCommittedMarkerRecoversForward(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "memory")}
	initial, err := s.SaveWithOptions(Memory{Name: "launch", Description: "launch", Body: "Friday"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := memoryBatchSnapshot(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	actual := 0
	previousHook := fileutil.CrashPoint
	fileutil.CrashPoint = func(_ string, path string) {
		if strings.HasPrefix(path, s.Dir+string(filepath.Separator)) && filepath.Base(path) != memoryBatchJournal {
			actual++
			if actual == 2 {
				panic("simulated crash")
			}
		}
	}
	func() {
		defer func() {
			fileutil.CrashPoint = previousHook
			if recover() == nil {
				t.Error("crash point did not run")
			}
		}()
		_ = s.ApplyBatch(t.Context(), []BatchChange{{Memory: Memory{ID: initial.Memory.ID, Description: "launch moved", Body: "Saturday"}, ExpectedRevision: initial.Memory.Revision}})
	}()
	if err := s.RecoverBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := memoryBatchSnapshot(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sameBatchSnapshot(before, after) {
		t.Fatal("recovery did not restore the complete original store")
	}
	file := batchDelta{Path: "committed.md", Before: batchFile{}, After: batchFile{Exists: true, Data: []byte("committed fact")}}
	encoded, err := json.Marshal(batchJournal{SchemaVersion: 1, Committed: true, Files: []batchDelta{file}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, memoryBatchJournal), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, file.Path))
	if err != nil || string(data) != "committed fact" {
		t.Fatalf("committed recovery rolled back success: %q %v", data, err)
	}
}

func TestMemoryBatchRecoveryPreservesConflictingUserEdits(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	file := batchDelta{Path: "launch.md", Before: batchFile{Exists: true, Data: []byte("before")}, After: batchFile{Exists: true, Data: []byte("after")}}
	encoded, err := json.Marshal(batchJournal{SchemaVersion: 1, Files: []batchDelta{file}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, memoryBatchJournal), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, file.Path), []byte("later user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverBatch(t.Context()); err == nil {
		t.Fatal("recovery overwrote an unrelated edit")
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, file.Path))
	if err != nil || string(data) != "later user edit" {
		t.Fatalf("user edit lost: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, memoryBatchJournal)); err != nil {
		t.Fatal("conflicting recovery discarded its evidence")
	}
}

func TestUnknownBatchSchemaSealsOrdinaryMemoryMutations(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	result, err := s.SaveWithOptions(Memory{Name: "existing", Description: "existing", Body: "retained"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	journal := []byte(`{"SchemaVersion":999,"Files":[]}`)
	if err := os.WriteFile(filepath.Join(s.Dir, memoryBatchJournal), journal, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveWithOptions(Memory{Name: "other", Description: "other", Body: "must not write"}, SaveOptions{}); err == nil {
		t.Fatal("saved through unknown recovery state")
	}
	if _, err := s.Archive(result.Memory.ID); err == nil {
		t.Fatal("archived through unknown recovery state")
	}
	if facts := s.ListAll(); len(facts) != 1 || facts[0].Body != "retained" {
		t.Fatalf("sealed namespace was overwritten: %+v", facts)
	}
}
