package anthropic

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
	ssePing       = "event: ping\ndata: {\"type\":\"ping\"}\n\n"
	sseTextStart  = "data: {\"type\":\"message_start\",\"message\":{}}\n\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\n"
	sseTextHi     = "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"
	sseTextStop   = "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"
	sseThinkStart = "data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"redacted_thinking\",\"data\":\"x\"}}\n\n"
	sseThinkStop  = "data: {\"type\":\"content_block_stop\",\"index\":1}\n\n"
	sseEnd        = "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
)

// pingServer writes head, then a ping event every 20ms for pings (forever when
// zero), then tail. Bytes never stop, so the idle timeout cannot see a stall.
func pingServer(t *testing.T, head string, pings time.Duration, tail string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, head)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		var stop <-chan time.Time
		if pings > 0 {
			stop = time.After(pings)
		}
	loop:
		for {
			select {
			case <-r.Context().Done():
				return
			case <-stop:
				break loop
			case <-tick.C:
				if _, err := io.WriteString(w, ssePing); err != nil {
					return
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
		_, _ = io.WriteString(w, tail)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func drainWithProgress(t *testing.T, url string, progress time.Duration) (text string, streamErr error) {
	t.Helper()
	p, err := New(provider.Config{Name: "claude", BaseURL: url, Model: "claude-opus-4-8", APIKey: "k"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Idle and first-output are far beyond the test: only Progress may fire.
	p.(*client).streamWatch = provider.StreamWatchConfig{Idle: 30 * time.Second, FirstOutput: 30 * time.Second, Progress: progress}
	ch, err := p.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}, MaxTokens: 16})
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
			case provider.ChunkText:
				b.WriteString(chunk.Text)
			case provider.ChunkError:
				streamErr = chunk.Err
			}
		case <-deadline:
			t.Fatal("stream hung: pings kept a dead generation open")
		}
	}
}

// Pings after output has started do not prove the generation is alive.
func TestStreamPingsAfterOutputStall(t *testing.T) {
	srv := pingServer(t, sseTextStart+sseTextHi, 0, "")
	text, err := drainWithProgress(t, srv.URL, 150*time.Millisecond)
	if text != "hi" {
		t.Errorf("text = %q, want the partial output forwarded before the cut", text)
	}
	if got := provider.StreamInterruptReason(err); got != provider.StreamInterruptIdleTimeout {
		t.Fatalf("err = %v (reason %q), want a replayable idle_timeout interrupt", err, got)
	}
}

// A thinking block that streams no text (redacted) after output has started is
// a legitimate silent phase: the short progress window must not cut it.
func TestStreamPingsDuringHiddenThinkingAreTolerated(t *testing.T) {
	head := sseTextStart + sseTextHi + sseTextStop + sseThinkStart
	srv := pingServer(t, head, 500*time.Millisecond, sseThinkStop+sseEnd)
	text, err := drainWithProgress(t, srv.URL, 100*time.Millisecond)
	if err != nil || text != "hi" {
		t.Fatalf("text = %q, err = %v; want the response delivered intact", text, err)
	}
}
