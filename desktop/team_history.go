package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"os"
	"time"

	"deepzora/internal/teamroster"
)

const teamHistoryPageSize = 32

type TeamGroupHistoryPage struct {
	Status      string                 `json:"status"`
	Lines       []teamroster.GroupLine `json:"lines"`
	OlderCursor string                 `json:"olderCursor"`
	NewerCursor string                 `json:"newerCursor"`
	Snapshot    string                 `json:"snapshot"`
	MaxSequence int64                  `json:"maxSequence"`
}
type teamGroupSnapshot struct {
	group  string
	max    int64
	digest string
	file   os.FileInfo
	at     time.Time
}
type teamGroupCursor struct {
	Snapshot string `json:"snapshot"`
	Position int64  `json:"position"`
	Newer    bool   `json:"newer"`
}

func groupLineHash(h hash.Hash, line teamroster.GroupLine) {
	encoded, _ := json.Marshal(line)
	_, _ = h.Write(encoded)
	_, _ = h.Write([]byte{'\n'})
}
func groupLogInfo(store teamroster.Store, group string) (os.FileInfo, error) {
	path, err := store.GroupLogPath(group)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return info, err
}
func sameGroupLog(first, second os.FileInfo) bool {
	return first == nil && second == nil || first != nil && second != nil && os.SameFile(first, second)
}
func encodeGroupCursor(id string, position int64, newer bool) string {
	encoded, _ := json.Marshal(teamGroupCursor{Snapshot: id, Position: position, Newer: newer})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// Group history uses fixed snapshots and a bounded body page. Appends preserve
// cursors; a replacement, unreadable cursor or expired snapshot returns the same
// typed stale_cursor result. The frontend retains at most three adjacent pages.
func (a *App) ReadTeamGroupHistory(groupID, cursor string) (TeamGroupHistoryPage, error) {
	stale := TeamGroupHistoryPage{Status: "stale_cursor", Lines: []teamroster.GroupLine{}}
	store := desktopTeamStore()
	r, err := store.Load()
	if err != nil {
		return stale, err
	}
	_, exists := r.Group(groupID)
	for _, retired := range r.RetiredGroups {
		exists = exists || retired.ID == groupID
	}
	if !exists {
		return stale, teamroster.ErrUnknownGroup
	}
	var token teamGroupCursor
	var snapshot teamGroupSnapshot
	initial := cursor == ""
	if !initial {
		encoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(encoded) > 1024 || json.Unmarshal(encoded, &token) != nil || token.Position < 0 {
			return stale, nil
		}
		a.teamState.historyMu.Lock()
		var ok bool
		snapshot, ok = a.teamState.history[token.Snapshot]
		a.teamState.historyMu.Unlock()
		if !ok || snapshot.group != groupID || time.Since(snapshot.at) > 10*time.Minute {
			return stale, nil
		}
	}
	file, err := groupLogInfo(store, groupID)
	if err != nil {
		if !initial {
			return stale, nil
		}
		return stale, err
	}
	if !initial && !sameGroupLog(snapshot.file, file) {
		return stale, nil
	}
	page, digest, maxSeq, err := readTeamGroupPage(store, groupID, snapshot, token, initial)
	if err != nil {
		if !initial {
			return stale, nil
		}
		return stale, err
	}
	after, err := groupLogInfo(store, groupID)
	if err != nil || !sameGroupLog(file, after) {
		return stale, nil
	}
	if !initial && (digest != snapshot.digest || maxSeq != snapshot.max) {
		return stale, nil
	}
	if initial {
		token.Snapshot = newTabID()
		snapshot = teamGroupSnapshot{group: groupID, max: maxSeq, digest: digest, file: file, at: time.Now()}
		a.rememberGroupSnapshot(token.Snapshot, snapshot)
	}
	page.Status, page.Snapshot, page.MaxSequence = "ready", token.Snapshot, snapshot.max
	if len(page.Lines) > 0 {
		first, last := page.Lines[0].Seq, page.Lines[len(page.Lines)-1].Seq
		if first > 1 {
			page.OlderCursor = encodeGroupCursor(token.Snapshot, first, false)
		}
		if last < snapshot.max {
			page.NewerCursor = encodeGroupCursor(token.Snapshot, last, true)
		}
	}
	return page, nil
}

func readTeamGroupPage(store teamroster.Store, group string, snapshot teamGroupSnapshot, cursor teamGroupCursor, initial bool) (TeamGroupHistoryPage, string, int64, error) {
	page := TeamGroupHistoryPage{Lines: []teamroster.GroupLine{}}
	h := sha256.New()
	var maxSeq int64
	err := store.ScanLines(group, func(line teamroster.GroupLine) {
		if !initial && line.Seq > snapshot.max {
			return
		}
		groupLineHash(h, line)
		maxSeq = line.Seq
		if initial || !cursor.Newer && line.Seq < cursor.Position {
			page.Lines = append(page.Lines, line)
			if len(page.Lines) > teamHistoryPageSize {
				copy(page.Lines, page.Lines[1:])
				page.Lines = page.Lines[:teamHistoryPageSize]
			}
		} else if cursor.Newer && line.Seq > cursor.Position && len(page.Lines) < teamHistoryPageSize {
			page.Lines = append(page.Lines, line)
		}
	})
	return page, hex.EncodeToString(h.Sum(nil)), maxSeq, err
}

func (a *App) rememberGroupSnapshot(id string, snapshot teamGroupSnapshot) {
	a.teamState.historyMu.Lock()
	defer a.teamState.historyMu.Unlock()
	if a.teamState.history == nil {
		a.teamState.history = map[string]teamGroupSnapshot{}
	}
	if len(a.teamState.history) >= 64 {
		oldest := ""
		for key, value := range a.teamState.history {
			if oldest == "" || value.at.Before(a.teamState.history[oldest].at) {
				oldest = key
			}
		}
		delete(a.teamState.history, oldest)
	}
	a.teamState.history[id] = snapshot
}
