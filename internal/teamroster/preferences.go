package teamroster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"deepzora/internal/fileutil"
)

type GroupPreferences struct {
	DraftRevision uint64 `json:"draftRevision"`
	DraftJSON     string `json:"draftJson"`
	ReadSequence  int64  `json:"readSequence"`
	ReadDigest    string `json:"readDigest"`
}

type groupPreferencesFile struct {
	SchemaVersion int `json:"schemaVersion"`
	GroupPreferences
}

func (s Store) GroupPreferences(group string) (GroupPreferences, error) {
	dir, err := s.groupDir(group)
	if err != nil {
		return GroupPreferences{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "desktop.json"))
	if errors.Is(err, os.ErrNotExist) {
		return GroupPreferences{}, nil
	}
	if err != nil {
		return GroupPreferences{}, err
	}
	var saved groupPreferencesFile
	if err := json.Unmarshal(data, &saved); err != nil {
		return GroupPreferences{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if saved.SchemaVersion != 1 {
		return GroupPreferences{}, ErrNewerSchema
	}
	return saved.GroupPreferences, nil
}

// Drafts and read watermarks have their own lock/revision, so typing never
// conflicts with an unrelated roster edit or an arriving group message.
func (s Store) UpdateGroupPreferences(ctx context.Context, group string, mutate func(*GroupPreferences) error) (GroupPreferences, error) {
	dir, err := s.groupDir(group)
	if err != nil {
		return GroupPreferences{}, err
	}
	var result GroupPreferences
	err = s.withLock(ctx, dir, "desktop.json", func() error {
		current, err := s.GroupPreferences(group)
		if err != nil {
			return err
		}
		if err := mutate(&current); err != nil {
			return err
		}
		data, err := json.Marshal(groupPreferencesFile{SchemaVersion: 1, GroupPreferences: current})
		if err != nil {
			return err
		}
		if err := fileutil.AtomicWriteFileStrict(filepath.Join(dir, "desktop.json"), data, 0600); err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}
