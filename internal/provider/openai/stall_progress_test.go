package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"deepzora/internal/provider"
)

const (
	sseRoleOnly = "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"
	sseLet      = "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Let\"}}]}\n\n"
	sseOK       = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
)

// keepAliveServer writes head, then a keep-alive comment every 20ms for
// keepAlive (forever when zero), then tail. Bytes never stop arriving, so the
// idle timeout cannot see a stall here.
func keepAliveServer(t *testing.T, head string, keepAlive time.Duration, tail string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, head)
		flush(w)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		var stop <-chan time.Time
		if keepAlive > 0 {
			stop = time.After(keepAlive)
		}
	loop:
		for {
			select {
			case <-r.Context().Done():
				return
			case <-stop:
				break loop
			case <-tick.C:
				if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
					return
				}
				flush(w)
			}
		}
		_, _ = io.WriteString(w, tail)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func stallClient(t *testing.T, url string, firstOutput, progress time.Duration) provider.Provider {
	t.Helper()
	p, err := New(provider.Config{Name: "deepseek", BaseURL: url, Model: "deepseek-v4", APIKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Idle is far beyond the test: only the output deadlines may fire.
	p.(*client).streamWatch = provider.StreamWatchConfig{Idle: 30 * time.Second, FirstOutput: firstOutput, Progress: progress}
	return p
}

// drain collects the stream's text and terminal error, failing if it hangs.
func drain(t *testing.T, p provider.Provider) (text string, streamErr error) {
	t.Helper()
	ch, err := p.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var b strings.Builder
	deadline := time.After(10 * time.Second)
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				return b.String(), streamErr
			}
			switch chunk.Type {
			case provider.ChunkText, provider.ChunkReasoning:
				b.WriteString(chunk.Text)
			case provider.ChunkError:
				streamErr = chunk.Err
			}
		case <-deadline:
			t.Fatal("stream hung: keep-alives kept a dead generation open")
		}
	}
}

// A generation that emits one token and then only keep-alives must be cut and
// replayed. Observed against DeepSeek: one reasoning delta, then 320s without
// output while the body reader stayed active, so the idle timeout never fired.
func TestStreamKeepAlivesAfterOutputStall(t *testing.T) {
	srv := keepAliveServer(t, sseRoleOnly+sseLet, 0, "")
	text, err := drain(t, stallClient(t, srv.URL, 30*time.Second, 150*time.Millisecond))
	if text != "Let" {
		t.Errorf("text = %q, want the partial output %q forwarded before the cut", text, "Let")
	}
	if got := provider.StreamInterruptReason(err); got != provider.StreamInterruptIdleTimeout {
		t.Fatalf("err = %v (reason %q), want a replayable idle_timeout interrupt", err, got)
	}
}

// Keep-alives and role-only scaffolding before the first output are a queued
// request or a hidden-reasoning model, not a stall: the short progress window
// must not apply until output has started.
func TestStreamKeepAlivesBeforeFirstOutputAreTolerated(t *testing.T) {
	srv := keepAliveServer(t, sseRoleOnly, 500*time.Millisecond, sseOK)
	text, err := drain(t, stallClient(t, srv.URL, 30*time.Second, 100*time.Millisecond))
	if err != nil || text != "ok" {
		t.Fatalf("text = %q, err = %v; want the queued response delivered intact", text, err)
	}
}

// Keep-alives with no output at all are still bounded.
func TestStreamKeepAlivesWithoutFirstOutputStall(t *testing.T) {
	srv := keepAliveServer(t, sseRoleOnly, 0, "")
	_, err := drain(t, stallClient(t, srv.URL, 150*time.Millisecond, 30*time.Second))
	if got := provider.StreamInterruptReason(err); got != provider.StreamInterruptIdleTimeout {
		t.Fatalf("err = %v (reason %q), want a replayable idle_timeout interrupt", err, got)
	}
}
