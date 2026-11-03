package main

import (
	"context"
	"deepzora/internal/config"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

type TeamSecretMetadata struct {
	Name      string    `json:"name"`
	Purpose   string    `json:"purpose"`
	Agents    []string  `json:"agents"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type TeamSecretsView struct {
	Available      bool                           `json:"available"`
	Revision       uint64                         `json:"revision"`
	RosterRevision uint64                         `json:"rosterRevision"`
	Secrets        []TeamSecretMetadata           `json:"secrets"`
	Requests       []teamroster.TeamSecretRequest `json:"requests"`
	RetiredCount   int                            `json:"retiredCount"`
}

func (a *App) GetTeamSecrets() (TeamSecretsView, error) {
	view := TeamSecretsView{Available: teamSecretMaskingReady(), Secrets: []TeamSecretMetadata{}, Requests: []teamroster.TeamSecretRequest{}}
	if !view.Available {
		return view, nil
	}
	roster, err := desktopTeamStore().Load()
	if err != nil {
		return view, err
	}
	view.RosterRevision = roster.Revision
	book, err := desktopTeamStore().TeamSecrets()
	if err != nil {
		return view, err
	}
	view.Revision = book.Revision
	view.Requests = book.Requests
	view.RetiredCount = len(book.Retired)
	for _, sc := range book.Secrets {
		view.Secrets = append(view.Secrets, TeamSecretMetadata{Name: sc.Name, Purpose: sc.Purpose, Agents: slices.Clone(sc.Agents), UpdatedAt: sc.UpdatedAt})
	}
	return view, nil
}

type SaveTeamSecretRequest struct {
	ExpectedRevision       uint64   `json:"expectedRevision"`
	ExpectedRosterRevision uint64   `json:"expectedRosterRevision"`
	RequestID              string   `json:"requestId"`
	Operation              string   `json:"operation"` // save, authorize, delete, decline, forget_retired
	Name                   string   `json:"name"`
	Purpose                string   `json:"purpose"`
	Value                  string   `json:"value"`
	Agents                 []string `json:"agents"`
	SecretRequestID        string   `json:"secretRequestId"`
	ConfirmForget          bool     `json:"confirmForget"`
}
type TeamSecretResult struct {
	Outcome string          `json:"outcome"`
	Reason  string          `json:"reason,omitempty"`
	Message string          `json:"message,omitempty"`
	View    TeamSecretsView `json:"view"`
}

type TeamSecretSeatView struct {
	AgentID string `json:"agentId"`
	GroupID string `json:"groupId"`
}

func (a *App) TeamSecretSeatForTab(tabID string) TeamSecretSeatView {
	a.mu.RLock()
	defer a.mu.RUnlock()
	tab := a.tabByEventSinkIDLocked(tabID)
	if tab == nil || tab.teamSeat == nil {
		return TeamSecretSeatView{}
	}
	_, ref := tab.teamSeat.Team()
	return TeamSecretSeatView{AgentID: ref.Agent, GroupID: ref.Group}
}

func (a *App) SaveTeamSecret(req SaveTeamSecretRequest) (TeamSecretResult, error) {
	result := TeamSecretResult{Outcome: "rejected"}
	if !teamSecretMaskingReady() {
		result.Reason = "unavailable"
		result.Message = team.ErrSecretsUnavailable.Error()
		result.View, _ = a.GetTeamSecrets()
		return result, nil
	}
	err := a.withTeamWriter(func(ctx context.Context, svc *team.Service) error {
		return svc.Configure(ctx, func() error {
			return desktopTeamStore().InspectAt(ctx, req.ExpectedRosterRevision, func(roster teamroster.Roster) error {
				encoded, err := json.Marshal(req)
				if err != nil {
					return err
				}
				slots := config.DefaultTeamSecretSlots()
				digest, err := slots.Fingerprint(ctx, encoded)
				if err != nil {
					return err
				}
				store := desktopTeamStore()
				_, err = store.MutateTeamSecrets(ctx, req.ExpectedRevision, req.RequestID, digest, func(book *teamroster.TeamSecretBook) error {
					return applyUserTeamSecret(ctx, book, roster, slots, req, teamSecretIntentDir(store))
				})
				// Re-read under the same lock even after an uncertain write error;
				// deleting the returned slot blindly could destroy a committed value.
				return errors.Join(err, recoverTeamSecretCreations(ctx, store, slots))
			})
		})
	})
	result.View, _ = a.GetTeamSecrets()
	if err != nil {
		result.Reason = teamMutationReason(err)
		result.Message = "The Team secret change was not applied. Reload metadata and review its current authorization before retrying."
		return result, nil
	}
	result.Outcome = "applied"
	a.emitTeamChanged()
	return result, nil
}

func applyUserTeamSecret(ctx context.Context, book *teamroster.TeamSecretBook, roster teamroster.Roster, slots config.TeamSecretSlots, req SaveTeamSecretRequest, intentDir string) error {
	if len(req.RequestID) < 8 || len(req.RequestID) > 128 {
		return teamroster.ErrInvalid
	}
	if req.Operation == "forget_retired" {
		if !req.ConfirmForget {
			return errors.New("forgetting retired secret values needs independent confirmation")
		}
		book.Retired = []teamroster.RetiredTeamSecret{}
		return nil
	}
	if req.Name == "" || teamroster.NormalizeSecretName(req.Name) != req.Name {
		return teamroster.ErrInvalid
	}
	i := slices.IndexFunc(book.Secrets, func(sc teamroster.TeamSecret) bool { return sc.Name == req.Name })
	now := time.Now().UTC()
	switch req.Operation {
	case "decline":
		index := slices.IndexFunc(book.Requests, func(q teamroster.TeamSecretRequest) bool { return q.ID == req.SecretRequestID && q.Name == req.Name })
		if index < 0 {
			return teamroster.ErrInvalid
		}
		book.Requests = slices.Delete(book.Requests, index, index+1)
		return nil
	case "delete":
		if i < 0 {
			return teamroster.ErrUnknownSecret
		}
		book.Retire(book.Secrets[i], now)
		book.Secrets = slices.Delete(book.Secrets, i, i+1)
		return nil
	case "save", "authorize":
		proposed := *book
		proposed.Secrets = slices.Clone(book.Secrets)
		proposed.Retired = slices.Clone(book.Retired)
		proposed.Requests = slices.Clone(book.Requests)
		agents := slices.Clone(req.Agents)
		if agents == nil {
			agents = []string{}
		}
		for _, id := range agents {
			if _, ok := roster.Agent(id); !ok {
				return teamroster.ErrUnknownAgent
			}
		}
		if req.SecretRequestID != "" {
			requestIndex := slices.IndexFunc(book.Requests, func(q teamroster.TeamSecretRequest) bool { return q.ID == req.SecretRequestID && q.Name == req.Name })
			if requestIndex < 0 {
				return teamroster.ErrInvalid
			}
		}
		sc := teamroster.TeamSecret{Name: req.Name, Purpose: req.Purpose, Agents: agents, UpdatedAt: now}
		if req.Operation == "authorize" {
			if i < 0 || req.Value != "" {
				return teamroster.ErrInvalid
			}
			sc.Slot = book.Secrets[i].Slot
		} else {
			sc.Slot = "pending"
			if i >= 0 {
				proposed.Retire(book.Secrets[i], now)
			}
		}
		if i < 0 {
			i = len(proposed.Secrets)
			proposed.Secrets = append(proposed.Secrets, sc)
		} else {
			proposed.Secrets[i] = sc
		}
		proposed.Requests = slices.DeleteFunc(proposed.Requests, func(q teamroster.TeamSecretRequest) bool { return q.ID == req.SecretRequestID })
		if err := proposed.Validate(); err != nil {
			return err
		}
		if req.Operation == "save" {
			slot, err := slots.SaveWithIntent(ctx, req.Value, intentDir)
			if err != nil {
				return err
			}
			proposed.Secrets[i].Slot = slot
		}
		*book = proposed
		return nil
	default:
		return teamroster.ErrInvalid
	}
}
