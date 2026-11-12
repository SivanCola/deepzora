package service

import (
	"deepzora/internal/control"
	"deepzora/internal/teamroster"
	"errors"
)

func controllerHasTeamSeat(ctrl control.SessionAPI) bool {
	bound, ok := ctrl.(interface{ BoundTeamSeat() control.TeamSeat })
	return ok && bound.BoundTeamSeat() != nil
}

// Team membership owns the persistent preset for every seat. A saved sidecar
// or a different seat must never restore a broader, obsolete permission.
func teamPermissionForController(ctrl control.SessionAPI, fallback string) (string, error) {
	bound, ok := ctrl.(interface{ BoundTeamSeat() control.TeamSeat })
	if !ok || bound.BoundTeamSeat() == nil {
		return fallback, nil
	}
	_, ref := bound.BoundTeamSeat().Team()
	roster, err := desktopTeamStore().Load()
	if err != nil {
		return "", err
	}
	member, ok := roster.Agent(ref.Agent)
	if !ok {
		return "", teamroster.ErrUnknownAgent
	}
	return normalizeToolApprovalMode(member.Permission), nil
}

func (a *App) reconcileTeamPermissions() error {
	a.runtimeAdmissionMu.RLock()
	defer a.runtimeAdmissionMu.RUnlock()
	a.mu.RLock()
	var tabs []*WorkspaceTab
	for _, tab := range a.runtimeTabsLocked() {
		if tab.teamSeat != nil && tab.Ctrl != nil {
			tabs = append(tabs, tab)
		}
	}
	a.mu.RUnlock()
	for _, tab := range tabs {
		tab.turnStartMu.Lock()
		ctrl := a.controllerForTab(tab)
		mode, err := teamPermissionForController(ctrl, "read-only")
		if errors.Is(err, teamroster.ErrUnknownAgent) {
			mode, err = "read-only", nil
		}
		if err != nil {
			tab.turnStartMu.Unlock()
			return err
		}
		applyTabToolApprovalModeToController(ctrl, mode)
		a.mu.Lock()
		tab.toolApprovalMode = mode
		a.mu.Unlock()
		tab.turnStartMu.Unlock()
	}
	return nil
}
