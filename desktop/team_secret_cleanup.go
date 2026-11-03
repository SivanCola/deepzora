package main

import (
	"context"
	"fmt"

	"deepzora/internal/config"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

func (a *App) cleanupTeamSecretSlots(ctx context.Context) error {
	return a.withTeamWriter(func(_ context.Context, svc *team.Service) error {
		return svc.Configure(ctx, func() error {
			store := desktopTeamStore()
			book, err := store.TeamSecrets()
			if err != nil || len(book.PendingDeletion) == 0 {
				return err
			}
			slots := config.DefaultTeamSecretSlots()
			for _, slot := range book.PendingDeletion {
				if err := slots.Remove(slot); err != nil {
					return err
				}
			}
			_, err = store.MutateTeamSecrets(ctx, book.Revision, fmt.Sprintf("cleanup-retired-%d", book.Revision), "retired-slot-cleanup", func(current *teamroster.TeamSecretBook) error { current.PendingDeletion = nil; return nil })
			return err
		})
	})
}
