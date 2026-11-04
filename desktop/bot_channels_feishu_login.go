package main

import (
	"context"
	"errors"
	"time"

	"deepzora/desktop/internal/botchannels"
	"github.com/google/uuid"
)

func (a *App) BeginFeishuBotLogin(provider string) (BotLoginView, error) {
	if provider != "feishu" && provider != "lark" {
		return BotLoginView{}, errors.New("unsupported QR provider")
	}
	ctx, cancel := context.WithTimeout(a.bootContext(), 15*time.Second)
	defer cancel()
	client, err := botchannels.HTTPClient(a.networkProxySpecForRoot(""))
	if err != nil {
		return BotLoginView{}, err
	}
	defer client.CloseIdleConnections()
	result, err := botchannels.BeginFeishuRegistration(ctx, client)
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
	a.botChannels.logins[id] = &botLoginAttempt{code: result.Code, provider: provider, domain: result.Domain, interval: result.Interval, expires: time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)}
	return BotLoginView{ID: id, Image: result.Image, Status: "pending", Provider: provider, Interval: result.Interval}, nil
}
func (a *App) PollFeishuBotLogin(id string) (BotLoginView, error) {
	a.botChannels.mu.Lock()
	attempt := a.botChannels.logins[id]
	if attempt == nil || time.Now().After(attempt.expires) {
		delete(a.botChannels.logins, id)
		a.botChannels.mu.Unlock()
		return BotLoginView{ID: id, Status: "expired"}, nil
	}
	if attempt.provider != "feishu" && attempt.provider != "lark" {
		a.botChannels.mu.Unlock()
		return BotLoginView{}, errors.New("login provider mismatch")
	}
	view := BotLoginView{ID: id, Status: "pending", Provider: attempt.provider, Interval: attempt.interval}
	if attempt.secret != "" {
		view.Status = "success"
		view.AppID = attempt.appID
		view.UserID = attempt.userID
		view.ChatID = feishuLoginChatID(attempt.userID)
		a.botChannels.mu.Unlock()
		return view, nil
	}
	if attempt.polling || time.Now().Before(attempt.nextPoll) {
		a.botChannels.mu.Unlock()
		return view, nil
	}
	attempt.polling = true
	domain, code := attempt.domain, attempt.code
	a.botChannels.mu.Unlock()
	ctx, cancel := context.WithTimeout(a.bootContext(), 15*time.Second)
	defer cancel()
	client, err := botchannels.HTTPClient(a.networkProxySpecForRoot(""))
	var result botchannels.FeishuRegistration
	if err == nil {
		defer client.CloseIdleConnections()
		result, err = botchannels.PollFeishuRegistration(ctx, client, domain, code)
	}
	a.botChannels.mu.Lock()
	defer a.botChannels.mu.Unlock()
	attempt.polling = false
	if err != nil {
		return BotLoginView{}, err
	}
	if time.Now().After(attempt.expires) {
		delete(a.botChannels.logins, id)
		return BotLoginView{ID: id, Status: "expired"}, nil
	}
	attempt.domain = result.Domain
	attempt.interval = result.Interval
	attempt.nextPoll = time.Now().Add(time.Duration(result.Interval) * time.Second)
	if result.Status == "success" {
		attempt.provider = result.Domain
		attempt.secret = result.Secret
		attempt.appID = result.AppID
		attempt.userID = result.UserID
		attempt.expires = time.Now().Add(10 * time.Minute)
	}
	if result.Status == "expired" || result.Status == "denied" {
		delete(a.botChannels.logins, id)
	}
	return BotLoginView{ID: id, Status: result.Status, Provider: attempt.provider, AppID: result.AppID, UserID: result.UserID, ChatID: feishuLoginChatID(result.UserID), Interval: result.Interval}, nil
}
func feishuLoginChatID(userID string) string {
	if userID == "" {
		return ""
	}
	return "openid:" + userID
}
