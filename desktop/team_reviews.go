package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"deepzora/internal/control"
	"deepzora/internal/memory"
	"deepzora/internal/secrets"
	"deepzora/internal/session"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

const teamReviewDelay = 2 * time.Second

type teamReviewJob struct {
	ref team.SeatRef
	due time.Time
}
type teamMemoryReviews struct {
	app     *App
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	pending map[string]teamReviewJob
	running *team.SeatRef
	stopRun context.CancelFunc
	ask     func(context.Context, team.SeatRef, string, string, string) (team.Reviewer, func(), error)
}

func (a *App) scheduleTeamMemoryReview(ref team.SeatRef) {
	a.teamState.mu.Lock()
	defer a.teamState.mu.Unlock()
	if a.shuttingDown.Load() || a.teamState.closing || a.teamState.reviewsPaused {
		return
	}
	roster, err := desktopTeamStore().Load()
	if err != nil || !roster.Prefs.MemoryReview {
		return
	}
	r := a.teamState.reviews
	if r == nil {
		ctx, cancel := context.WithCancel(a.bootContext())
		r = &teamMemoryReviews{app: a, ctx: ctx, cancel: cancel, done: make(chan struct{}), pending: map[string]teamReviewJob{}, ask: a.teamMemoryReviewer}
		a.teamState.reviews = r
		go r.loop()
	}
	r.schedule(ref)
}
func (r *teamMemoryReviews) schedule(ref team.SeatRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() == nil {
		r.pending[teamReviewKey(ref)] = teamReviewJob{ref: ref, due: time.Now().Add(teamReviewDelay)}
	}
}
func (r *teamMemoryReviews) loop() {
	defer close(r.done)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case now := <-ticker.C:
			var job *teamReviewJob
			r.mu.Lock()
			for key, queued := range r.pending {
				if !queued.due.After(now) {
					copy := queued
					job = &copy
					delete(r.pending, key)
					break
				}
			}
			r.mu.Unlock()
			if job != nil {
				if err := r.run(job.ref); err != nil && r.ctx.Err() == nil {
					slog.Warn("desktop: Team memory review", "agent", job.ref.Agent, "group", job.ref.Group, "err", secrets.RedactError(err))
				}
			}
		}
	}
}
func (r *teamMemoryReviews) busy(agent, group string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running != nil && (agent != "" && r.running.Agent == agent || group != "" && r.running.Group == group)
}
func (r *teamMemoryReviews) cancelTarget(agent, group string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, job := range r.pending {
		if agent != "" && job.ref.Agent == agent || group != "" && job.ref.Group == group {
			delete(r.pending, key)
		}
	}
	if r.running != nil && (agent != "" && r.running.Agent == agent || group != "" && r.running.Group == group) && r.stopRun != nil {
		r.stopRun()
	}
}
func (a *App) stopTeamMemoryReviews() {
	a.teamState.mu.Lock()
	r := a.teamState.reviews
	a.teamState.reviews = nil
	a.teamState.mu.Unlock()
	if r != nil {
		r.cancel()
		<-r.done
	}
}
func (a *App) cancelTeamMemoryReviewTarget(agent, group string) {
	a.teamState.mu.Lock()
	r := a.teamState.reviews
	a.teamState.mu.Unlock()
	if r != nil {
		r.cancelTarget(agent, group)
	}
}

func (a *App) scheduleTeamMemoryReviewForTab(tabID string) {
	a.mu.RLock()
	tab := a.tabByEventSinkIDLocked(tabID)
	var ref team.SeatRef
	bound := tab != nil && tab.teamSeat != nil
	if bound {
		_, ref = tab.teamSeat.Team()
	}
	a.mu.RUnlock()
	if bound {
		a.scheduleTeamMemoryReview(ref)
	}
}

func (r *teamMemoryReviews) run(ref team.SeatRef) error {
	roster, err := desktopTeamStore().Load()
	if err != nil || !roster.Prefs.MemoryReview {
		return err
	}
	member, ok := roster.Agent(ref.Agent)
	if !ok {
		return nil
	}
	if ref.Group != "" {
		g, ok := roster.Group(ref.Group)
		if !ok || g.Archived || !g.IsMember(ref.Agent) {
			return nil
		}
	}
	if err := r.app.requireTeamTargetIdle(ref.Agent, ref.Group); err != nil {
		if errors.Is(err, errTeamBusy) {
			r.schedule(ref)
			return nil
		}
		return err
	}
	ctx, cancel := context.WithTimeout(r.ctx, 2*time.Minute)
	defer cancel()
	r.mu.Lock()
	r.running = &ref
	r.stopRun = cancel
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.running = nil; r.stopRun = nil; r.mu.Unlock() }()
	h, err := r.app.localTeamHost()
	if err != nil {
		return err
	}
	return h.WithWriter(ctx, func(ctx context.Context, svc *team.Service) error {
		return r.reviewWithWriter(ctx, svc, roster, *member, ref)
	})
}

func (r *teamMemoryReviews) reviewWithWriter(ctx context.Context, svc *team.Service, roster teamroster.Roster, member teamroster.Agent, ref team.SeatRef) error {
	state, err := loadTeamReviewState(roster.Prefs.MemoryReviewStartedAt)
	if err != nil {
		return err
	}
	lines, store, advance, more, err := r.evidence(ctx, roster, ref, &state)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return advance()
	}
	if err := store.RecoverBatch(ctx); err != nil {
		return err
	}
	path := member.SessionPath
	if ref.Group != "" {
		g, _ := roster.Group(ref.Group)
		path = g.MemberSessions[ref.Agent]
	}
	id, ok := parseSessionRoute(path)
	if !ok {
		return nil
	}
	root, err := teamRuntimeRoot(desktopTeamStore(), &roster, ref)
	if err != nil {
		return err
	}
	ask, close, err := r.ask(ctx, ref, id, root, roster.Prefs.MemoryReviewModel)
	var report team.ReviewReport
	if err == nil {
		defer close()
		checked := func(ctx context.Context, system, prompt string) (string, error) {
			answer, err := ask(ctx, system, prompt)
			if err != nil {
				return "", err
			}
			latest, err := desktopTeamStore().Load()
			if err != nil {
				return "", err
			}
			if !latest.Prefs.MemoryReview || !latest.Prefs.MemoryReviewStartedAt.Equal(roster.Prefs.MemoryReviewStartedAt) || latest.Prefs.MemoryReviewModel != roster.Prefs.MemoryReviewModel {
				return "", context.Canceled
			}
			return answer, nil
		}
		report, err = team.ReviewMemory(ctx, store, lines, checked)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	status := TeamMemoryReviewStatus{AgentID: ref.Agent, GroupID: ref.Group, At: time.Now().UTC(), Outcome: "applied", Added: report.Added, Edited: report.Edited, Archived: report.Removed}
	if err != nil {
		status.Outcome = "failed"
		status.Error = secrets.RedactError(err)
	}
	state.Status = append(state.Status, status)
	if err != nil {
		_ = saveTeamReviewState(state)
		r.app.emitTeamChanged()
		return err
	}
	if err := advance(); err != nil {
		return err
	}
	if report != (team.ReviewReport{}) {
		note := team.MemoryReviewNote(report)
		if ref.Group != "" {
			if err := svc.RecordMemoryChange(ctx, ref.Agent, ref.Group, note); err != nil {
				return err
			}
		} else {
			r.app.notifyPrivateTeamMemory(path, note)
		}
	}
	r.app.emitTeamChanged()
	if more {
		r.schedule(ref)
	}
	return nil

}

func (r *teamMemoryReviews) evidence(ctx context.Context, roster teamroster.Roster, ref team.SeatRef, state *teamReviewState) ([]team.ReviewLine, memory.Store, func() error, bool, error) {
	if ref.Group != "" {
		last := state.Groups[ref.Group]
		var lines []team.ReviewLine
		var through int64
		more := false
		err := desktopTeamStore().ScanLines(ref.Group, func(line teamroster.GroupLine) {
			if line.Seq <= last || more {
				return
			}
			if line.Kind == teamroster.LinePost && line.From == teamroster.ByUser && line.Source == "" && !line.At.Before(roster.Prefs.MemoryReviewStartedAt) {
				if len(lines) == 16 {
					more = true
					return
				}
				lines = append(lines, team.ReviewLine{From: "user", Text: line.Text, User: true})
			}
			through = line.Seq
		})
		return lines, team.SharedStore(desktopTeamStore().Dir, team.TierGroup, ref.Group), func() error { state.Groups[ref.Group] = through; return saveTeamReviewState(*state) }, more, err
	}
	member, _ := roster.Agent(ref.Agent)
	id, ok := parseSessionRoute(member.SessionPath)
	store := memory.Store{Dir: team.AgentMemoryDir(desktopTeamStore().Dir, ref.Agent)}
	if !ok {
		return nil, store, func() error { return nil }, false, nil
	}
	position := state.Sessions[id]
	page, err := r.app.desktopSessionService("").Query().ReadUserEvidenceAfter(ctx, session.SessionRef{HostID: localDesktopHostID, SessionID: id}, position.Through, position.Generation, roster.Prefs.MemoryReviewStartedAt, 16)
	if err != nil {
		return nil, store, nil, false, err
	}
	var lines []team.ReviewLine
	for _, line := range page.Lines {
		lines = append(lines, team.ReviewLine{From: "user", Text: line.Text, User: true})
	}
	return lines, store, func() error {
		state.Sessions[id] = teamReviewPosition{Through: page.Through, Generation: page.Generation}
		return saveTeamReviewState(*state)
	}, page.More, nil
}
func (a *App) notifyPrivateTeamMemory(path, note string) {
	a.mu.RLock()
	tab := a.liveRuntimeTabMatchingLocked(nil, path)
	var ctrl *control.Controller
	if tab != nil {
		ctrl, _ = tab.Ctrl.(*control.Controller)
	}
	a.mu.RUnlock()
	if ctrl != nil {
		ctrl.NotifyTeamMemory(note)
	}
}
