package botchannels

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

type telegramAdapter struct {
	config      Saved
	client      *http.Client
	base        string
	offset      int64
	initialized bool
}

func (t *telegramAdapter) call(ctx context.Context, method string, body any, out any) error {
	return request(ctx, t.client, t.base+"/bot"+t.config.Secret+"/"+method, nil, body, out)
}
func (t *telegramAdapter) Run(ctx context.Context, receive func(Incoming) error, status func(string)) error {
	var check struct {
		OK bool `json:"ok"`
	}
	if err := t.call(ctx, "getMe", map[string]any{}, &check); err != nil {
		return err
	}
	if !check.OK {
		return fmt.Errorf("Telegram rejected credentials")
	}
	if t.initialized {
		status("connected")
	}
	for ctx.Err() == nil {
		var response struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int64 `json:"update_id"`
				Message  *struct {
					ID   int64  `json:"message_id"`
					Text string `json:"text"`
					From struct {
						ID    int64 `json:"id"`
						IsBot bool  `json:"is_bot"`
					} `json:"from"`
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
			} `json:"result"`
		}
		offset, timeout := t.offset, 25
		if !t.initialized {
			offset, timeout = -1, 0
		}
		if err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": timeout, "allowed_updates": []string{"message"}}, &response); err != nil {
			return err
		}
		if !response.OK {
			return fmt.Errorf("Telegram polling rejected; check token and existing webhook/poller")
		}
		for _, u := range response.Result {
			if m := u.Message; t.initialized && m != nil && !m.From.IsBot && m.From.ID > 0 {
				if err := receive(Incoming{ID: strconv.FormatInt(u.UpdateID, 10), ChatID: strconv.FormatInt(m.Chat.ID, 10), SenderID: strconv.FormatInt(m.From.ID, 10), Text: m.Text}); err != nil {
					return err
				}
			}
			t.offset = u.UpdateID + 1
		}
		if !t.initialized {
			t.initialized = true
			status("connected")
		}
	}
	return ctx.Err()
}
func (t *telegramAdapter) Send(ctx context.Context, text string) error {
	for _, part := range chunks(text, 1800) {
		var result struct {
			OK bool `json:"ok"`
		}
		if err := t.call(ctx, "sendMessage", map[string]any{"chat_id": t.config.ChatID, "text": part}, &result); err != nil {
			return err
		}
		if !result.OK {
			return fmt.Errorf("Telegram send rejected")
		}
	}
	return nil
}
