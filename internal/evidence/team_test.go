package evidence

import "testing"

func TestTeamCoordinationDoesNotClaimAWorkProductMutation(t *testing.T) {
	for _, name := range []string{"message_agent", "create_agent", "update_agent", "post_to_group", "pass_turn", "hand_turn"} {
		p := ClassifyEffect(EffectInput{ToolName: name, StaticReadOnly: true})
		if !p.Known || !p.HostState || p.WorkspaceWrite || p.ExternalState {
			t.Fatalf("%s: %+v", name, p)
		}
	}
}
