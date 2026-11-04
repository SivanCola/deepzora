package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"deepzora/internal/fileutil"
)

const memoryBatchJournal = ".memory-batch.json"

var ErrRevisionConflict = errors.New("memory revision conflict")

// BatchChange is a revision-bound review operation. Remove archives the fact;
// automatic review never changes pinned guidance. This API is for one isolated
// namespace, not a merged project/global store.
type BatchChange struct {
	Memory           Memory
	ExpectedRevision int
	Remove           bool
}

type batchFile struct {
	Exists bool
	Data   []byte
}
type batchDelta struct {
	Path          string
	Before, After batchFile
}
type batchJournal struct {
	SchemaVersion int
	Committed     bool
	Files         []batchDelta
}

// ApplyBatch validates every operation on an isolated staging copy before
// touching the authoritative store. A durable before-image journal allows
// crash recovery; failed publication rolls the entire set back. Ordinary
// single-fact writes keep their existing format and behavior.
func (s Store) ApplyBatch(ctx context.Context, changes []BatchChange) error {
	if len(changes) == 0 {
		return nil
	}
	if len(changes) > 8 {
		return errors.New("memory batch exceeds eight changes")
	}
	return s.applyNamespaceTransaction(ctx, func(staged Store) error {
		seen := map[string]bool{}
		for _, change := range changes {
			if err := ctx.Err(); err != nil {
				return err
			}
			if seen[change.Memory.ID] && change.Memory.ID != "" {
				return errors.New("memory batch repeats a target")
			}
			seen[change.Memory.ID] = true
			if _, err := staged.applyStagedChange(change, false); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s Store) applyStagedChange(change BatchChange, manual bool) (Memory, error) {
	m := change.Memory
	if m.Scope != "" && m.Scope != FactScopeProject {
		return Memory{}, errors.New("memory batch cannot change namespace")
	}
	if m.ID != "" {
		current, _, exists := s.findActive(m.ID)
		if !exists || current.Revision != change.ExpectedRevision {
			return Memory{}, ErrRevisionConflict
		}
		if !manual && ResolveActivation(current) == ActivationPinned {
			return Memory{}, errors.New("automatic memory cannot change pinned guidance")
		}
	} else if change.Remove || change.ExpectedRevision != 0 {
		return Memory{}, errors.New("memory batch target is invalid")
	}
	if !manual && ResolveActivation(m) == ActivationPinned {
		return Memory{}, errors.New("automatic memory cannot pin guidance")
	}
	if change.Remove {
		_, err := s.archiveLocked(m.ID)
		return m, err
	}
	saved, err := s.saveWithOptionsLocked(m, SaveOptions{ExpectedRevision: change.ExpectedRevision, RequireExpectedRevision: m.ID != "", RequireCreate: m.ID == ""})
	return saved.Memory, err
}

func (s Store) applyNamespaceTransaction(ctx context.Context, apply func(Store) error) error {
	if s.Dir == "" || s.GlobalDir != "" {
		return errors.New("memory batch requires one isolated namespace")
	}
	memoryStoreMutationMu.Lock()
	defer memoryStoreMutationMu.Unlock()
	if err := s.recoverBatchLocked(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	before, err := memoryBatchSnapshot(s.Dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Dir), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(s.Dir), ".memory-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for path, data := range before {
		if err := publishBatchFile(stage, path, data); err != nil {
			return err
		}
	}
	if err := apply(Store{Dir: stage}); err != nil {
		return err
	}
	after, err := memoryBatchSnapshot(stage)
	if err != nil {
		return err
	}
	current, err := memoryBatchSnapshot(s.Dir)
	if err != nil {
		return err
	}
	if !sameBatchSnapshot(before, current) {
		return errors.New("memory changed while preparing batch")
	}
	journal := makeBatchJournal(before, after)
	return s.publishBatch(ctx, journal)
}

func makeBatchJournal(before, after map[string]batchFile) batchJournal {
	journal := batchJournal{SchemaVersion: 1}
	paths := map[string]bool{}
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	for path := range paths {
		if !sameBatchFile(before[path], after[path]) {
			journal.Files = append(journal.Files, batchDelta{Path: path, Before: before[path], After: after[path]})
		}
	}
	slices.SortFunc(journal.Files, func(a, b batchDelta) int { return strings.Compare(a.Path, b.Path) })
	return journal
}

func (s Store) publishBatch(ctx context.Context, journal batchJournal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	encoded, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	if err := fileutil.AtomicWriteFileStrict(filepath.Join(s.Dir, memoryBatchJournal), encoded, 0600); err != nil {
		return err
	}
	for _, file := range journal.Files {
		if err := publishBatchFile(s.Dir, file.Path, file.After); err != nil {
			return errors.Join(err, s.recoverBatchLocked())
		}
	}
	journal.Committed = true
	encoded, err = json.Marshal(journal)
	if err != nil {
		return err
	}
	if err := fileutil.AtomicWriteFileStrict(filepath.Join(s.Dir, memoryBatchJournal), encoded, 0600); err != nil {
		return errors.Join(err, s.recoverBatchLocked())
	}
	// A durable commit marker makes cleanup retryable without rolling back a
	// batch already reported as successful.
	_ = os.Remove(filepath.Join(s.Dir, memoryBatchJournal))
	return nil
}

func (s Store) RecoverBatch(ctx context.Context) error {
	if s.Dir == "" || s.GlobalDir != "" {
		return errors.New("memory recovery requires one isolated namespace")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	memoryStoreMutationMu.Lock()
	defer memoryStoreMutationMu.Unlock()
	return s.recoverBatchLocked()
}

func (s Store) recoverIsolatedBatchLocked() error {
	if s.Dir == "" || s.GlobalDir != "" {
		return nil
	}
	return s.recoverBatchLocked()
}

func (s Store) recoverBatchLocked() error {
	path := filepath.Join(s.Dir, memoryBatchJournal)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal batchJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return err
	}
	if journal.SchemaVersion != 1 {
		return errors.New("unknown memory batch schema; journal retained")
	}
	for _, file := range journal.Files {
		if !safeBatchPath(file.Path) {
			return errors.New("invalid memory recovery path")
		}
		data, err := readBatchFile(filepath.Join(s.Dir, file.Path))
		if err != nil {
			return err
		}
		if !sameBatchFile(data, file.Before) && !sameBatchFile(data, file.After) {
			return errors.New("memory recovery conflicts with a later edit; journal retained")
		}
	}
	for _, file := range journal.Files {
		target := file.Before
		if journal.Committed {
			target = file.After
		}
		if err := publishBatchFile(s.Dir, file.Path, target); err != nil {
			return err
		}
	}
	return os.Remove(path)
}

func safeBatchPath(path string) bool {
	return path != "" && !filepath.IsAbs(path) && filepath.Clean(path) == path && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator)) && path != memoryBatchJournal
}
func sameBatchFile(a, b batchFile) bool { return a.Exists == b.Exists && bytes.Equal(a.Data, b.Data) }
func sameBatchSnapshot(a, b map[string]batchFile) bool {
	if len(a) != len(b) {
		return false
	}
	for path, data := range a {
		if !sameBatchFile(data, b[path]) {
			return false
		}
	}
	return true
}
func readBatchFile(path string) (batchFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return batchFile{}, nil
	}
	return batchFile{Exists: err == nil, Data: data}, err
}
func publishBatchFile(root, path string, data batchFile) error {
	if !safeBatchPath(path) {
		return errors.New("invalid memory batch path")
	}
	target := filepath.Join(root, path)
	if !data.Exists {
		err := os.Remove(target)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFileStrict(target, data.Data, 0600)
}
func memoryBatchSnapshot(root string) (map[string]batchFile, error) {
	out := map[string]batchFile{}
	size := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("memory batch refuses symbolic links")
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == memoryBatchJournal {
			return nil
		}
		file, err := readBatchFile(path)
		if err != nil {
			return err
		}
		size += len(file.Data)
		if len(out) >= 2000 || size > 20*1024*1024 {
			return fmt.Errorf("memory batch store exceeds staging budget")
		}
		out[rel] = file
		return nil
	})
	return out, err
}
