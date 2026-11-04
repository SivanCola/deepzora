package main

import (
	"context"
	"path/filepath"

	"deepzora/internal/config"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func teamSecretIntentDir(store teamroster.Store) string {
	return filepath.Join(store.Dir, "secret-slot-intents-v1")
}

func recoverTeamSecretCreations(ctx context.Context, store teamroster.Store, slots config.TeamSecretSlots) error {
	return store.InspectTeamSecrets(ctx, func(book teamroster.TeamSecretBook) error {
		referenced := make(map[string]bool)
		for _, sc := range book.Secrets {
			referenced[sc.Slot] = true
		}
		for _, sc := range book.Retired {
			referenced[sc.Slot] = true
		}
		for _, slot := range book.PendingDeletion {
			referenced[slot] = true
		}
		return slots.RecoverPending(ctx, teamSecretIntentDir(store), referenced)
	})
}

func (a *App) recoverTeamSecretCreations(ctx context.Context) error {
	if !teamSecretMaskingReady() {
		return nil
	}
	h, err := a.localTeamHost()
	if err != nil {
		return err
	}
	return h.WithWriter(ctx, func(ctx context.Context, _ *team.Service) error {
		return recoverTeamSecretCreations(ctx, desktopTeamStore(), config.DefaultTeamSecretSlots())
	})
}
