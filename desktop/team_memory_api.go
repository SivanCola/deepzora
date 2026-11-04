package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"deepzora/internal/memory"
	"deepzora/internal/secrets"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type TeamMemoryTarget struct {
	Tier    string `json:"tier"`
	AgentID string `json:"agentId,omitempty"`
	GroupID string `json:"groupId,omitempty"`
}
type TeamMemoryView struct {
	RosterRevision uint64       `json:"rosterRevision"`
	Facts          []MemoryFact `json:"facts"`
}
type SaveTeamMemoryRequest struct {
	Target                 TeamMemoryTarget `json:"target"`
	ExpectedRosterRevision uint64           `json:"expectedRosterRevision"`
	RequestID              string           `json:"requestId"`
	Fact                   MemoryFact       `json:"fact"`
	Archive                bool             `json:"archive"`
}
type TeamMemoryResult struct {
	Outcome string     `json:"outcome"`
	Reason  string     `json:"reason,omitempty"`
	Message string     `json:"message,omitempty"`
	Fact    MemoryFact `json:"fact"`
}

func teamUserMemoryStore(r teamroster.Roster, target TeamMemoryTarget) (memory.Store, error) {
	switch target.Tier {
	case "private":
		if target.GroupID != "" {
			return memory.Store{}, teamroster.ErrInvalid
		}
		if _, ok := r.Agent(target.AgentID); !ok {
			return memory.Store{}, teamroster.ErrUnknownAgent
		}
		return memory.Store{Dir: team.AgentMemoryDir(desktopTeamStore().Dir, target.AgentID)}, nil
	case "team":
		if target.GroupID != "" || target.AgentID != "" {
			return memory.Store{}, teamroster.ErrInvalid
		}
		return team.SharedStore(desktopTeamStore().Dir, team.TierTeam, ""), nil
	case "group":
		if target.AgentID != "" {
			return memory.Store{}, teamroster.ErrInvalid
		}
		if _, ok := r.Group(target.GroupID); !ok {
			return memory.Store{}, teamroster.ErrUnknownGroup
		}
		return team.SharedStore(desktopTeamStore().Dir, team.TierGroup, target.GroupID), nil
	default:
		return memory.Store{}, teamroster.ErrInvalid
	}
}

func (a *App) GetTeamMemory(target TeamMemoryTarget) (TeamMemoryView, error) {
	r, err := desktopTeamStore().Load()
	if err != nil {
		return TeamMemoryView{}, err
	}
	store, err := teamUserMemoryStore(r, target)
	if err != nil {
		return TeamMemoryView{}, err
	}
	// Only the lease owner recovers an interrupted transaction.
	h, err := a.localTeamHost()
	if err != nil {
		return TeamMemoryView{}, err
	}
	if err := h.Claim(a.bootContext()); err != nil {
		return TeamMemoryView{}, err
	}
	err = h.WithWriter(a.bootContext(), func(ctx context.Context, _ *team.Service) error { return store.RecoverBatch(ctx) })
	if err != nil {
		return TeamMemoryView{}, err
	}
	view := TeamMemoryView{RosterRevision: r.Revision, Facts: []MemoryFact{}}
	for _, fact := range store.ListAll() {
		view.Facts = append(view.Facts, memoryFactView(fact))
	}
	return view, nil
}

func (a *App) SaveTeamMemory(req SaveTeamMemoryRequest) (TeamMemoryResult, error) {
	result := TeamMemoryResult{Outcome: "rejected"}
	h, err := a.localTeamHost()
	if err != nil {
		return result, err
	}
	if err = h.Claim(a.bootContext()); err == nil {
		err = h.WithWriter(a.bootContext(), func(ctx context.Context, _ *team.Service) error {
			return desktopTeamStore().InspectAt(ctx, req.ExpectedRosterRevision, func(r teamroster.Roster) error {
				store, err := teamUserMemoryStore(r, req.Target)
				if err != nil {
					return err
				}
				fact := memory.Memory{ID: req.Fact.ID, Name: req.Fact.Name, Description: req.Fact.Description, Body: req.Fact.Body, Type: memory.NormalizeType(req.Fact.Type), Activation: memory.ActivationRelevant}
				if !req.Archive && (strings.TrimSpace(fact.Description) == "" || strings.TrimSpace(fact.Body) == "") {
					return teamroster.ErrInvalid
				}
				text := fact.Name + "\n" + fact.Description + "\n" + fact.Body
				if secrets.RedactCredentials(text) != text || secrets.Masker().String(text) != text {
					return errors.New("credentials cannot be stored in Team memory")
				}
				saved, err := store.ApplyUserChange(ctx, req.RequestID, teamMutationDigest(req), memory.BatchChange{Memory: fact, ExpectedRevision: req.Fact.Revision, Remove: req.Archive})
				result.Fact = memoryFactView(saved)
				return err
			})
		})
	}
	if err != nil {
		result.Reason, result.Message = teamMutationReason(err), secrets.RedactError(err)
		if errors.Is(err, memory.ErrRequestReused) {
			result.Reason = "request_reused"
		}
		if errors.Is(err, memory.ErrRevisionConflict) {
			result.Reason = "conflict"
		}
		return result, nil
	}
	result.Outcome = "applied"
	a.emitTeamChanged()
	return result, nil
}

type SaveTeamMemoryPreferencesRequest struct {
	ExpectedRevision uint64 `json:"expectedRevision"`
	RequestID        string `json:"requestId"`
	Enabled          bool   `json:"enabled"`
	Model            string `json:"model"`
}

func (a *App) SaveTeamMemoryPreferences(req SaveTeamMemoryPreferencesRequest) (TeamMutationResult, error) {
	a.teamState.reviewPrefsMu.Lock()
	defer a.teamState.reviewPrefsMu.Unlock()
	a.teamState.mu.Lock()
	a.teamState.reviewsPaused = true
	a.teamState.mu.Unlock()
	defer func() { a.teamState.mu.Lock(); a.teamState.reviewsPaused = false; a.teamState.mu.Unlock() }()
	a.stopTeamMemoryReviews()
	return a.mutateTeam(req.ExpectedRevision, req.RequestID, req, func(_ context.Context, r *teamroster.Roster) (string, error) {
		if req.Enabled && (!r.Prefs.MemoryReview || r.Prefs.MemoryReviewModel != req.Model) {
			r.Prefs.MemoryReviewStartedAt = time.Now().UTC()
		}
		r.Prefs.MemoryReview, r.Prefs.MemoryReviewModel = req.Enabled, req.Model
		return "memory", nil
	})
}

func (a *App) GetTeamMemoryReviewStatus() ([]TeamMemoryReviewStatus, error) {
	r, err := desktopTeamStore().Load()
	if err != nil {
		return nil, err
	}
	state, err := loadTeamReviewState(r.Prefs.MemoryReviewStartedAt)
	return state.Status, err
}
