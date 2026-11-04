package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"deepzora/internal/config"
	"deepzora/internal/provider"
	"deepzora/internal/secrets"
	"deepzora/internal/skill"
)

func TestReviewCommandProtectsBotCredentialsInIndependentProcess(t *testing.T) {
	const child = "DEEPZORA_TEST_REVIEW_BOT_CHILD"
	if os.Getenv(child) != "1" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestReviewCommandProtectsBotCredentialsInIndependentProcess$", "-test.v")
		cmd.Env = append(os.Environ(), child+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("independent review: %v\n%s", err, out)
		}
		return
	}
	isolateCLIConfigHome(t)
	t.Setenv("DEEPZORA_HOME", t.TempDir())
	root := setupReviewCheckout(t, nil)
	dir := filepath.Join(config.DeepzoraHomeDir(), "bot-channels")
	active, backup := "review-bot-active-disposable-6813", "review-bot-backup-disposable-5972"
	for name, value := range map[string]string{"connections.json": active, "connections.pre-members.json": backup} {
		writeReviewTestFile(t, dir, name, `[{"id":"review-boundary","secret":"`+value+`"}]`)
	}
	path := filepath.Join(dir, "connections.json")
	// The direct CLI assembler must also refuse reads before a session exists.
	secrets.SetProtectSensitiveFiles(false)
	reg := buildReviewSubagentRegistry(skill.Skill{ReadOnly: true, AllowedTools: []string{"read_file"}}, config.Default(), root)
	reader, ok := reg.Get("read_file")
	if !ok {
		t.Fatal("review registry omitted read_file")
	}
	args, _ := json.Marshal(map[string]string{"path": path})
	if _, err := reader.Execute(t.Context(), args); err == nil {
		t.Fatal("direct CLI reader exposed Bot credentials")
	}
	writeReviewTestFile(t, root, "marker.txt", active+"\n")
	writeReviewTestFile(t, root, "deepzora.toml", "")
	requests := make(chan string, 4)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests <- string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "bot-read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": string(args)},
			}}}}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"no findings\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	writeReviewTestFile(t, filepath.Dir(config.UserCredentialsPath()), filepath.Base(config.UserCredentialsPath()), "DEEPZORA_TEST_REVIEW_KEY=test-key\n")
	writeReviewTestFile(t, filepath.Dir(config.UserConfigPath()), filepath.Base(config.UserConfigPath()), fmt.Sprintf(`
default_model = "reviewer/model"
[[providers]]
name = "reviewer"
kind = "openai"
base_url = %q
models = ["model"]
default = "model"
api_key_env = "DEEPZORA_TEST_REVIEW_KEY"
`, server.URL+"/v1"))
	captureStdout(t, func() {
		if rc := reviewCommand([]string{"--instructions", backup}); rc != 0 {
			t.Fatalf("review exit=%d", rc)
		}
	})
	if calls.Load() != 2 {
		t.Fatalf("review made %d requests, expected a read attempt and final response", calls.Load())
	}
	for i := range 2 {
		body := <-requests
		if strings.Contains(body, active) || strings.Contains(body, backup) || !strings.Contains(body, "[secret:BOT_review-boundary]") {
			t.Fatal("review wire request did not mask the saved Bot credentials")
		}
		if i == 1 && !strings.Contains(body, `"role":"tool"`) {
			t.Fatal("review never returned the refused read to the provider")
		}
	}
	// The command's watcher must not survive its successful return.
	writeReviewTestFile(t, dir, "connections.json", "{")
	if err := provider.CheckSecretSignedReplay(provider.Request{}); err != nil {
		t.Fatalf("completed review retained its credential watcher: %v", err)
	}
}
