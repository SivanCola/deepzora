package main

import (
	"context"
	"errors"
	"slices"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/internal/control"
)

var errBotSessionUnavailable = errors.New("open the bound local session in Desktop before connecting")

// An enabled bot owns its runtime even between turns. Navigation can detach
// that owner, but must not close or reuse it as an ordinary idle surface.
func (a *App) botOwnsController(ctrl control.SessionAPI) bool {
	c, ok := ctrl.(*control.Controller)
	if !ok || c == nil {
		return false
	}
	a.botChannels.mu.Lock()
	defer a.botChannels.mu.Unlock()
	for _, r := range a.botChannels.runs {
		r.mu.Lock()
		matches := r.ctrl == c
		r.mu.Unlock()
		if matches {
			select {
			case <-r.done:
			default:
				return true
			}
		}
	}
	return false
}

func (a *App) preserveRuntimeForNavigation(ctrl control.SessionAPI) bool {
	return controllerHasActiveRuntimeWork(ctrl) || a.botOwnsController(ctrl)
}

// op is held. Shutdown stops workers without changing this durable intent;
// only the user's Disconnect action disables restart restoration.
func (a *App) setBotEnabledLocked(id string, enabled bool) error {
	a.botChannels.mu.Lock()
	closed := a.botChannels.closed
	a.botChannels.mu.Unlock()
	if closed || a.shuttingDown.Load() {
		return errors.New("desktop is shutting down")
	}
	if err := a.stopBotChannel(id); err != nil {
		return err
	}
	var unlock func()
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	return a.botStore().Edit(func(entries []botchannels.Saved) ([]botchannels.Saved, error) {
		idx := slices.IndexFunc(entries, func(c botchannels.Saved) bool { return c.ID == id })
		if idx < 0 {
			if !enabled {
				return entries, nil
			}
			return nil, errors.New("bot connection not found")
		}
		var err error
		unlock, err = a.lockBotAccount(entries[idx])
		if err != nil {
			return nil, err
		}
		entries[idx].Enabled = enabled
		return entries, nil
	})
}

// Saving and starting are separate outcomes. A failed start remains visible on
// the saved card, without throwing away a successfully persisted editor draft.
func (a *App) startSavedBotLocked(id string, restoring bool) error {
	err := a.connectBotChannelLocked(id, restoring)
	if err == nil {
		return nil
	}
	entries, readErr := a.botStore().Read()
	if readErr != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		done := make(chan struct{})
		close(done)
		r := &botChannelRun{config: entry, status: "error", message: err.Error(), done: done, cancel: func() {}}
		a.botChannels.mu.Lock()
		if a.botChannels.runs == nil {
			a.botChannels.runs = map[string]*botChannelRun{}
		}
		a.botChannels.runs[id] = r
		a.botChannels.mu.Unlock()
		break
	}
	return err
}

func (a *App) restoreEnabledBotChannels() {
	select {
	case <-a.bootContext().Done():
		return
	case <-a.tabsRestoredSignal():
	}
	a.botChannels.op.Lock()
	defer a.botChannels.op.Unlock()
	if a.shuttingDown.Load() {
		return
	}
	errMigration := a.migrateBotMembersLocked()
	a.botChannels.mu.Lock()
	a.botChannels.migrationError = ""
	if errMigration != nil {
		a.botChannels.migrationError = "Bot member migration failed; original configuration and history retained: " + errMigration.Error()
	}
	a.botChannels.mu.Unlock()
	entries, err := a.botStore().Read()
	if err != nil {
		return
	} // List surfaces configuration read failures to the user.
	for _, entry := range entries {
		if entry.MemberID == "" && errMigration != nil {
			continue
		}
		if !entry.Enabled {
			continue
		}
		a.botChannels.mu.Lock()
		skip := a.botChannels.closed || a.botChannels.runs[entry.ID] != nil
		a.botChannels.mu.Unlock()
		if !skip {
			a.startSavedBotLocked(entry.ID, true)
		}
	}
}

func (a *App) waitForBotSession(ctx context.Context, r *botChannelRun) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := a.bindBotRun(r)
		if !errors.Is(err, errBotSessionUnavailable) {
			return err
		}
		select {
		case <-ctx.Done():
			return errBotSessionUnavailable
		case <-ticker.C:
		}
	}
}
