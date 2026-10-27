package agent

import (
	"testing"

	"deepzora/internal/provider"
)

func TestReasoningLanguageDirectiveIsNotUserAuthored(t *testing.T) {
	for _, injected := range []string{
		"<reasoning-language>\nUse Simplified Chinese",
		"<reasoning-language>\nUse Simplified Chinese\n</reasoning-language>",
	} {
		if IsUserAuthoredTurnMessage(provider.Message{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: injected}) {
			t.Fatalf("reasoning-language directive %q must not count as user-authored", injected)
		}
	}
}
