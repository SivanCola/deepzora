package main

import "deepzora/internal/sessioninbox"

// Called under both configuration and runtime admission. A settled exchange
// with an unread reply still owns its participants and cannot be discarded.
func (a *App) requireTeamTargetIdle(agent, group string) error {
	store := desktopTeamStore()
	exchanges, err := store.Exchanges()
	if err != nil {
		return err
	}
	for _, x := range exchanges {
		if (agent != "" && (x.From == agent || x.To == agent) || group != "" && x.Group == group) && (!x.State.Settled() || x.State == "replied" && x.ReplyTo == "me" && !x.Returned) {
			return errTeamBusy
		}
	}
	if group != "" {
		round, err := store.Round(group)
		if err != nil {
			return err
		}
		if round.Speaking != nil || len(round.Queue) > 0 || len(round.Working) > 0 || len(round.Parked) > 0 {
			return errTeamBusy
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, tab := range a.runtimeTabsLocked() {
		if tab.teamSeat == nil {
			continue
		}
		_, ref := tab.teamSeat.Team()
		if !(agent != "" && ref.Agent == agent || group != "" && ref.Group == group) {
			continue
		}
		if controllerHasActiveRuntimeWork(tab.Ctrl) {
			return errTeamBusy
		}
		if inbox, ok := tab.Ctrl.(interface {
			InboxSnapshot() sessioninbox.InboxSnapshot
		}); ok && len(inbox.InboxSnapshot().Items) > 0 {
			return errTeamBusy
		}
	}
	return nil
}
