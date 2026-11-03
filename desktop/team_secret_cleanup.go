package main

import (
	"context"
	"fmt"
	"path/filepath"

	"deepzora/internal/config"
	"deepzora/internal/fileutil"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func (a *App) cleanupTeamSecretSlots(ctx context.Context) error {
	return a.withTeamWriter(func(_ context.Context, svc *team.Service) error {
		return svc.Configure(ctx, func() error {
			return cleanupRetiredTeamSecretSlots(ctx, desktopTeamStore(), config.DefaultTeamSecretSlots(), fileutil.SyncParentDir)
		})
	})
}

// The caller holds the Team writer lease and configuration admission. Persist
// the forget decision before deleting any value: atomic publication otherwise
// treats post-rename directory sync as best effort, which is insufficient for
// a destructive follow-up. A failed sync leaves all slots and intents intact.
func cleanupRetiredTeamSecretSlots(ctx context.Context, store teamroster.Store, slots config.TeamSecretSlots, syncMetadata func(string) error) error {
	book, err := store.TeamSecrets()
	if err != nil || len(book.PendingDeletion) == 0 {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := syncMetadata(filepath.Join(store.Dir, teamroster.SecretFile)); err != nil {
		return err
	}
	for _, slot := range book.PendingDeletion {
		if err := slots.Remove(slot); err != nil {
			return err
		}
	}
	_, err = store.MutateTeamSecrets(ctx, book.Revision, fmt.Sprintf("cleanup-retired-%d", book.Revision), "retired-slot-cleanup", func(current *teamroster.TeamSecretBook) error { current.PendingDeletion = nil; return nil })
	return err
}
