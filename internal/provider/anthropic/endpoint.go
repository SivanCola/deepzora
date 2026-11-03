package anthropic

import (
	"strings"

	"deepzora/internal/provider"
)

// resolveEndpoints returns the API root and the URL requests POST to. The root
// never carries a trailing /v1: users paste the OpenAI-style ".../v1" that
// /models probes expect, and appending /v1/messages to it would 404. A
// request_url that only repeats the root, with or without /v1, is unset.
func resolveEndpoints(baseURL string, extra map[string]any) (root, requestURL string) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// A complete Messages URL is sometimes pasted into base_url. Keep it as
	// the request target instead of appending another /v1/messages.
	complete := strings.HasSuffix(base, "/messages")
	root = strings.TrimSuffix(strings.TrimSuffix(base, "/messages"), "/v1")
	if root == "" {
		root = defaultBaseURL
	}
	requestURL, _ = extra["request_url"].(string)
	requestURL = strings.TrimSpace(requestURL)
	if requestURL == "" || provider.EndpointOverrideRepeatsBase(requestURL, root, root+"/v1") {
		requestURL = root + "/v1/messages"
		if complete {
			requestURL = base
		}
	}
	return root, requestURL
}
