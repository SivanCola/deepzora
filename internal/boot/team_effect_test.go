package boot

import (
	"reflect"
	"strings"
	"testing"

	"deepzora/internal/agentpreset"
	"deepzora/internal/team"
	"deepzora/internal/teamroster"
	"deepzora/internal/tool/builtin"
)

func TestEffectTeamPresetSnapshotSurvivesSourceReplacement(t *testing.T) {
	opts, rec := presetEffectSetup(t)
	dirs := AgentPresetDirs(opts.WorkspaceRoot)
	p, err := agentpreset.Write(dirs.Project, "copied", "---\ncomplete: true\ninclude-runtime-context: false\ntools: []\n---\nORIGINAL_TEAM_DEFINITION")
	if err != nil {
		t.Fatal(err)
	}
	store := teamroster.Store{Dir: t.TempDir()}
	r, err := store.Update(t.Context(), func(r *teamroster.Roster) error {
		r.AddAgent(teamroster.Agent{Name: "Chief", Main: true, Preset: &p})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentpreset.Write(dirs.Project, "copied", "CHANGED_SOURCE"); err != nil {
		t.Fatal(err)
	}
	opts.TeamSeat = team.Seat{Service: team.New(team.Options{Store: store}), Ref: team.SeatRef{Agent: r.Agents[0].ID}}
	member := buildPresetEffect(t, opts)
	if err := member.Controller.Run(t.Context(), "first"); err != nil {
		t.Fatal(err)
	}
	request := rec.requests()[0]
	if !strings.Contains(request.Messages[0].Content, "ORIGINAL_TEAM_DEFINITION") || strings.Contains(request.Messages[0].Content, "CHANGED_SOURCE") {
		t.Fatal("team followed the mutable source definition")
	}
	for _, schema := range request.Tools {
		if schema.Name == "write_file" {
			t.Fatal("copied empty tool allowlist was lost")
		}
	}
}

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
