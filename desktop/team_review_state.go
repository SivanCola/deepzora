package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"deepzora/internal/fileutil"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
)

type TeamMemoryReviewStatus struct {
	AgentID  string    `json:"agentId"`
	GroupID  string    `json:"groupId,omitempty"`
	At       time.Time `json:"at"`
	Outcome  string    `json:"outcome"`
	Added    int       `json:"added"`
	Edited   int       `json:"edited"`
	Archived int       `json:"archived"`
	Error    string    `json:"error,omitempty"`
}
type teamReviewPosition struct {
	Through    uint64
	Generation string
}
type teamReviewState struct {
	SchemaVersion int
	Epoch         time.Time
	Sessions      map[string]teamReviewPosition
	Groups        map[string]int64
	Status        []TeamMemoryReviewStatus
}

func teamReviewStatePath() string {
	return filepath.Join(desktopTeamStore().Dir, "memory", "review.json")
}
func loadTeamReviewState(epoch time.Time) (teamReviewState, error) {
	state := teamReviewState{SchemaVersion: 1, Epoch: epoch, Sessions: map[string]teamReviewPosition{}, Groups: map[string]int64{}, Status: []TeamMemoryReviewStatus{}}
	body, err := os.ReadFile(teamReviewStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	var saved teamReviewState
	if err := json.Unmarshal(body, &saved); err != nil {
		return state, err
	}
	if saved.SchemaVersion != 1 {
		return state, teamroster.ErrNewerSchema
	}
	if !saved.Epoch.Equal(epoch) {
		return state, nil
	}
	if saved.Sessions == nil {
		saved.Sessions = map[string]teamReviewPosition{}
	}
	if saved.Groups == nil {
		saved.Groups = map[string]int64{}
	}
	if saved.Status == nil {
		saved.Status = []TeamMemoryReviewStatus{}
	}
	return saved, nil
}
func saveTeamReviewState(state teamReviewState) error {
	if len(state.Status) > 64 {
		state.Status = state.Status[len(state.Status)-64:]
	}
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(teamReviewStatePath()), 0700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFileStrict(teamReviewStatePath(), body, 0600)
}
func teamReviewKey(ref team.SeatRef) string {
	if ref.Group != "" {
		return "group:" + ref.Group
	}
	return "agent:" + ref.Agent
}
