package boot

import (
	"reflect"
	"strings"
	"testing"

	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"deepzora/internal/tool/builtin"
)

func TestEffectTeamIdentityChangesOnlyTurnTailAndTeamTools(t *testing.T) {
	opts, rec := presetEffectSetup(t)
	plain := buildPresetEffect(t, opts)
	if err := plain.Controller.Run(t.Context(), "ordinary"); err != nil {
		t.Fatal(err)
	}
	svc := team.New(team.Options{Store: teamroster.Store{Dir: t.TempDir()}})
	r, err := svc.Bootstrap(t.Context(), "Chief", "Lead")
	if err != nil {
		t.Fatal(err)
	}
	opts.TeamSeat = team.Seat{Service: svc, Ref: team.SeatRef{Agent: r.Agents[0].ID}}
	member := buildPresetEffect(t, opts)
	for _, input := range []string{"first", "second"} {
		if err := member.Controller.Run(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	}
	reqs := rec.requests()
	if len(reqs) != 3 {
		t.Fatalf("requests=%d", len(reqs))
	}
	if reqs[0].Messages[0].Content != reqs[1].Messages[0].Content {
		t.Fatal("team identity changed system prefix")
	}
	if !reflect.DeepEqual(reqs[1].Tools, reqs[2].Tools) {
		t.Fatal("team tools changed between turns")
	}
	ordinary, memberTools := toolNames(reqs[0]), toolNames(reqs[1])
	for _, name := range builtin.TeamToolNames() {
		if ordinary[name] || !memberTools[name] {
			t.Fatalf("team surface %s ordinary=%v team=%v", name, ordinary[name], memberTools[name])
		}
	}
	found := false
	for _, m := range reqs[1].Messages {
		if strings.Contains(m.Content, "<agent-identity>") && strings.Contains(m.Content, "Chief") {
			found = true
		}
	}
	if !found {
		t.Fatal("team identity missing from provider-visible tail")
	}
}
