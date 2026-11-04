package teamroster

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTeamScheduleSavedTimezoneAndDST(t *testing.T) {
	sc := Schedule{Repeat: "daily", Clock: "09:00", Timezone: "America/New_York"}
	now := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	next := sc.NextAfter(now)
	if want := time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("DST calendar drift: %s want %s", next, want)
	}
	if got := sc.NextAfter(now.In(time.FixedZone("device", 8*3600))); !got.Equal(next) {
		t.Fatalf("device zone changed saved time: %s", got)
	}
	sc.Clock = "02:30"
	next = sc.NextAfter(now)
	if next.In(mustScheduleZone(t, sc.Timezone)).Day() != 9 {
		t.Fatalf("nonexistent clock was silently shifted: %s", next)
	}
	sc.Repeat = "every"
	sc.IntervalMinutes = 10
	sc.Anchor = now
	if got := sc.NextAfter(now.Add(31 * time.Minute)); !got.Equal(now.Add(40 * time.Minute)) {
		t.Fatalf("interval lost anchor: %s", got)
	}
}
func mustScheduleZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestScheduleCASNoopUnknownSchemaAndConcurrentWriters(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	rev := uint64(0)
	_, err := store.UpdateSchedules(t.Context(), &rev, func(book *ScheduleBook) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateSchedules(t.Context(), &rev, func(book *ScheduleBook) (bool, error) { t.Error("stale callback executed"); return true, nil })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale CAS: %v", err)
	}
	book, err := store.UpdateSchedules(t.Context(), nil, func(book *ScheduleBook) (bool, error) { return false, nil })
	if err != nil || book.Revision != 1 {
		t.Fatalf("noop moved version: %+v %v", book, err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, ScheduleFile), []byte(`{"schemaVersion":999,"schedules":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateSchedules(t.Context(), nil, func(book *ScheduleBook) (bool, error) { t.Error("future schema overwritten"); return true, nil })
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("future schema: %v", err)
	}
}
