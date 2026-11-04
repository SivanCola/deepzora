package botchannels

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreAndReceiptsSurviveRestart(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	c := Saved{Config: Config{ID: "test", Name: "Bot", Provider: "telegram", SessionID: "session", ChatID: "1", SenderID: "2"}, Secret: "secret"}
	if err := s.Edit(func([]Saved) ([]Saved, error) { return []Saved{c}, nil }); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Read()
	if err != nil || len(loaded) != 1 || loaded[0] != c {
		t.Fatalf("read %v %v", loaded, err)
	}
	assertCredentialPrivate(t, filepath.Join(s.Dir, "connections.json"))
	if err := s.Edit(func(entries []Saved) ([]Saved, error) { return entries, nil }); err != nil {
		t.Fatal(err)
	}
	assertCredentialPrivate(t, filepath.Join(s.Dir, "connections.json"))
	path := filepath.Join(s.Dir, "receipts.json")
	r := Receipts{Path: path}
	if ok, err := r.Claim("message"); !ok || err != nil {
		t.Fatalf("claim %v %v", ok, err)
	}
	restarted := Receipts{Path: path}
	if ok, err := restarted.Claim("message"); ok || err != nil {
		t.Fatalf("replayed command %v %v", ok, err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Claim("new"); err == nil {
		t.Fatal("corrupt journal accepted")
	}
}

type providerTransport func(*http.Request) (*http.Response, error)

func (f providerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestFeishuAndLarkUseCorrectDomainAndCacheToken(t *testing.T) {
	for _, provider := range []string{"feishu", "lark"} {
		t.Run(provider, func(t *testing.T) {
			tokens, sends := 0, 0
			domain := "open.feishu.cn"
			if provider == "lark" {
				domain = "open.larksuite.com"
			}
			client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != domain {
					t.Fatalf("wrong domain %s", r.URL.Host)
				}
				if strings.Contains(r.URL.Path, "tenant_access_token") {
					tokens++
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["app_id"] != "app" || body["app_secret"] != "secret" {
						t.Fatal("wrong credentials")
					}
					return jsonResponse(`{"code":0,"tenant_access_token":"tenant-token","expire":7200}`), nil
				}
				if r.Header.Get("Authorization") != "Bearer tenant-token" || r.URL.Query().Get("receive_id_type") != "chat_id" {
					t.Fatal("missing send identity")
				}
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["receive_id"] != "chat" || body["msg_type"] != "text" {
					t.Fatal("wrong target")
				}
				sends++
				return jsonResponse(`{"code":0}`), nil
			})}
			f := &feishuAdapter{config: Saved{Config: Config{Provider: provider, AppID: "app", ChatID: "chat"}, Secret: "secret"}, client: client}
			if err := f.Send(t.Context(), "first"); err != nil {
				t.Fatal(err)
			}
			if err := f.Send(t.Context(), "second"); err != nil {
				t.Fatal(err)
			}
			if tokens != 1 || sends != 2 {
				t.Fatalf("tokens=%d sends=%d", tokens, sends)
			}
		})
	}
}
func TestWeixinLoginKeepsCredentialOutOfQRCode(t *testing.T) {
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "ilinkai.weixin.qq.com" {
			t.Fatal("wrong login endpoint")
		}
		if strings.HasSuffix(r.URL.Path, "get_bot_qrcode") {
			return jsonResponse(`{"ret":0,"qrcode":"code+with?characters","qrcode_img_content":"https://example.invalid/login"}`), nil
		}
		if r.URL.Query().Get("qrcode") != "code+with?characters" {
			t.Fatal("QR code not URL encoded")
		}
		return jsonResponse(`{"ret":0,"status":"confirmed","bot_token":"write-only-token","ilink_user_id":"scanner@im.wechat","ilink_bot_id":"robot@im.bot"}`), nil
	})}
	code, image, err := BeginWeixinLogin(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(image, "data:image/png;base64,"))
	if err != nil || !strings.HasPrefix(string(png), "\x89PNG") {
		t.Fatal("QR is not a PNG")
	}
	result, err := PollWeixinLogin(t.Context(), client, code)
	if err != nil || result.Status != "success" || result.Token != "write-only-token" || result.UserID != "scanner@im.wechat" {
		t.Fatalf("login status=%s user=%s err=%v", result.Status, result.UserID, err)
	}
	if strings.Contains(image, result.Token) {
		t.Fatal("credential leaked into image")
	}
}
func TestWeixinLoginDoesNotInferPeerFromBotID(t *testing.T) {
	client := &http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"status":"confirmed","bot_token":"fixture-token","ilink_bot_id":"robot@im.bot"}`), nil
	})}
	result, err := PollWeixinLogin(t.Context(), client, "code")
	if err != nil || result.Status != "success" || result.UserID != "" {
		t.Fatalf("missing scanner ID was replaced: status=%s user=%s err=%v", result.Status, result.UserID, err)
	}
}
func TestValidateRejectsPublicAccess(t *testing.T) {
	c := Config{ID: "test", Name: "Bot", Provider: "telegram", SessionID: "s", ChatID: "c", SenderID: "u"}
	for _, mutate := range []func(*Config){func(c *Config) { c.ChatID = "" }, func(c *Config) { c.SenderID = "" }, func(c *Config) { c.SessionID = "" }, func(c *Config) { c.Provider = "unknown" }, func(c *Config) { c.ID = "../escape" }} {
		bad := c
		mutate(&bad)
		if Validate(bad) == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
func TestTelegramPollingAndReply(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var sent []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "getMe"):
			fmt.Fprint(w, `{"ok":true}`)
		case strings.HasSuffix(r.URL.Path, "getUpdates"):
			var body struct {
				Offset int64 `json:"offset"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Offset == -1 {
				fmt.Fprint(w, `{"ok":true,"result":[{"update_id":11,"message":{"text":"old session input","from":{"id":2},"chat":{"id":1}}}]}`)
				return
			}
			if body.Offset != 12 {
				t.Error("old backlog was not acknowledged")
			}
			fmt.Fprint(w, `{"ok":true,"result":[{"update_id":12,"message":{"message_id":4,"text":"hello","from":{"id":2},"chat":{"id":1}}}]}`)
		case strings.HasSuffix(r.URL.Path, "sendMessage"):
			var body struct {
				Text string `json:"text"`
				Chat string `json:"chat_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Chat != "1" {
				t.Error("wrong chat")
			}
			sent = append(sent, body.Text)
			fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Error("unexpected request")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	adapter := &telegramAdapter{config: Saved{Config: Config{ChatID: "1"}, Secret: "private-token"}, client: server.Client(), base: server.URL}
	var got Incoming
	err := adapter.Run(ctx, func(m Incoming) error { got = m; cancel(); return nil }, func(string) {})
	if !errors.Is(err, context.Canceled) || got.ID != "12" || got.SenderID != "2" || got.ChatID != "1" || got.Text != "hello" {
		t.Fatalf("incoming %+v %v", got, err)
	}
	if err := adapter.Send(t.Context(), strings.Repeat("好", 4000)); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 3 || strings.Join(sent, "") != strings.Repeat("好", 4000) {
		t.Fatal("reply splitting lost unicode")
	}
}
func TestHTTPRedactsCredentialAndRejectsRedirect(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++; fmt.Fprint(w, `{}`) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	var result any
	err := request(t.Context(), providerHTTP, redirect.URL+"/secret-token", map[string]string{"Authorization": "secret"}, map[string]string{}, &result)
	if err == nil || strings.Contains(err.Error(), "secret") || targetCalls != 0 {
		t.Fatalf("unsafe redirect: %v calls=%d", err, targetCalls)
	}
}
func TestWeixinReplyRequiresAuthorizedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sends := 0
	polls := 0
	created := time.Now().Add(time.Second).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("AuthorizationType") != "ilink_bot_token" || r.Header.Get("Authorization") != "Bearer token" {
			t.Error("auth missing")
		}
		if strings.HasSuffix(r.URL.Path, "getconfig") {
			fmt.Fprint(w, `{"ret":0}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "getupdates") {
			polls++
			if polls == 1 {
				fmt.Fprint(w, `{"ret":0,"get_updates_buf":"old","msgs":[{"message_id":1,"create_time_ms":1,"message_type":1,"from_user_id":"allowed","context_token":"old-context","item_list":[{"type":1,"text_item":{"text":"old session message"}}]}]}`)
				return
			}
			if polls == 2 {
				fmt.Fprint(w, `{"ret":0,"get_updates_buf":"initial","msgs":[]}`)
				return
			}
			fmt.Fprintf(w, `{"ret":0,"get_updates_buf":"next","msgs":[{"message_id":7,"create_time_ms":%d,"message_type":1,"from_user_id":"attacker","context_token":"bad","item_list":[{"type":1,"text_item":{"text":"x"}}]},{"message_id":8,"create_time_ms":%d,"message_type":1,"from_user_id":"allowed","context_token":"good","item_list":[{"type":1,"text_item":{"text":"hello"}}]}]}`, created, created)
			return
		}
		var body struct {
			Msg struct {
				To      string `json:"to_user_id"`
				Context string `json:"context_token"`
			} `json:"msg"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Msg.Context != "good" || body.Msg.To != "allowed" {
			t.Error("wrong reply identity")
		}
		sends++
		fmt.Fprint(w, `{"ret":0}`)
	}))
	defer server.Close()
	adapter := &weixinAdapter{config: Saved{Config: Config{ChatID: "allowed", SenderID: "allowed"}, Secret: "token"}, client: server.Client(), base: server.URL}
	noContext, cancelNoContext := context.WithCancel(t.Context())
	cancelNoContext()
	if adapter.Send(noContext, "premature") == nil {
		t.Fatal("sent without context")
	}
	err := adapter.Run(ctx, func(m Incoming) error {
		if m.ID == "1" {
			t.Error("pre-connection backlog reached the new session")
		}
		if m.SenderID == "allowed" {
			cancel()
		}
		return nil
	}, func(string) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := adapter.Send(t.Context(), "answer"); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatal("reply missing")
	}
}
