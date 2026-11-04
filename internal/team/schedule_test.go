package team

import (
	"context"
	"deepzora/internal/teamroster"
	"strings"
	"testing"
)

func TestScheduledGroupInputPreservesOriginCeilingAndDuplicateAdmission(t *testing.T) {
	h := newHarness(t, "Chief", "Ada", "Ben")
	g := h.group("Launch", teamroster.ModeEveryone, "Ada", "Ada", "Ben")
	sc := teamroster.Schedule{ID: "s-0000000000000001", Group: g, Name: "Review", Prompt: "Review the release"}
	dispatch := func() error {
		return h.svc.WithScheduleAdmission(t.Context(), func() error { return h.svc.DispatchSchedule(t.Context(), sc, "schedule-test-occurrence") })
	}
	if err := dispatch(); err != nil {
		t.Fatal(err)
	}
	delivery := h.lastTurn("Ada", g)
	if delivery.Ceiling != "workspace-write" || !strings.Contains(delivery.Text, "not a fresh human request") {
		t.Fatalf("scheduler gained human authority: %+v", delivery)
	}
	if err := dispatch(); err != nil {
		t.Fatal(err)
	}
	posts := 0
	if err := h.svc.store.ScanLines(g, func(line teamroster.GroupLine) {
		if line.Kind == teamroster.LinePost {
			posts++
			if line.Source != "scheduler" {
				t.Error("scheduler masqueraded as human evidence")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("occurrence replayed %d posts", posts)
	}
	sc.Prompt = "changed"
	if err := h.svc.WithScheduleAdmission(context.Background(), func() error { return h.svc.DispatchSchedule(t.Context(), sc, "schedule-test-occurrence") }); err == nil {
		t.Fatal("reused occurrence changed content")
	}
}
