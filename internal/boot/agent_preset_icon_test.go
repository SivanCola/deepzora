package boot

import (
	"testing"

	"deepzora/internal/agentpreset"
)

func TestAgentPresetIconsDoNotChangeRuntimeFingerprintOrPrompt(t *testing.T) {
	preset := agentpreset.Default()
	preset.ID, preset.Body = "reviewer", "Review carefully."
	fingerprint, prompt := agentPresetFingerprint(preset), preset.Apply("Base instructions")
	preset.Icon, preset.IconImage = "search", "data:image/png;base64,display-only"
	if agentPresetFingerprint(preset) != fingerprint || preset.Apply("Base instructions") != prompt {
		t.Fatal("display-only icon changed the runtime fingerprint or prompt")
	}
	preset.Body = "Different persona"
	if agentPresetFingerprint(preset) == fingerprint {
		t.Fatal("persona change no longer affects the runtime fingerprint")
	}
}
