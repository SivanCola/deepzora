package service

import (
	"errors"
	"slices"
	"strings"
	"time"

	"deepzora/desktop/internal/botchannels"
)

func (a *App) SaveBotConnection(input BotConnectionInput) error {
	if err := a.normalizeBotInput(&input); err != nil {
		return err
	}
	a.botChannels.op.Lock()
	defer a.botChannels.op.Unlock()
	previous, err := a.botStore().Read()
	if err != nil {
		return err
	}
	if err := validateBotEdit(&input, previous); err != nil {
		return err
	}
	if err := a.stopBotChannel(input.ID); err != nil {
		return err
	}
	var editUnlock func()
	defer func() {
		if editUnlock != nil {
			editUnlock()
		}
	}()
	enabled := input.Connect
	err = a.botStore().Edit(func(entries []botchannels.Saved) ([]botchannels.Saved, error) {
		idx := slices.IndexFunc(entries, func(e botchannels.Saved) bool { return e.ID == input.ID })
		if idx >= 0 {
			var err error
			editUnlock, err = a.lockBotAccount(entries[idx])
			if err != nil {
				return nil, err
			}
		}
		secret := strings.TrimSpace(input.Secret)
		if secret == "" && idx >= 0 && entries[idx].Provider == input.Provider && entries[idx].AppID == input.AppID {
			secret = entries[idx].Secret
		}
		if secret == "" {
			return nil, errors.New("bot credential is required")
		}
		if idx >= 0 {
			if entries[idx].MemberID != "" && entries[idx].MemberID != input.MemberID {
				return nil, errors.New("channel belongs to a fixed member; create another channel to use a different member")
			}
			input.LegacySessionID = entries[idx].LegacySessionID
			enabled = enabled || entries[idx].Enabled
		}
		next := botchannels.Saved{Config: input.Config, Secret: secret, Enabled: enabled}
		if idx >= 0 {
			entries[idx] = next
		} else {
			if len(entries) >= 32 {
				return nil, errors.New("maximum 32 bot connections")
			}
			entries = append(entries, next)
		}
		return entries, nil
	})
	if editUnlock != nil {
		editUnlock()
		editUnlock = nil
	}
	if err != nil {
		return err
	}
	if enabled {
		_ = a.startSavedBotLocked(input.ID, false) // Saved successfully; connection errors remain on the card.
	}
	return nil
}

// Reject invalid edits before interrupting a currently connected channel.
func validateBotEdit(input *BotConnectionInput, previous []botchannels.Saved) error {
	prior := slices.IndexFunc(previous, func(c botchannels.Saved) bool { return c.ID == input.ID })
	if prior >= 0 && previous[prior].MemberID != "" && previous[prior].MemberID != input.MemberID {
		return errors.New("channel belongs to a fixed member; create another channel to use a different member")
	}
	if prior < 0 {
		input.LegacySessionID = ""
	}
	if strings.TrimSpace(input.Secret) == "" && (prior < 0 || previous[prior].Provider != input.Provider || previous[prior].AppID != input.AppID) {
		return errors.New("bot credential is required")
	}
	if prior < 0 && len(previous) >= 32 {
		return errors.New("maximum 32 bot connections")
	}
	return nil
}

func (a *App) normalizeBotInput(input *BotConnectionInput) error {
	if input.MemberID != "" {
		m, err := botTeamMember(input.MemberID)
		if err != nil {
			return err
		}
		input.Mode = "member"
		input.ModelRef = ""
		input.WorkspaceID = ""
		input.SessionID, _ = parseSessionRoute(m.SessionPath)
	}
	if input.Mode == "" {
		input.Mode = "follow"
	}
	if input.Mode == "dedicated" {
		if err := a.validateBotModel(input.Config); err != nil {
			return err
		}
		// Never accept a caller-supplied shared or other bot session as owned.
		input.SessionID = botDedicatedSession(input.Config)
	} else if input.Mode != "member" {
		input.ModelRef, input.WorkspaceID = "", ""
	}
	if err := a.applyBotLogin(input); err != nil {
		return err
	}
	if err := botchannels.Validate(input.Config); err != nil {
		return err
	}
	if len(input.Secret) > 8192 {
		return errors.New("credential exceeds limit")
	}
	return nil
}

func (a *App) applyBotLogin(input *BotConnectionInput) error {
	if input.LoginID != "" {
		a.botChannels.mu.Lock()
		attempt := a.botChannels.logins[input.LoginID]
		if attempt == nil || time.Now().After(attempt.expires) || attempt.secret == "" {
			a.botChannels.mu.Unlock()
			return errors.New("QR login expired; sign in again")
		}
		provider := attempt.provider
		if provider == "" {
			provider = "weixin"
		}
		if input.Provider != provider {
			a.botChannels.mu.Unlock()
			return errors.New("QR login provider mismatch")
		}
		input.Secret = attempt.secret
		chatID := attempt.userID
		if provider == "feishu" || provider == "lark" {
			input.AppID = attempt.appID
			chatID = feishuLoginChatID(attempt.userID)
		}
		// 自动识别只补空字段；用户明确编辑过的访问对象仍由保存表单决定。
		if strings.TrimSpace(input.ChatID) == "" {
			input.ChatID = chatID
		}
		if strings.TrimSpace(input.SenderID) == "" {
			input.SenderID = attempt.userID
		}
		a.botChannels.mu.Unlock()
	}
	return nil
}
