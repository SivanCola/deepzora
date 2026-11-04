package botchannels

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	im "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/larksuite/oapi-sdk-go/v3/ws"
)

// The SDK's default logger can include server data. Statuses below are the only
// diagnostics exported by this credential-owning adapter.
type quietLogger struct{}

func (quietLogger) Debug(context.Context, ...interface{}) {}
func (quietLogger) Info(context.Context, ...interface{})  {}
func (quietLogger) Warn(context.Context, ...interface{})  {}
func (quietLogger) Error(context.Context, ...interface{}) {}

type feishuAdapter struct {
	config    Saved
	client    *http.Client
	mu        sync.Mutex
	token     string
	expires   time.Time
	startedAt int64
}

func (f *feishuAdapter) domain() string {
	if f.config.Provider == "lark" {
		return "https://open.larksuite.com"
	}
	return "https://open.feishu.cn"
}
func (f *feishuAdapter) Run(ctx context.Context, receive func(Incoming) error, status func(string)) error {
	if f.startedAt == 0 {
		f.startedAt = time.Now().UnixMilli()
	}
	handler := dispatcher.NewEventDispatcher("", "").OnP2MessageReceiveV1(func(_ context.Context, e *im.P2MessageReceiveV1) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e == nil || e.Event == nil || e.Event.Message == nil || e.Event.Sender == nil {
			return nil
		}
		m, s := e.Event.Message, e.Event.Sender
		if m.CreateTime == nil {
			return nil
		}
		created, err := strconv.ParseInt(*m.CreateTime, 10, 64)
		if err != nil || created < f.startedAt {
			return nil
		}
		if s.SenderType == nil || *s.SenderType != "user" || s.SenderId == nil || s.SenderId.OpenId == nil || m.MessageId == nil || m.ChatId == nil {
			return nil
		}
		text := ""
		if m.MessageType != nil && *m.MessageType == "text" && m.Content != nil {
			var body struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(*m.Content), &body) == nil {
				text = body.Text
			}
		}
		chatID := *m.ChatId
		chatType := ""
		if m.ChatType != nil {
			chatType = *m.ChatType
		}
		chatID, allowed := feishuInboundChat(f.config.Config, chatType, chatID, *s.SenderId.OpenId)
		if !allowed {
			return nil
		}
		return receive(Incoming{ID: *m.MessageId, ChatID: chatID, SenderID: *s.SenderId.OpenId, Text: text})
	})
	dialer := *websocket.DefaultDialer
	if transport, ok := f.client.Transport.(*http.Transport); ok {
		dialer.Proxy = transport.Proxy
		dialer.NetDialContext = transport.DialContext
	}
	client := ws.NewClient(f.config.AppID, f.config.Secret, ws.WithDomain(f.domain()), ws.WithHttpClient(f.client), ws.WithWebSocketDialer(&dialer), ws.WithLogger(quietLogger{}), ws.WithEventHandler(handler), ws.WithOnReady(func() { status("connected") }), ws.WithOnReconnecting(func() { status("reconnecting") }), ws.WithOnReconnected(func() { status("connected") }), ws.WithOnDisconnected(func() { status("reconnecting") }), ws.WithOnError(func(error) { status("reconnecting") }))
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("Feishu/Lark connection failed; check app credentials, event subscriptions and permissions")
	}
	return nil
}

func feishuInboundChat(config Config, chatType, chatID, sender string) (string, bool) {
	if kind, recipient := PrivateFeishuRecipient(config); kind == "open_id" {
		if chatType != "p2p" || sender != recipient {
			return "", false
		}
		return config.ChatID, true
	}
	return chatID, true
}
func (f *feishuAdapter) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Now().Before(f.expires) {
		return f.token, nil
	}
	var res struct {
		Code   int    `json:"code"`
		Token  string `json:"tenant_access_token"`
		Expire int    `json:"expire"`
	}
	if err := request(ctx, f.client, f.domain()+"/open-apis/auth/v3/tenant_access_token/internal", nil, map[string]string{"app_id": f.config.AppID, "app_secret": f.config.Secret}, &res); err != nil {
		return "", err
	}
	if res.Code != 0 || res.Token == "" {
		return "", fmt.Errorf("Feishu/Lark credentials rejected")
	}
	f.token = res.Token
	f.expires = time.Now().Add(time.Duration(max(0, res.Expire-60)) * time.Second)
	return f.token, nil
}
func (f *feishuAdapter) Send(ctx context.Context, text string) error {
	token, err := f.accessToken(ctx)
	if err != nil {
		return err
	}
	for _, part := range chunks(text, 1800) {
		recipientType, recipient := PrivateFeishuRecipient(f.config.Config)
		content, _ := json.Marshal(map[string]string{"text": part})
		var res struct {
			Code int `json:"code"`
		}
		if err := request(ctx, f.client, f.domain()+"/open-apis/im/v1/messages?receive_id_type="+recipientType, map[string]string{"Authorization": "Bearer " + token}, map[string]string{"receive_id": recipient, "msg_type": "text", "content": string(content)}, &res); err != nil {
			return err
		}
		if res.Code != 0 {
			return fmt.Errorf("Feishu/Lark reply rejected; check bot messaging permissions")
		}
	}
	return nil
}
