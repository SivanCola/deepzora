package botchannels

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type weixinAdapter struct {
	buf          string
	acceptSince  int64
	config       Saved
	client       *http.Client
	base         string
	mu           sync.Mutex
	contextToken string
	contextReady chan struct{}
}

func (w *weixinAdapter) call(ctx context.Context, path string, body map[string]any, out any) error {
	body["base_info"] = map[string]any{"channel_version": "1.0.0"}
	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	return request(ctx, w.client, w.base+"/ilink/bot/"+path, map[string]string{"AuthorizationType": "ilink_bot_token", "Authorization": "Bearer " + w.config.Secret, "X-WECHAT-UIN": base64.StdEncoding.EncodeToString(fmt.Append(nil, uint32(nonce[0])<<24|uint32(nonce[1])<<16|uint32(nonce[2])<<8|uint32(nonce[3])))}, body, out)
}
func (w *weixinAdapter) Run(ctx context.Context, receive func(Incoming) error, status func(string)) error {
	// Keep the original connection boundary across transient retries. Waiting for
	// an empty poll would discard new messages along with the initial backlog.
	if w.acceptSince == 0 {
		w.acceptSince = time.Now().UnixMilli()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	var config struct {
		Ret     *int `json:"ret"`
		ErrCode int  `json:"errcode"`
	}
	err := w.call(probeCtx, "getconfig", map[string]any{"ilink_user_id": w.config.SenderID}, &config)
	cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if config.Ret == nil || *config.Ret != 0 || config.ErrCode != 0 {
		return fmt.Errorf("Weixin rejected connection; sign in again")
	}
	status("connected")
	for ctx.Err() == nil {
		var response struct {
			Ret     int    `json:"ret"`
			ErrCode int    `json:"errcode"`
			Buf     string `json:"get_updates_buf"`
			Msgs    []struct {
				ID      json.Number `json:"message_id"`
				Created json.Number `json:"create_time_ms"`
				From    string      `json:"from_user_id"`
				Type    int         `json:"message_type"`
				Context string      `json:"context_token"`
				Items   []struct {
					Type int `json:"type"`
					Text struct {
						Text string `json:"text"`
					} `json:"text_item"`
				} `json:"item_list"`
			} `json:"msgs"`
		}
		if err := w.call(ctx, "getupdates", map[string]any{"get_updates_buf": w.buf}, &response); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if response.Ret != 0 || response.ErrCode != 0 {
			return fmt.Errorf("Weixin rejected polling; sign in again")
		}
		for _, m := range response.Msgs {
			created, err := m.Created.Int64()
			// Missing/invalid timestamps cannot establish that a message belongs
			// to this session binding, so fail closed before caching reply context.
			if m.Type != 1 || err != nil || created < w.acceptSince {
				continue
			}
			var text strings.Builder
			for _, item := range m.Items {
				if item.Type == 1 {
					text.WriteString(item.Text.Text)
				}
			}
			if m.From == w.config.SenderID && m.From == w.config.ChatID {
				w.mu.Lock()
				if w.contextReady == nil {
					w.contextReady = make(chan struct{})
				}
				if w.contextToken == "" && m.Context != "" {
					close(w.contextReady)
				}
				if m.Context != "" {
					w.contextToken = m.Context
				}
				w.mu.Unlock()
			}
			if err := receive(Incoming{ID: string(m.ID), ChatID: m.From, SenderID: m.From, Text: text.String(), ContextToken: m.Context}); err != nil {
				return err
			}
		}
		if response.Buf != "" {
			w.buf = response.Buf
		}
	}
	return ctx.Err()
}
func (w *weixinAdapter) Send(ctx context.Context, text string) error {
	w.mu.Lock()
	if w.contextReady == nil {
		w.contextReady = make(chan struct{})
	}
	ready := w.contextReady
	token := w.contextToken
	w.mu.Unlock()
	if token == "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ready:
		}
		w.mu.Lock()
		token = w.contextToken
		w.mu.Unlock()
	}
	for _, part := range chunks(text, 1800) {
		var nonce [16]byte
		_, _ = rand.Read(nonce[:])
		var result struct {
			Ret     int `json:"ret"`
			ErrCode int `json:"errcode"`
		}
		if err := w.call(ctx, "sendmessage", map[string]any{"msg": map[string]any{"from_user_id": "", "to_user_id": w.config.ChatID, "client_id": "deepzora-" + hex.EncodeToString(nonce[:]), "message_type": 2, "message_state": 2, "context_token": token, "item_list": []any{map[string]any{"type": 1, "text_item": map[string]string{"text": part}}}}}, &result); err != nil {
			return err
		}
		if result.Ret != 0 || result.ErrCode != 0 {
			return fmt.Errorf("Weixin reply rejected")
		}
	}
	return nil
}
