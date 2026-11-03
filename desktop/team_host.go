package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"deepzora/desktop/internal/teamhost"
	"deepzora/internal/config"
	"deepzora/internal/control"
	"deepzora/internal/secrets"
	"deepzora/internal/session"
	"deepzora/internal/sessioninbox"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type desktopTeamState struct {
	mu                 sync.Mutex
	host               *teamhost.Host
	cancel             context.CancelFunc
	done               chan struct{}
	changed            func() // Test observer, installed before host startup.
	historyMu          sync.Mutex
	preferencesMu      sync.Mutex
	history            map[string]teamGroupSnapshot
	reviews            *teamMemoryReviews
	reviewsPaused      bool
	reviewPrefsMu      sync.Mutex
	scheduleMu         sync.Mutex
	schedulesRecovered bool
	closing            bool
	releaseMasking     func()
}

func desktopTeamStore() teamroster.Store {
	return teamroster.Store{Dir: filepath.Join(config.MemoryUserDir(), "team")}
}

// Reserved Team identities cannot be reopened as ordinary sessions after an
// idle eviction, deletion, lease loss or a stale project-history navigation.
func (a *App) teamSeatForCanonicalSession(id string) (control.TeamSeat, error) {
	if !strings.HasPrefix(id, "desktop-team-") {
		return nil, nil
	}
	r, err := desktopTeamStore().Load()
	if err != nil {
		return nil, err
	}
	path := sessionRoute(id)
	var ref team.SeatRef
	for _, member := range r.Agents {
		if member.SessionPath == path {
			ref.Agent = member.ID
		}
	}
	for _, group := range r.Groups {
		for agent, binding := range group.MemberSessions {
			if binding == path {
				ref = team.SeatRef{Agent: agent, Group: group.ID}
			}
		}
	}
	if ref.Agent == "" {
		return nil, teamroster.ErrUnknownAgent
	}
	if _, err := teamSessionPath(&r, ref); err != nil {
		return nil, err
	}
	h, err := a.localTeamHost()
	if err != nil {
		return nil, err
	}
	if err := h.Claim(a.bootContext()); err != nil {
		return nil, err
	}
	if _, err := h.Ensure(a.bootContext(), ref); err != nil {
		return nil, err
	}
	return team.Seat{Service: h.BindingService(), Ref: ref}, nil
}

func (a *App) localTeamHost() (*teamhost.Host, error) {
	a.ensureTeamSecretMasking()
	a.teamState.mu.Lock()
	defer a.teamState.mu.Unlock()
	if a.shuttingDown.Load() {
		return nil, context.Canceled
	}
	if a.teamState.host != nil {
		return a.teamState.host, nil
	}
	h, err := teamhost.New(teamhost.Options{Store: desktopTeamStore(), Open: a.openTeamRuntime, Changed: a.emitTeamChanged, SecretSlots: config.DefaultTeamSecretSlots(), SecretsReady: teamSecretMaskingReady, AgentConfigurationChanged: func(ctx context.Context, r *teamroster.Roster, before, after teamroster.Agent) error {
		if before.Folder == after.Folder {
			return nil
		}
		defer a.lockRuntimeMutation("apply reviewed team working directory")()
		return a.migrateTeamAgentDirectories(ctx, r, after.ID, "reviewed-directory-"+teamMutationDigest([]any{r.Revision, after.ID, after.Folder}))
	}, Validate: func(ref team.SeatRef) error {
		r, err := desktopTeamStore().Load()
		if err != nil {
			return err
		}
		_, err = teamSessionPath(&r, ref)
		return err
	}, Binding: func(ref team.SeatRef) (string, error) {
		r, err := desktopTeamStore().Load()
		if err != nil {
			return "", err
		}
		return teamSessionPath(&r, ref)
	}})
	if err != nil {
		return nil, err
	}
	a.teamState.host = h
	return h, nil
}

func (a *App) emitTeamChanged() {
	if a.teamState.changed != nil {
		a.teamState.changed()
	}
	a.emitRuntimeEvent("team:changed")
}

func (a *App) emitTeamRuntimeChangedForTab(tabID string) {
	a.mu.RLock()
	tab := a.tabByEventSinkIDLocked(tabID)
	bound := tab != nil && tab.teamSeat != nil
	a.mu.RUnlock()
	if bound {
		a.emitTeamChanged()
	}
}

func (a *App) startTeamHost() {
	a.teamState.mu.Lock()
	if a.teamState.done != nil {
		a.teamState.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.bootContext())
	done := make(chan struct{})
	a.teamState.cancel, a.teamState.done = cancel, done
	a.teamState.mu.Unlock()
	go func() {
		defer close(done)
		select {
		case <-a.tabsRestoredSignal():
		case <-ctx.Done():
			return
		}
		h, err := a.localTeamHost()
		if err != nil {
			return
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			if err := h.Tick(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("desktop: team tick", "err", secrets.RedactError(err))
			}
			if h.Ready() {
				if err := a.cleanupTeamSecretSlots(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("desktop: retired Team secret cleanup", "err", secrets.RedactError(err))
				}
				if err := a.recoverTeamSecretCreations(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("desktop: Team secret creation recovery", "err", secrets.RedactError(err))
				}
				if err := a.tickTeamSchedules(ctx, time.Now()); err != nil && ctx.Err() == nil {
					slog.Warn("desktop: Team schedule", "err", secrets.RedactError(err))
				}
				if err := a.reconcileTeamPresets(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("desktop: apply team preset", "err", secrets.RedactError(err))
				}
				a.notifyTeamInboxReady()
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (a *App) stopTeamHost() error {
	a.teamState.mu.Lock()
	a.teamState.closing = true
	h, cancel, done := a.teamState.host, a.teamState.cancel, a.teamState.done
	a.teamState.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.stopTeamMemoryReviews()
	// Shutdown cancels outstanding seat admission before joining the clock.
	var err error
	if h != nil {
		err = h.Shutdown(context.Background())
	}
	if done != nil {
		<-done
	}
	a.releaseTeamSecretMasking()
	return err
}

func (a *App) notifyTeamInboxReady() {
	a.mu.RLock()
	var controllers []*control.Controller
	for _, tab := range a.runtimeTabsLocked() {
		if tab.teamSeat != nil {
			if c, ok := tab.Ctrl.(*control.Controller); ok {
				controllers = append(controllers, c)
			}
		}
	}
	a.mu.RUnlock()
	for _, c := range controllers {
		c.NotifyInboxRuntimeReady()
	}
}

// The roster persists canonical session routes, never legacy transcript paths.
func teamSessionPath(r *teamroster.Roster, ref team.SeatRef) (string, error) {
	a, ok := r.Agent(ref.Agent)
	if !ok {
		return "", teamroster.ErrUnknownAgent
	}
	if ref.Group == "" {
		return a.SessionPath, nil
	}
	g, ok := r.Group(ref.Group)
	if !ok {
		return "", teamroster.ErrUnknownGroup
	}
	if !g.IsMember(ref.Agent) {
		return "", teamroster.ErrNotMember
	}
	if g.Archived {
		return "", errors.New("team group is archived")
	}
	return g.MemberSessions[ref.Agent], nil
}

func (a *App) openTeamRuntime(ctx context.Context, ref team.SeatRef, service *team.Service) (result teamhost.Runtime, resultErr error) {
	store := desktopTeamStore()
	r, err := store.Load()
	if err != nil {
		return nil, err
	}
	path, err := teamSessionPath(&r, ref)
	if err != nil {
		return nil, err
	}
	member, _ := r.Agent(ref.Agent)
	root, err := teamRuntimeRoot(store, &r, ref)
	if err != nil {
		return nil, err
	}
	if path == "" {
		// Commit the reservation first so a crash/retry cannot create a second
		// session. Creation below uses the normal workspace registry protocol.
		path = sessionRoute("desktop-team-" + ref.Agent + "-" + ref.Group + "-" + newTabID())
		_, err = store.UpdateAt(ctx, r.Revision, func(current *teamroster.Roster) error {
			if _, err := teamSessionPath(current, ref); err != nil {
				return err
			}
			if ref.Group == "" {
				m, _ := current.Agent(ref.Agent)
				m.SessionPath = path
			} else {
				g, _ := current.Group(ref.Group)
				if g.MemberSessions == nil {
					g.MemberSessions = map[string]string{}
				}
				g.MemberSessions[ref.Agent] = path
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	id, ok := parseSessionRoute(path)
	if !ok {
		return nil, errors.New("team session binding is not a canonical session identity")
	}
	a.mu.RLock()
	existing := a.liveRuntimeTabMatchingLocked(nil, path)
	a.mu.RUnlock()
	if existing != nil {
		if existing.teamSeat == nil {
			return nil, errors.New("team session already has an unbound runtime")
		}
		return &desktopTeamRuntime{app: a, ref: ref, sessionID: id}, nil
	}
	workspace, err := a.ensureDesktopWorkspace(ctx, "project", root)
	if err != nil {
		return nil, err
	}
	settings := a.defaultSessionSettings("project", root)
	if member.InitialModel != "" {
		settings.Model = member.InitialModel
	}
	settings.ToolApprovalMode = member.Permission
	if settings.ToolApprovalMode == "" {
		settings.ToolApprovalMode = "read-only"
	}
	view := ManualSessionCreationView{OperationID: "team-create-" + id, WorkspaceID: workspace, Scope: "project", WorkspaceRoot: root,
		Ref: session.SessionRef{HostID: localDesktopHostID, SessionID: id}, TopicID: "team-" + id, Settings: settings}
	tab, err := a.reserveManualSessionTab(ctx, view, func(string) {})
	if err != nil {
		return nil, err
	}
	retained := false
	defer func() {
		if retained {
			return
		}
		// Cancellation may arrive after the canonical builder published its
		// Controller. Join it before allowing Ensure to forget this admission.
		runtime := &desktopTeamRuntime{app: a, ref: ref, sessionID: id}
		resultErr = errors.Join(resultErr, runtime.Close(context.Background()))
		unlock := a.lockRuntimeMutation("discard failed team seat")
		defer unlock()
		a.mu.Lock()
		if a.tabs[tab.ID] == tab {
			a.markTabRemovedLocked(tab)
			delete(a.tabs, tab.ID)
			a.removeTabOrderLocked(tab.ID)
			a.saveTabsLocked()
		}
		a.mu.Unlock()
	}()
	a.mu.Lock()
	tab.teamSeat = team.Seat{Service: service, Ref: ref}
	a.mu.Unlock()
	if err := a.startManualSessionTab(ctx, tab, func(string) {}); err != nil {
		return nil, err
	}
	// Transfer to the existing detached registry. Background opens never select
	// a workspace, tab or browser surface.
	unlock := a.lockRuntimeMutation("background team seat")
	a.mu.Lock()
	tab.PendingCreateOperationID = ""
	if a.tabs[tab.ID] == tab {
		if !a.detachRuntimeForReplacementLocked(tab) {
			a.mu.Unlock()
			unlock()
			return nil, errors.New("cannot retain team session runtime")
		}
		delete(a.tabs, tab.ID)
		a.removeTabOrderLocked(tab.ID)
		a.saveTabsLocked()
	}
	a.mu.Unlock()
	unlock()
	retained = true
	return &desktopTeamRuntime{app: a, ref: ref, sessionID: id}, nil
}

func teamRuntimeRoot(store teamroster.Store, r *teamroster.Roster, ref team.SeatRef) (string, error) {
	member, ok := r.Agent(ref.Agent)
	if !ok {
		return "", teamroster.ErrUnknownAgent
	}
	root := member.Folder
	if ref.Group != "" {
		group, ok := r.Group(ref.Group)
		if !ok {
			return "", teamroster.ErrUnknownGroup
		}
		if group.Folder != "" {
			root = group.Folder
		}
	}
	if root == "" {
		root = filepath.Join(store.Dir, "workspaces", ref.Agent)
		if err := os.MkdirAll(root, 0700); err != nil {
			return "", err
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("team working directory is not a directory")
	}
	return root, nil
}

// Every access resolves the session's current owner after navigation/rebuild.
type desktopTeamRuntime struct {
	app       *App
	ref       team.SeatRef
	sessionID string
	closeMu   sync.Mutex
	closing   <-chan struct{}
}

func (r *desktopTeamRuntime) owner() *WorkspaceTab {
	r.app.mu.RLock()
	defer r.app.mu.RUnlock()
	return r.app.liveRuntimeTabMatchingLocked(nil, sessionRoute(r.sessionID))
}

func (r *desktopTeamRuntime) Deliver(d team.Delivery) error {
	r.app.runtimeAdmissionMu.RLock()
	defer r.app.runtimeAdmissionMu.RUnlock()
	roster, err := desktopTeamStore().Load()
	if err != nil {
		return err
	}
	path, err := teamSessionPath(&roster, r.ref)
	if err != nil {
		return err
	}
	if path != r.BindingKey() {
		return errors.New("team seat binding changed; retry delivery")
	}
	tab := r.owner()
	if tab == nil {
		return errors.New("team runtime is unavailable")
	}
	c, ok := r.app.controllerForTab(tab).(interface{ DeliverTeam(team.Delivery) error })
	if !ok {
		return fmt.Errorf("team controller is unavailable")
	}
	return c.DeliverTeam(d)
}
func (r *desktopTeamRuntime) Cancel() {
	if tab := r.owner(); tab != nil {
		if c := r.app.controllerForTab(tab); c != nil {
			c.Cancel()
		}
	}
}
func (r *desktopTeamRuntime) Idle() bool {
	tab := r.owner()
	if tab == nil {
		return true
	}
	c := r.app.controllerForTab(tab)
	if controllerHasActiveRuntimeWork(c) {
		return false
	}
	if q, ok := c.(interface {
		InboxSnapshot() sessioninbox.InboxSnapshot
	}); ok && len(q.InboxSnapshot().Items) > 0 {
		return false
	}
	return true
}
func (r *desktopTeamRuntime) Close(ctx context.Context) error {
	_, err := r.close(ctx, false, false)
	return err
}
func (r *desktopTeamRuntime) BindingKey() string { return sessionRoute(r.sessionID) }
func (r *desktopTeamRuntime) CloseIdle(ctx context.Context) (bool, error) {
	return r.close(ctx, true, false)
}
func (r *desktopTeamRuntime) CloseRebound(ctx context.Context) (bool, error) {
	return r.close(ctx, true, true)
}
func (r *desktopTeamRuntime) close(ctx context.Context, idle, rebound bool) (bool, error) {
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	if r.closing != nil {
		return r.awaitClosed(ctx)
	}
	unlock := r.app.lockRuntimeMutation("close team seat")
	if err := ctx.Err(); err != nil {
		unlock()
		return false, err
	}
	tab := r.owner()
	if tab == nil {
		unlock()
		return true, nil
	}
	r.app.mu.RLock()
	visible := r.app.tabs[tab.ID] == tab
	r.app.mu.RUnlock()
	if idle && (visible && !rebound || !r.Idle()) {
		unlock()
		return false, nil
	}
	if err := r.app.snapshotTab(tab); err != nil {
		unlock()
		return false, err
	}
	if c, ok := r.app.controllerForTab(tab).(interface{ Closed() <-chan struct{} }); ok {
		r.closing = c.Closed()
	}
	r.app.closeTabRuntimeAdmissionHeld(tab)
	r.app.mu.Lock()
	r.app.unregisterDetachedRuntimeLocked(tab)
	tab.Ctrl = nil
	r.app.mu.Unlock()
	// A finalizing turn may deliver its final reply through another runtime's
	// admission. Never join that work while holding the global mutation gate.
	unlock()
	return r.awaitClosed(ctx)
}

func (r *desktopTeamRuntime) awaitClosed(ctx context.Context) (bool, error) {
	if r.closing == nil {
		return true, nil
	}
	select {
	case <-r.closing:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
