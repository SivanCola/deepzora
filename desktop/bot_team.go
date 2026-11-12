package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"deepzora/desktop/internal/botchannels"
	"deepzora/desktop/internal/workspacestate"
	"deepzora/internal/agentpreset"
	"deepzora/internal/boot"
	"deepzora/internal/control"
	"deepzora/internal/event"
	"deepzora/internal/session"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type BotMemberView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	Permission string `json:"permission"`
	Folder     string `json:"folder"`
}

func botTeamMember(id string) (teamroster.Agent, error) {
	r, err := desktopTeamStore().Load()
	if err != nil {
		return teamroster.Agent{}, err
	}
	m, ok := r.Agent(id)
	if !ok {
		return teamroster.Agent{}, errors.New("bound team member is archived or deleted; restore it in Team settings")
	}
	return *m, nil
}

func (a *App) botMembers() ([]BotMemberView, error) {
	r, err := desktopTeamStore().Load()
	if err != nil {
		return nil, err
	}
	list := []BotMemberView{}
	for _, m := range r.Agents {
		list = append(list, BotMemberView{ID: m.ID, Name: m.Name, Model: m.InitialModel, Permission: m.Permission, Folder: m.Folder})
	}
	return list, nil
}

// Migration is idempotent across the two files: member identity and fork
// operation derive only from the existing bot ID. A crash never creates a
// duplicate member, and original sessions/config fields remain untouched.
func (a *App) migrateBotMembersLocked() error {
	entries, err := a.botStore().Read()
	if err != nil {
		return err
	}
	if slices.ContainsFunc(entries, func(c botchannels.Saved) bool { return c.MemberID == "" }) {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(a.botStore().Dir, "connections.pre-members.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil && !os.IsExist(err) {
			return err
		}
		if err == nil {
			if err := botchannels.RestrictCredentialFile(f); err != nil {
				_ = f.Close()
				_ = os.Remove(f.Name())
				return err
			}
			_, writeErr := f.Write(data)
			syncErr := f.Sync()
			closeErr := f.Close()
			if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
				return err
			}
		}
	}
	for _, entry := range entries {
		if entry.MemberID != "" {
			continue
		}
		if err := a.stopBotChannel(entry.ID); err != nil {
			return err
		}
		unlock, err := a.lockBotAccount(entry)
		if err != nil {
			return err
		}
		memberID, err := a.createLegacyBotMember(entry)
		if err == nil {
			err = a.botStore().Edit(func(current []botchannels.Saved) ([]botchannels.Saved, error) {
				idx := slices.IndexFunc(current, func(c botchannels.Saved) bool { return c.ID == entry.ID })
				if idx < 0 || current[idx] != entry {
					return nil, errors.New("bot configuration changed during migration")
				}
				current[idx].MemberID = memberID
				current[idx].Mode = "member"
				current[idx].LegacySessionID = entry.SessionID
				return current, nil
			})
		}
		unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) createLegacyBotMember(c botchannels.Saved) (string, error) {
	sum := sha256.Sum256([]byte("bot-member:" + c.ID))
	id := "a-" + hex.EncodeToString(sum[:8])
	var result string
	err := a.withTeamWriter(func(ctx context.Context, s *team.Service) error {
		return s.Configure(ctx, func() error {
			r, err := desktopTeamStore().Load()
			if err != nil {
				return err
			}
			if _, ok := r.Agent(id); ok {
				result = id
				return nil
			}
			for _, m := range r.RetiredAgents {
				if m.ID == id {
					return errors.New("migrated bot member was retired; restore it before retrying migration")
				}
			}
			if len(r.Agents) >= teamroster.MaxAgents {
				return teamroster.ErrAgentCapacity
			}
			m := teamroster.Agent{ID: id, Name: string([]rune(c.Name)[:min(len([]rune(c.Name)), 30)]), InitialModel: c.ModelRef, Permission: "read-only", CreatedBy: teamroster.ByUser, CreatedAt: time.Now().UTC(), Main: len(r.Agents) == 0}
			for _, other := range r.Agents {
				if strings.EqualFold(m.Name, other.Name) {
					m.Name += " · " + id[len(id)-6:]
					break
				}
			}
			if err := a.inheritLegacyBotHistory(ctx, c, &m); err != nil {
				return err
			}
			if m.Folder == "" && c.WorkspaceID != "" {
				state, err := a.workspaceRegistry().Load(ctx)
				if err != nil {
					return err
				}
				m.Folder = state.Workspaces[c.WorkspaceID].Root
			}
			if m.InitialModel == "" {
				cfg, _, err := a.loadDesktopUserConfigForView()
				if err != nil {
					return err
				}
				m.InitialModel = cfg.DefaultModel
			}
			_, err = desktopTeamStore().UpdateAt(ctx, r.Revision, func(current *teamroster.Roster) error {
				current.Agents = append(current.Agents, m)
				return current.Validate()
			})
			if err == nil {
				result = id
			}
			return err
		})
	})
	return result, err
}

func (a *App) prepareBotMember(ctx context.Context, id string) (*botChannelRun, error) {
	m, err := botTeamMember(id)
	if err != nil {
		return nil, err
	}
	h, err := a.localTeamHost()
	if err != nil {
		return nil, err
	}
	if err = h.Claim(ctx); err != nil {
		return nil, err
	}
	if _, err = h.Ensure(ctx, team.SeatRef{Agent: id}); err != nil {
		return nil, err
	}
	m, err = botTeamMember(id)
	if err != nil {
		return nil, err
	}
	sessionID, ok := parseSessionRoute(m.SessionPath)
	if !ok {
		return nil, errors.New("member session is unavailable")
	}
	a.mu.RLock()
	tab := a.liveRuntimeTabMatchingLocked(nil, m.SessionPath)
	a.mu.RUnlock()
	if tab == nil {
		return nil, errBotSessionUnavailable
	}
	if _, err := a.applyTeamPresetIfIdle(ctx, tab); err != nil {
		return nil, err
	}
	if err := a.reconcileTeamPermissions(); err != nil {
		return nil, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	tab = a.liveRuntimeTabMatchingLocked(nil, m.SessionPath)
	if tab == nil || tab.teamSeat == nil {
		return nil, errBotSessionUnavailable
	}
	_, seat := tab.teamSeat.Team()
	if seat.Agent != id || seat.Group != "" {
		return nil, errors.New("member seat identity mismatch")
	}
	c, ok := tab.Ctrl.(*control.Controller)
	if !ok {
		return nil, errBotSessionUnavailable
	}
	return &botChannelRun{config: botchannels.Saved{Config: botchannels.Config{SessionID: sessionID}}, ctrl: c, sink: tab.sink, tabID: tab.ID, generation: tab.SessionGeneration}, nil
}

func (a *App) OpenBotMemberHistory(id string) error {
	entries, err := a.botStore().Read()
	if err != nil {
		return err
	}
	for _, c := range entries {
		if c.ID == id && c.LegacySessionID != "" {
			_, err := a.OpenSession(session.SessionRef{HostID: localDesktopHostID, SessionID: c.LegacySessionID})
			return err
		}
	}
	return fmt.Errorf("original bot history is unavailable")
}

func (r *botChannelRun) ownsBotEvent(e event.Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.SubmissionID != "" && r.routes[e.SubmissionID] == e.SessionID && e.SessionID != "" {
		if e.TurnID != "" {
			r.turnRoutes[e.TurnID] = e.SessionID
		}
		return true
	}
	return e.SubmissionID == "" && e.TurnID != "" && e.SessionID != "" && r.turnRoutes[e.TurnID] == e.SessionID
}

// Keep the original conversation and fork only a verified completed boundary.
func (a *App) inheritLegacyBotHistory(ctx context.Context, c botchannels.Saved, m *teamroster.Agent) error {
	if c.SessionID != "" {
		m.Parts = []teamroster.Part{{SessionPath: sessionRoute(c.SessionID), EndedAt: time.Now().UTC()}}
		ref := session.SessionRef{HostID: localDesktopHostID, SessionID: c.SessionID}
		service := a.desktopSessionService("")
		info, statErr := service.Query().Stat(ctx, ref)
		if statErr == nil {
			m.Folder = info.CWD
			snapshot, err := service.Query().Snapshot(ctx, ref)
			if err != nil {
				return err
			}
			if snapshot.Projection.ModelRef != "" {
				m.InitialModel = snapshot.Projection.ModelRef
			}
			if p := snapshot.Projection.PermissionPreset; p != "" {
				m.Permission = normalizeToolApprovalMode(p)
			}
			a.mu.RLock()
			for _, tab := range a.runtimeTabsLocked() {
				if tab.SessionID == c.SessionID {
					m.Permission = normalizeToolApprovalMode(tab.toolApprovalMode)
					break
				}
			}
			a.mu.RUnlock()
			preset, err := agentpreset.Resolve(boot.AgentPresetDirs(m.Folder), snapshot.Projection.AgentPreset)
			if err != nil {
				return err
			}
			m.Preset = &preset
			if info.Turns > 0 {
				targets, err := service.ForkTargetSetFor(ctx, ref)
				if err != nil {
					return err
				}
				if !targets.Verifiable || len(targets.Targets) == 0 {
					return errors.New("bot history cannot be verified; original history retained, migration stopped")
				}
				last := targets.Targets[len(targets.Targets)-1]
				if !last.Available {
					return errors.New("finish the bot conversation before migrating its history")
				}
				childID := "desktop-team-" + m.ID + "-bot-migration"
				workspace, err := a.ensureDesktopWorkspace(ctx, "project", m.Folder)
				if err != nil {
					return err
				}
				op := "team-create-" + childID
				if err := a.workspaceRegistry().BeginCreate(ctx, workspacestate.PendingCreate{OperationID: op, WorkspaceID: workspace, SessionID: childID, ParentSessionID: c.SessionID}); err != nil {
					return err
				}
				if _, err := service.CreateFork(ctx, session.ForkRequest{Source: ref, ChildID: childID, OperationID: op, TurnID: last.TurnID, BoundarySequence: last.BoundarySequence, CWD: m.Folder}); err != nil {
					return err
				}
				if err := a.workspaceRegistry().AttachSession(ctx, op, workspace, childID, ""); err != nil {
					return err
				}
				m.SessionPath = sessionRoute(childID)
			}
		} else if !errors.Is(statErr, session.ErrSessionNotFound) {
			return statErr
		}
	}
	return nil
}
