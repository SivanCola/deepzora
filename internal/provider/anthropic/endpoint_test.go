package anthropic

import "testing"

func TestCompleteMessagesBaseURL(t *testing.T) {
	for _, base := range []string{
		"https://api.deepseek.com/anthropic/messages",
		"https://api.deepseek.com/anthropic/v1/messages",
	} {
		root, request := resolveEndpoints(base+"/", nil)
		if root != "https://api.deepseek.com/anthropic" || request != base {
			t.Fatalf("resolveEndpoints(%s) = %s, %s", base, root, request)
		}
		_, request = resolveEndpoints(base, map[string]any{"request_url": base})
		if request != base {
			t.Fatalf("complete request URL mistaken for a repeated API root: %s", request)
		}
		const override = "https://gateway.example/custom/messages?route=1"
		_, request = resolveEndpoints(base, map[string]any{"request_url": override})
		if request != override {
			t.Fatalf("explicit request override changed: %s", request)
		}
	}
}
