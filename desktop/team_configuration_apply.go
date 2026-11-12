package service

import (
	"context"
	"errors"

	"deepzora/internal/agentpreset"
	"deepzora/internal/boot"
	"deepzora/internal/control"
	"deepzora/internal/teamroster"
)

func savedTeamPresetFingerprint(member teamroster.Agent) string {
	p := agentpreset.Default()
	if member.Preset != nil {
		p = *member.Preset
	}
	return boot.TeamPresetFingerprint(p)
}

func (a *App) teamPresetPending(tab *WorkspaceTab, ctrl control.SessionAPI) (bool, error) {
	if tab == nil || ctrl == nil {
		return false, nil
	}
	a.mu.RLock()
	seat := tab.teamSeat
	a.mu.RUnlock()
	if seat == nil {
		return false, nil
	}
	actual, ok := ctrl.(interface{ TeamPresetFingerprint() string })
	if !ok {
		return false, errors.New("team runtime has no applied configuration identity")
	}
	_, ref := seat.Team()
	r, err := desktopTeamStore().Load()
	if err != nil {
		return false, err
	}
	member, ok := r.Agent(ref.Agent)
	if !ok {
		return false, teamroster.ErrUnknownAgent
	}
	path, err := teamSessionPath(&r, ref)
	if err != nil {
		return false, err
	}
	a.mu.RLock()
	current := sessionRoute(tab.SessionID)
	a.mu.RUnlock()
	if path != current {
		return false, errors.New("team working directory changed; reopen this member from Team navigation")
	}
	return actual.TeamPresetFingerprint() != savedTeamPresetFingerprint(*member), nil
}

// Static prompt/tool changes are applied at the same idle publication boundary
// as ordinary settings rebuilds. A failed build retains the previous runtime.
func (a *App) applyTeamPresetIfIdle(ctx context.Context, tab *WorkspaceTab) (bool, error) {
	pending, err := a.teamPresetPending(tab, a.controllerForTab(tab))
	if err != nil || !pending {
		return false, err
	}
	defer a.lockRuntimeMutation("apply team preset")()
	tab.turnStartMu.Lock()
	defer tab.turnStartMu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	ctrl := a.controllerForTab(tab)
	if controllerHasActiveRuntimeWork(ctrl) {
		return false, nil
	}
	if pending, err = a.teamPresetPending(tab, ctrl); err != nil || !pending {
		return false, err
	}
	if err := a.rebuildSettingTurnLocked("team preset", tab, true, false); err != nil {
		return false, err
	}
	a.emitTeamChanged()
	return true, nil
}

func (a *App) reconcileTeamPresets(ctx context.Context) error {
	a.mu.RLock()
	var tabs []*WorkspaceTab
	for _, tab := range a.runtimeTabsLocked() {
		if tab.teamSeat != nil && tab.Ctrl != nil {
			tabs = append(tabs, tab)
		}
	}
	a.mu.RUnlock()
	var failures error
	for _, tab := range tabs {
		if _, err := a.applyTeamPresetIfIdle(ctx, tab); err != nil {
			failures = errors.Join(failures, err)
		}
	}
	return failures
}
