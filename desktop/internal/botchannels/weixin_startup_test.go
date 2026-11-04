package botchannels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWeixinFirstPollKeepsMessageSentDuringConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	received := make(chan Incoming, 1)
	secondPoll := make(chan struct{})
	var polls atomic.Int32
	created := time.Now().Add(time.Second).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "getconfig") {
			fmt.Fprint(w, `{"ret":0,"typing_ticket":"fixture"}`)
			return
		}
		if polls.Add(1) == 1 {
			fmt.Fprintf(w, `{"ret":0,"get_updates_buf":"first","msgs":[{"message_id":1,"create_time_ms":%d,"message_type":1,"from_user_id":"allowed","context_token":"reply-context","item_list":[{"type":1,"text_item":{"text":"你是谁"}}]}]}`, created)
			return
		}
		close(secondPoll)
		<-ctx.Done()
	}))
	defer func() { cancel(); server.Close() }()
	adapter := &weixinAdapter{config: Saved{Config: Config{ChatID: "allowed", SenderID: "allowed"}, Secret: "fixture"}, client: server.Client(), base: server.URL}
	done := make(chan error, 1)
	go func() {
		done <- adapter.Run(ctx, func(m Incoming) error { received <- m; return nil }, func(string) {})
	}()
	select {
	case <-secondPoll:
	case <-time.After(3 * time.Second):
		t.Fatal("poll did not advance")
	}
	select {
	case message := <-received:
		if message.Text != "你是谁" {
			t.Fatal("first message changed")
		}
	default:
		t.Error("fresh first message was discarded during connection initialization")
	}
	cancel()
	<-done
}

func TestWeixinReadyBeforeLongPollCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	connected := false
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "getconfig") {
			var body struct {
				User string `json:"ilink_user_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.User != "allowed" {
				t.Error("missing configuration peer")
			}
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > 10*time.Second {
				t.Error("unbounded connection probe")
			}
			return jsonResponse(`{"ret":0}`), nil
		}
		if !connected {
			t.Error("connection waits for long poll")
		}
		cancel()
		return nil, r.Context().Err()
	})}
	w := &weixinAdapter{config: Saved{Config: Config{SenderID: "allowed"}}, client: client, base: "https://example.invalid"}
	err := w.Run(ctx, func(Incoming) error { t.Error("unexpected message"); return nil }, func(s string) { connected = s == "connected" })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestWeixinRejectsUnverifiedConnection(t *testing.T) {
	for _, body := range []string{`{"ret":-14}`, `{"ret":0,"errcode":-14}`, `{}`} {
		t.Run(body, func(t *testing.T) {
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(r.URL.Path, "getconfig") {
					t.Error("polled after failed authentication")
				}
				return jsonResponse(body), nil
			})}
			w := &weixinAdapter{client: client, base: "https://example.invalid"}
			if w.Run(t.Context(), func(Incoming) error { t.Error("unexpected message"); return nil }, func(string) { t.Error("unverified connection marked ready") }) == nil {
				t.Fatal("missing authentication error")
			}
		})
	}
}

func TestWeixinIgnoresOldAndUnverifiableMessages(t *testing.T) {
	for _, stamp := range []string{"", `,"create_time_ms":null`, `,"create_time_ms":0`, `,"create_time_ms":-1`, `,"create_time_ms":1`, `,"create_time_ms":1.5`} {
		t.Run(stamp, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			polls := 0
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "getconfig") {
					return jsonResponse(`{"ret":0}`), nil
				}
				polls++
				if polls == 2 {
					cancel()
					return nil, ctx.Err()
				}
				return jsonResponse(fmt.Sprintf(`{"ret":0,"msgs":[{"message_id":1%s,"message_type":1,"from_user_id":"allowed","context_token":"stale"}]}`, stamp)), nil
			})}
			w := &weixinAdapter{config: Saved{Config: Config{ChatID: "allowed", SenderID: "allowed"}}, client: client, base: "https://example.invalid"}
			_ = w.Run(ctx, func(Incoming) error { t.Error("unsafe backlog reached session"); return nil }, func(string) {})
			if w.contextToken != "" {
				t.Error("unsafe backlog supplied reply context")
			}
		})
	}
}

func TestWeixinRetryRetainsBoundaryAndCursor(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Model an existing connection: a message sent after its start but before
	// the retry must still reach the same bound session.
	boundary := time.Now().Add(-time.Minute).UnixMilli()
	polls := 0
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "getconfig") {
			return jsonResponse(`{"ret":0}`), nil
		}
		polls++
		var body struct {
			Buf string `json:"get_updates_buf"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if polls > 1 && body.Buf != "cursor" {
			t.Error("cursor lost across empty response/retry")
		}
		switch polls {
		case 1:
			return jsonResponse(`{"ret":0,"get_updates_buf":"cursor","msgs":[]}`), nil
		case 2:
			return jsonResponse(`{"ret":0,"msgs":[]}`), nil
		case 3:
			return nil, errors.New("temporary fixture failure")
		default:
			return jsonResponse(fmt.Sprintf(`{"ret":0,"msgs":[{"message_id":1,"create_time_ms":"%d","message_type":1,"from_user_id":"allowed","context_token":"fresh"}]}`, boundary+1)), nil
		}
	})}
	w := &weixinAdapter{acceptSince: boundary, client: client, base: "https://example.invalid"}
	received := 0
	receive := func(Incoming) error { received++; cancel(); return nil }
	if err := w.Run(ctx, receive, func(string) {}); !Retryable(err) {
		t.Fatalf("retry expected: %v", err)
	}
	if err := w.Run(ctx, receive, func(string) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if received != 1 {
		t.Fatalf("received %d messages", received)
	}
}
