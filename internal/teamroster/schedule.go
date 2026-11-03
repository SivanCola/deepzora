package teamroster

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"deepzora/internal/fileutil"
)

const ScheduleFile = "schedules.json"
const ScheduleGrace = 2 * time.Minute

// Schedule time rules are copied configuration; runtime positions are owned
// exclusively by the Desktop lease holder, never by a renderer update.
type Schedule struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Agent           string    `json:"agent,omitempty"`
	Group           string    `json:"group,omitempty"`
	Prompt          string    `json:"prompt"`
	Repeat          string    `json:"repeat"`
	Timezone        string    `json:"timezone"`
	Clock           string    `json:"clock,omitempty"`
	Days            []int     `json:"days"`
	IntervalMinutes int       `json:"intervalMinutes,omitempty"`
	Anchor          time.Time `json:"anchor"`
	Enabled         bool      `json:"enabled"`
	Next            time.Time `json:"next"`
	LastOccurrence  string    `json:"lastOccurrence,omitempty"`
	LastOutcome     string    `json:"lastOutcome,omitempty"`
	LastAt          time.Time `json:"lastAt"`
}
type ScheduleReceipt struct{ Key, Digest, Target, Outcome string }
type ScheduleBook struct {
	SchemaVersion int               `json:"schemaVersion"`
	Revision      uint64            `json:"revision"`
	Schedules     []Schedule        `json:"schedules"`
	Receipts      []ScheduleReceipt `json:"receipts"`
}

func (sc Schedule) Validate() error {
	if (sc.Agent == "") == (sc.Group == "") {
		return fieldErr("target", ErrInvalid)
	}
	if sc.Agent != "" && !agentID.MatchString(sc.Agent) || sc.Group != "" && !groupID.MatchString(sc.Group) {
		return fieldErr("target", ErrInvalid)
	}
	if utf8.RuneCountInString(sc.Name) > 60 || utf8.RuneCountInString(sc.Prompt) > 2000 {
		return ErrTooLong
	}
	if strings.TrimSpace(sc.Name) == "" || strings.TrimSpace(sc.Prompt) == "" {
		return ErrEmpty
	}
	if sc.Timezone == "" || sc.Timezone == "Local" {
		return fieldErr("timezone", ErrInvalid)
	}
	if _, err := time.LoadLocation(sc.Timezone); err != nil {
		return fieldErr("timezone", ErrInvalid)
	}
	switch sc.Repeat {
	case "once":
		if sc.Anchor.IsZero() {
			return fieldErr("anchor", ErrEmpty)
		}
	case "every":
		if sc.IntervalMinutes < 10 || sc.IntervalMinutes > 7*24*60 || sc.Anchor.IsZero() {
			return fieldErr("interval", ErrInvalid)
		}
	case "daily", "weekly":
		if _, _, ok := scheduleClock(sc.Clock); !ok {
			return fieldErr("clock", ErrInvalid)
		}
		if sc.Repeat == "weekly" && (len(sc.Days) == 0 || slices.ContainsFunc(sc.Days, func(d int) bool { return d < 0 || d > 6 })) {
			return fieldErr("days", ErrInvalid)
		}
	default:
		return fieldErr("repeat", ErrInvalid)
	}
	return nil
}
func scheduleClock(clock string) (int, int, bool) {
	if len(clock) != 5 || clock[2] != ':' {
		return 0, 0, false
	}
	h, e1 := strconv.Atoi(clock[:2])
	m, e2 := strconv.Atoi(clock[3:])
	return h, m, e1 == nil && e2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60
}

// NextAfter uses the saved IANA zone even after the device changes zones.
// Calendar days use AddDate, so DST does not become a 24-hour drift.
func (sc Schedule) NextAfter(now time.Time) time.Time {
	loc, err := time.LoadLocation(sc.Timezone)
	if err != nil {
		return time.Time{}
	}
	switch sc.Repeat {
	case "once":
		if sc.Anchor.After(now) {
			return sc.Anchor
		}
	case "every":
		step := time.Duration(sc.IntervalMinutes) * time.Minute
		if step < 10*time.Minute || step > 7*24*time.Hour {
			return time.Time{}
		}
		if sc.Anchor.After(now) {
			return sc.Anchor
		}
		return sc.Anchor.Add(step * (now.Sub(sc.Anchor)/step + 1))
	case "daily", "weekly":
		hh, mm, ok := scheduleClock(sc.Clock)
		if !ok {
			return time.Time{}
		}
		now = now.In(loc)
		for d := range 8 {
			day := now.AddDate(0, 0, d)
			at := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, loc)
			// A nonexistent wall time is skipped, never normalized to another hour.
			if at.Hour() != hh || at.Minute() != mm || !at.After(now) {
				continue
			}
			if sc.Repeat == "daily" || slices.Contains(sc.Days, int(at.Weekday())) {
				return at.UTC()
			}
		}
	}
	return time.Time{}
}

func (s Store) ScheduleBook() (ScheduleBook, error) {
	book := ScheduleBook{SchemaVersion: 1, Schedules: []Schedule{}, Receipts: []ScheduleReceipt{}}
	raw, err := os.ReadFile(filepath.Join(s.Dir, ScheduleFile))
	if errors.Is(err, os.ErrNotExist) {
		return book, nil
	}
	if err != nil {
		return book, err
	}
	if err := json.Unmarshal(raw, &book); err != nil {
		return book, ErrCorrupt
	}
	if book.SchemaVersion != 1 {
		return book, ErrNewerSchema
	}
	if book.Schedules == nil {
		book.Schedules = []Schedule{}
	}
	if book.Receipts == nil {
		book.Receipts = []ScheduleReceipt{}
	}
	for _, sc := range book.Schedules {
		if err := sc.Validate(); err != nil {
			return book, errors.Join(ErrCorrupt, err)
		}
	}
	return book, nil
}

// UpdateSchedules holds an independent revision and cross-process lock. A
// no-op tick does not publish or increment revision.
func (s Store) UpdateSchedules(ctx context.Context, revision *uint64, update func(*ScheduleBook) (bool, error)) (ScheduleBook, error) {
	var out ScheduleBook
	err := s.withLock(ctx, s.Dir, ScheduleFile, func() error {
		book, err := s.ScheduleBook()
		if err != nil {
			return err
		}
		if revision != nil && *revision != book.Revision {
			return ErrConflict
		}
		changed, err := update(&book)
		if err != nil {
			return err
		}
		if !changed {
			out = book
			return nil
		}
		if len(book.Schedules) > 128 {
			return ErrInvalid
		}
		for _, sc := range book.Schedules {
			if err := sc.Validate(); err != nil {
				return err
			}
		}
		book.Revision++
		if len(book.Receipts) > 512 {
			book.Receipts = book.Receipts[len(book.Receipts)-512:]
		}
		raw, err := json.Marshal(book)
		if err != nil {
			return err
		}
		if err := fileutil.AtomicWriteFileStrict(filepath.Join(s.Dir, ScheduleFile), raw, 0600); err != nil {
			return err
		}
		out = book
		return nil
	})
	return out, err
}

func NewScheduleID() string { return "s-" + randomHex() }
