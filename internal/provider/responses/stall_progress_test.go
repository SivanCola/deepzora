package responses

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
	sseHi             = "data: {\"type\":\"response.output_text.delta\",\"item_id\":\"m1\",\"delta\":\"hi\"}\n\n"
	sseReasoningAdded = "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"r1\",\"type\":\"reasoning\"}}\n\n"
	sseCompleted      = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\"}}\n\n"
)

// keepAliveServer writes head, then a keep-alive comment every 20ms for
// keepAlive (forever when zero), then tail. Bytes never stop, so the idle
// timeout cannot see a stall.
func keepAliveServer(t *testing.T, head string, keepAlive time.Duration, tail string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, head)
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

func drainWithProgress(t *testing.T, url string, progress time.Duration) (text string, streamErr error) {
	t.Helper()
	p := New(Config{Name: "responses", BaseURL: url, Model: "model", APIKey: "k"}).(*client)
	// Idle and first-output are far beyond the test: only Progress may fire.
	p.streamWatch = provider.StreamWatchConfig{Idle: 30 * time.Second, FirstOutput: 30 * time.Second, Progress: progress}
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
			case provider.ChunkText:
				b.WriteString(chunk.Text)
			case provider.ChunkError:
				streamErr = chunk.Err
			}
		case <-deadline:
			t.Fatal("stream hung: keep-alives kept a dead generation open")
		}
	}
}

// Keep-alives after output has started do not prove the generation is alive.
func TestStreamKeepAlivesAfterOutputStall(t *testing.T) {
	srv := keepAliveServer(t, sseHi, 0, "")
	text, err := drainWithProgress(t, srv.URL, 150*time.Millisecond)
	if text != "hi" {
		t.Errorf("text = %q, want the partial output forwarded before the cut", text)
	}
	if got := provider.StreamInterruptReason(err); got != provider.StreamInterruptIdleTimeout {
		t.Fatalf("err = %v (reason %q), want a replayable idle_timeout interrupt", err, got)
	}
}

// A model may reason silently after a text preamble; an open reasoning item
// must not be cut by the short progress window.
func TestStreamKeepAlivesDuringReasoningAreTolerated(t *testing.T) {
	srv := keepAliveServer(t, sseHi+sseReasoningAdded, 500*time.Millisecond, sseCompleted)
	text, err := drainWithProgress(t, srv.URL, 100*time.Millisecond)
	if err != nil || text != "hi" {
		t.Fatalf("text = %q, err = %v; want the response delivered intact", text, err)
	}
}
