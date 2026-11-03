package agent

import "deepzora/internal/secrets"

func (a *Agent) boundProviderVisibleResult(raw, toolName, callID string) (body, notice, original string) {
	raw = secrets.Masker().String(raw)
	summarized := summarizeCIOutput(raw)
	body, notice = truncateToolOutputFor(summarized, toolName, callID)
	if summarized != raw || notice != "" {
		original = raw
	}
	return body, notice, original
}
