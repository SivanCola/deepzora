package main

import (
	"deepzora/internal/config"
	"deepzora/internal/secrets"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"path/filepath"
)

func (a *App) ensureTeamSecretMasking() {
	a.teamState.mu.Lock()
	defer a.teamState.mu.Unlock()
	if a.teamState.releaseMasking != nil || a.teamState.closing {
		return
	}
	slots := config.DefaultTeamSecretSlots()
	store := desktopTeamStore()
	secrets.RegisterHostSecretPath(slots.Dir)
	secrets.RegisterHostSecretPath(filepath.Join(store.Dir, teamroster.SecretFile))
	releaseTeam := team.InstallSecretValues(store, slots)
	releaseBots := a.botStore().InstallSecretValues()
	a.teamState.releaseMasking = func() { releaseTeam(); releaseBots() }
	_ = secrets.Masker()
}
func (a *App) releaseTeamSecretMasking() {
	a.teamState.mu.Lock()
	release := a.teamState.releaseMasking
	a.teamState.releaseMasking = nil
	a.teamState.mu.Unlock()
	if release != nil {
		release()
	}
}
