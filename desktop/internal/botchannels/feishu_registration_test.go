package botchannels

import (
	"net/http"
	"strings"
	"testing"
)

func TestFeishuQRRegistrationSwitchAndCredentials(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/oauth/v1/app/registration" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatal("wrong registration endpoint")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if r.URL.Host != "accounts.feishu.cn" || r.Form.Get("archetype") != "PersonalAgent" || r.Form.Get("request_user_info") != "open_id" {
				t.Fatal("registration must start at Feishu with scanner identity")
			}
			return jsonResponse(`{"device_code":"opaque-code","verification_uri_complete":"https://open.feishu.cn/oauth/authorize?code=example","interval":5,"expire_in":600}`), nil
		}
		if r.Form.Get("device_code") != "opaque-code" {
			t.Fatal("device code missing")
		}
		if calls == 2 {
			return jsonResponse(`{"user_info":{"tenant_brand":"lark"}}`), nil
		}
		if r.URL.Host != "accounts.larksuite.com" {
			t.Fatal("Lark issuer not used")
		}
		return jsonResponse(`{"client_id":"cli_fixture","client_secret":"secret_fixture","user_info":{"tenant_brand":"lark","open_id":"ou_scanner"}}`), nil
	})}
	begin, err := BeginFeishuRegistration(t.Context(), client)
	if err != nil {
		t.Fatal(err)
	}
	if begin.Code != "opaque-code" || !strings.HasPrefix(begin.Image, "data:image/png;base64,") || begin.Interval != 5 {
		t.Fatal("begin result invalid")
	}
	switched, err := PollFeishuRegistration(t.Context(), client, begin.Domain, begin.Code)
	if err != nil || switched.Status != "pending" || switched.Domain != "lark" {
		t.Fatalf("switch failed: %+v %v", switched, err)
	}
	complete, err := PollFeishuRegistration(t.Context(), client, switched.Domain, begin.Code)
	if err != nil || complete.Status != "success" || complete.AppID != "cli_fixture" || complete.Secret != "secret_fixture" || complete.UserID != "ou_scanner" {
		t.Fatal("credentials or scanner identity missing")
	}
}
func TestFeishuQRRegistrationTerminalAndSlowDown(t *testing.T) {
	for _, tc := range []struct {
		wire, status string
		interval     int
	}{{"authorization_pending", "pending", 5}, {"slow_down", "pending", 10}, {"access_denied", "denied", 5}, {"expired_token", "expired", 5}} {
		client := &http.Client{Transport: providerTransport(func(*http.Request) (*http.Response, error) {
			r := jsonResponse(`{"error":"` + tc.wire + `","error_description":"credential-like-sensitive-data"}`)
			r.StatusCode = 400
			return r, nil
		})}
		result, err := PollFeishuRegistration(t.Context(), client, "feishu", "code")
		if err != nil || result.Status != tc.status || result.Interval != tc.interval {
			t.Fatalf("state %s failed: %v", tc.wire, err)
		}
	}
}
func TestFeishuQRPrivateBindingRejectsGroupAndDifferentSender(t *testing.T) {
	config := Config{ChatID: "openid:ou_scanner", SenderID: "ou_scanner"}
	for _, tc := range []struct {
		kind, sender string
		allowed      bool
	}{{"p2p", "ou_scanner", true}, {"group", "ou_scanner", false}, {"p2p", "ou_other", false}, {"", "ou_scanner", false}} {
		chat, allowed := feishuInboundChat(config, tc.kind, "oc_actual", tc.sender)
		if allowed != tc.allowed {
			t.Fatalf("unexpected access: %+v", tc)
		}
		if allowed && chat != config.ChatID {
			t.Fatal("private routing mismatch")
		}
	}
	kind, id := PrivateFeishuRecipient(config)
	if kind != "open_id" || id != "ou_scanner" {
		t.Fatal("reply target mismatch")
	}
	config.ChatID = "oc_group"
	kind, id = PrivateFeishuRecipient(config)
	if kind != "chat_id" || id != "oc_group" {
		t.Fatal("manual chat target changed")
	}
}
