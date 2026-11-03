package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"deepzora/internal/fileutil"
)

// RecoverPending reconciles only explicitly journaled new slots. It never
// sweeps untracked credentials. The caller must hold the owning secret-book
// lock and derive referenced from freshly loaded, validated metadata.
func (s TeamSecretSlots) RecoverPending(ctx context.Context, intentDir string, referenced map[string]bool) error {
	entries, err := os.ReadDir(intentDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		slot := entry.Name()
		if !teamSecretSlotName.MatchString(slot) {
			continue
		}
		path := filepath.Join(intentDir, slot)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != 2 {
			return errors.New("invalid Team secret creation intent")
		}
		body, err := os.ReadFile(path)
		if err != nil || string(body) != "1\n" {
			return errors.New("unknown Team secret creation intent version")
		}
		if !referenced[slot] {
			if err := s.removeUnpublished(slot); err != nil {
				return err
			}
		}
		// Sync the metadata directory before retiring the intent. In particular,
		// a prior metadata rename may have returned a directory-sync error.
		if err := fileutil.SyncParentDir(intentDir); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := fileutil.SyncParentDir(path); err != nil {
			return err
		}
	}
	return nil
}

func (s TeamSecretSlots) removeUnpublished(slot string) error {
	root, err := os.OpenRoot(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(slot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("Team secret cleanup requires a regular slot")
		}
		if err := root.Remove(slot); err != nil {
			return err
		}
	}
	return fileutil.SyncParentDir(filepath.Join(s.Dir, slot))
}
