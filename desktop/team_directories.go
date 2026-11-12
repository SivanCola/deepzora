package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"deepzora/desktop/internal/workspacestate"
	"deepzora/internal/session"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

// Directory changes fork at a proven completed turn, because the existing
// session header's workspace is immutable. The original remains a history
// parent; no session files or user messages are rewritten in place.
func (a *App) migrateTeamDirectory(ctx context.Context, r *teamroster.Roster, ref team.SeatRef, requestID string) error {
	path, err := teamSessionPath(r, ref)
	if err != nil {
		return err
	}
	root, err := teamRuntimeRoot(desktopTeamStore(), r, ref)
	if err != nil || path == "" {
		return err
	}
	if err := a.requireTeamTargetIdle(ref.Agent, ref.Group); err != nil {
		return err
	}
	id, ok := parseSessionRoute(path)
	if !ok {
		return teamroster.ErrInvalid
	}
	service := a.desktopSessionService("")
	source := session.SessionRef{HostID: localDesktopHostID, SessionID: id}
	info, err := service.Query().Stat(ctx, source)
	if err != nil && !errors.Is(err, session.ErrSessionNotFound) {
		return err
	}
	if err == nil && sameDesktopPath(info.CWD, root) {
		return nil
	}
	if release, err := a.beginWorkspaceRuntimeAdmission(root); err != nil {
		return err
	} else {
		defer release()
	}
	digest := sha256.Sum256([]byte(requestID + "\x00" + path + "\x00" + root))
	childID := "desktop-team-" + ref.Agent + "-" + ref.Group + "-dir-" + hex.EncodeToString(digest[:12])
	if err == nil && info.Turns > 0 {
		targets, err := service.ForkTargetSetFor(ctx, source)
		if err != nil {
			return err
		}
		if !targets.Verifiable || len(targets.Targets) == 0 {
			return &session.ForkUnavailableError{Reason: session.ForkHistoryUnverifiable}
		}
		last := targets.Targets[len(targets.Targets)-1]
		if !last.Available {
			return &session.ForkUnavailableError{TurnID: last.TurnID, Reason: last.Reason}
		}
		workspace, err := a.ensureDesktopWorkspace(ctx, "project", root)
		if err != nil {
			return err
		}
		operation := "team-create-" + childID
		if err := a.workspaceRegistry().BeginCreate(ctx, workspacestate.PendingCreate{OperationID: operation, WorkspaceID: workspace, SessionID: childID, ParentSessionID: id}); err != nil {
			return err
		}
		if _, err := service.CreateFork(ctx, session.ForkRequest{Source: source, ChildID: childID, OperationID: operation, TurnID: last.TurnID, BoundarySequence: last.BoundarySequence, CWD: root}); err != nil {
			return err
		}
		if err := a.validateDesktopWorkspaceMembership(ctx, workspace, session.SessionRef{HostID: localDesktopHostID, SessionID: childID}); err != nil {
			return err
		}
		if err := a.workspaceRegistry().AttachSession(ctx, operation, workspace, childID, ""); err != nil {
			return err
		}
	}
	if ref.Group == "" {
		member, _ := r.Agent(ref.Agent)
		member.Parts = append(member.Parts, teamroster.Part{SessionPath: path, EndedAt: time.Now().UTC()})
		member.SessionPath = sessionRoute(childID)
	} else {
		group, _ := r.Group(ref.Group)
		group.MemberSessions[ref.Agent] = sessionRoute(childID)
	}
	return nil
}

func (a *App) migrateTeamAgentDirectories(ctx context.Context, r *teamroster.Roster, agent, requestID string) error {
	if err := a.migrateTeamDirectory(ctx, r, team.SeatRef{Agent: agent}, requestID); err != nil {
		return err
	}
	for _, group := range r.Groups {
		if group.IsMember(agent) && group.Folder == "" && !group.Archived {
			if err := a.migrateTeamDirectory(ctx, r, team.SeatRef{Agent: agent, Group: group.ID}, requestID); err != nil {
				return err
			}
		}
	}
	return nil
}
