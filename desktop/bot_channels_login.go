package main

import (
	"context"
	"errors"
	"time"

	"deepzora/desktop/internal/botchannels"
	"github.com/google/uuid"
)

type BotLoginView struct {
	ID       string `json:"id"`
	Image    string `json:"image"`
	Status   string `json:"status"`
	UserID   string `json:"userId,omitempty"`
	ChatID   string `json:"chatId,omitempty"`
	AppID    string `json:"appId,omitempty"`
	Provider string `json:"provider,omitempty"`
	Interval int    `json:"interval,omitempty"`
}
type botLoginAttempt struct {
	provider, domain, appID string
	interval                int
	nextPoll                time.Time
	code, secret, userID    string
	expires                 time.Time
	polling                 bool
}

func (a *App) BeginWeixinBotLogin() (BotLoginView, error) {
	ctx, cancel := context.WithTimeout(a.bootContext(), 35*time.Second)
	defer cancel()
	client, err := botchannels.HTTPClient(a.networkProxySpecForRoot(""))
	if err != nil {
		return BotLoginView{}, err
	}
	defer client.CloseIdleConnections()
	code, image, err := botchannels.BeginWeixinLogin(ctx, client)
	if err != nil {
		return BotLoginView{}, err
	}
	a.botChannels.mu.Lock()
	defer a.botChannels.mu.Unlock()
	if a.botChannels.logins == nil {
		a.botChannels.logins = map[string]*botLoginAttempt{}
	}
	for id, l := range a.botChannels.logins {
		if time.Now().After(l.expires) {
			delete(a.botChannels.logins, id)
		}
	}
	if len(a.botChannels.logins) >= 8 {
		return BotLoginView{}, errors.New("too many pending logins; wait for an existing login to expire")
	}
	id := uuid.NewString()
	a.botChannels.logins[id] = &botLoginAttempt{provider: "weixin", code: code, expires: time.Now().Add(3 * time.Minute)}
	return BotLoginView{ID: id, Image: image, Status: "pending"}, nil
}
func (a *App) PollWeixinBotLogin(id string) (BotLoginView, error) {
	a.botChannels.mu.Lock()
	attempt := a.botChannels.logins[id]
	if attempt == nil || time.Now().After(attempt.expires) {
		delete(a.botChannels.logins, id)
		a.botChannels.mu.Unlock()
		return BotLoginView{ID: id, Status: "expired"}, nil
	}
	if attempt.secret != "" {
		view := BotLoginView{ID: id, Status: "success", UserID: attempt.userID}
		a.botChannels.mu.Unlock()
		return view, nil
	}
	if attempt.provider != "" && attempt.provider != "weixin" {
		a.botChannels.mu.Unlock()
		return BotLoginView{}, errors.New("login provider mismatch")
	}
	if attempt.polling {
		a.botChannels.mu.Unlock()
		return BotLoginView{ID: id, Status: "pending"}, nil
	}
	attempt.polling = true
	a.botChannels.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.bootContext(), 35*time.Second)
	defer cancel()
	client, err := botchannels.HTTPClient(a.networkProxySpecForRoot(""))
	if err != nil {
		a.botChannels.mu.Lock()
		attempt.polling = false
		a.botChannels.mu.Unlock()
		return BotLoginView{}, err
	}
	defer client.CloseIdleConnections()
	result, err := botchannels.PollWeixinLogin(ctx, client, attempt.code)
	a.botChannels.mu.Lock()
	defer a.botChannels.mu.Unlock()
	attempt.polling = false
	if err != nil {
		if ctx.Err() != nil {
			return BotLoginView{ID: id, Status: "pending"}, nil
		}
		return BotLoginView{}, err
	}
	if time.Now().After(attempt.expires) {
		delete(a.botChannels.logins, id)
		return BotLoginView{ID: id, Status: "expired"}, nil
	}
	if result.Status == "success" {
		attempt.secret = result.Token
		attempt.userID = result.UserID
		attempt.expires = time.Now().Add(10 * time.Minute)
	}
	return BotLoginView{ID: id, Status: result.Status, UserID: result.UserID}, nil
}
