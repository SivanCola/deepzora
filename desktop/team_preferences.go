package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type TeamGroupDraft struct {
	RequestID string   `json:"requestId"`
	Text      string   `json:"text"`
	Mentions  []string `json:"mentions"`
	Everyone  bool     `json:"everyone"`
}

type TeamGroupDraftView struct {
	Revision uint64         `json:"revision"`
	Draft    TeamGroupDraft `json:"draft"`
}

type SaveTeamGroupDraftRequest struct {
	GroupID          string         `json:"groupId"`
	ExpectedRevision uint64         `json:"expectedRevision"`
	Draft            TeamGroupDraft `json:"draft"`
}

type TeamGroupDraftResult struct {
	Outcome string             `json:"outcome"`
	Reason  string             `json:"reason,omitempty"`
	View    TeamGroupDraftView `json:"view"`
}

func groupDraftView(saved teamroster.GroupPreferences) (TeamGroupDraftView, error) {
	view := TeamGroupDraftView{Revision: saved.DraftRevision, Draft: TeamGroupDraft{Mentions: []string{}}}
	if saved.DraftJSON != "" {
		if err := json.Unmarshal([]byte(saved.DraftJSON), &view.Draft); err != nil {
			return view, err
		}
	}
	return view, nil
}

func (a *App) ReadTeamGroupDraft(groupID string) (TeamGroupDraftView, error) {
	saved, err := desktopTeamStore().GroupPreferences(groupID)
	if err != nil {
		return TeamGroupDraftView{}, err
	}
	return groupDraftView(saved)
}

func (a *App) SaveTeamGroupDraft(req SaveTeamGroupDraftRequest) (TeamGroupDraftResult, error) {
	if len(req.Draft.Text) > 128*1024 || len(req.Draft.Mentions) > teamroster.MaxMembers {
		return TeamGroupDraftResult{Outcome: "rejected", Reason: "invalid"}, nil
	}
	var saved teamroster.GroupPreferences
	err := a.withTeamWriter(func(ctx context.Context, _ *team.Service) error {
		r, err := desktopTeamStore().Load()
		if err != nil {
			return err
		}
		if _, ok := r.Group(req.GroupID); !ok {
			return teamroster.ErrUnknownGroup
		}
		draft := req.Draft
		draft.Mentions = append([]string{}, draft.Mentions...)
		if slices.Contains(draft.Mentions, "") {
			return teamroster.ErrInvalid
		}
		body, err := json.Marshal(draft)
		if err != nil {
			return err
		}
		saved, err = desktopTeamStore().UpdateGroupPreferences(ctx, req.GroupID, func(current *teamroster.GroupPreferences) error {
			if current.DraftRevision != req.ExpectedRevision {
				if current.DraftRevision == req.ExpectedRevision+1 && current.DraftJSON == string(body) {
					return nil
				}
				return teamroster.ErrConflict
			}
			current.DraftJSON, current.DraftRevision = string(body), current.DraftRevision+1
			return nil
		})
		return err
	})
	if err != nil {
		return TeamGroupDraftResult{Outcome: "rejected", Reason: teamMutationReason(err)}, nil
	}
	view, err := groupDraftView(saved)
	return TeamGroupDraftResult{Outcome: "applied", View: view}, err
}

type TeamGroupStatus struct {
	GroupID      string `json:"groupId"`
	LastSequence int64  `json:"lastSequence"`
	ReadSequence int64  `json:"readSequence"`
	Unread       int    `json:"unread"`
}

func teamGroupStatus(store teamroster.Store, id string) (TeamGroupStatus, error) {
	status := TeamGroupStatus{GroupID: id}
	prefs, err := store.GroupPreferences(id)
	if err != nil {
		return status, err
	}
	h := sha256.New()
	total, unread := 0, 0
	valid := prefs.ReadSequence == 0
	err = store.ScanLines(id, func(line teamroster.GroupLine) {
		status.LastSequence = line.Seq
		if line.Seq <= prefs.ReadSequence {
			groupLineHash(h, line)
		}
		if line.Seq == prefs.ReadSequence {
			valid = hex.EncodeToString(h.Sum(nil)) == prefs.ReadDigest
		}
		if line.From != teamroster.ByUser {
			total++
			if line.Seq > prefs.ReadSequence {
				unread++
			}
		}
	})
	status.Unread = total
	if valid {
		status.ReadSequence, status.Unread = prefs.ReadSequence, unread
	}
	return status, err
}

var errTeamHistoryStale = errors.New("stale_cursor")

// MarkTeamGroupRead binds the watermark to the history snapshot actually
// presented. A late acknowledgement cannot mark a replacement log as read.
func (a *App) MarkTeamGroupRead(groupID, snapshotID string, sequence int64) error {
	a.teamState.preferencesMu.Lock()
	defer a.teamState.preferencesMu.Unlock()
	a.teamState.historyMu.Lock()
	snapshot, ok := a.teamState.history[snapshotID]
	a.teamState.historyMu.Unlock()
	if !ok || snapshot.group != groupID || sequence < 0 || sequence > snapshot.max {
		return errTeamHistoryStale
	}
	return a.withTeamWriter(func(ctx context.Context, _ *team.Service) error {
		store := desktopTeamStore()
		file, err := groupLogInfo(store, groupID)
		if err != nil || !sameGroupLog(snapshot.file, file) {
			return errTeamHistoryStale
		}
		_, digest, maxSeq, err := readTeamGroupPage(store, groupID, snapshot, teamGroupCursor{}, false)
		if err != nil || digest != snapshot.digest || maxSeq != snapshot.max {
			return errTeamHistoryStale
		}
		status, err := teamGroupStatus(store, groupID)
		if err != nil {
			return err
		}
		if status.ReadSequence >= sequence {
			return nil
		}
		h := sha256.New()
		if err := store.ScanLines(groupID, func(line teamroster.GroupLine) {
			if line.Seq <= sequence {
				groupLineHash(h, line)
			}
		}); err != nil {
			return err
		}
		_, err = store.UpdateGroupPreferences(ctx, groupID, func(current *teamroster.GroupPreferences) error {
			if current.ReadSequence > sequence && current.ReadSequence == status.ReadSequence {
				return nil
			}
			current.ReadSequence, current.ReadDigest = sequence, hex.EncodeToString(h.Sum(nil))
			return nil
		})
		if err == nil {
			a.emitTeamChanged()
		}
		return err
	})
}
