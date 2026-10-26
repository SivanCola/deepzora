package servecontract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// CapabilitiesHeader carries the comma-joined capability tokens a serve
// advertises on a successful token handshake.
const CapabilitiesHeader = "X-Deepzora-Serve-Capabilities"

// Handshake trades a serve's launch token for the session cookie, which lands
// in client's jar, and returns the advertised capabilities. Older serves omit
// the header and yield nil. Transport failures are returned unwrapped from
// client.Do so callers can tell an unreachable serve from a refusal.
func Handshake(ctx context.Context, client *http.Client, base, token string) ([]string, error) {
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/auth/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("serve auth handshake: status %d", resp.StatusCode)
	}
	var caps []string
	for capability := range strings.SplitSeq(resp.Header.Get(CapabilitiesHeader), ",") {
		if capability = strings.TrimSpace(capability); capability != "" {
			caps = append(caps, capability)
		}
	}
	return caps, nil
}
