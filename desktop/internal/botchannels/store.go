// Package botchannels owns desktop chat connections, never agent/session state.
package botchannels

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"deepzora/internal/filelock"
	"deepzora/internal/secrets"
)

type Config struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	AppID           string `json:"appId"`
	SessionID       string `json:"sessionId"`
	ChatID          string `json:"chatId"`
	SenderID        string `json:"senderId"`
	Mode            string `json:"mode,omitempty"` // empty legacy values follow the shared session
	ModelRef        string `json:"modelRef,omitempty"`
	WorkspaceID     string `json:"workspaceId,omitempty"`
	MemberID        string `json:"memberId,omitempty"`
	LegacySessionID string `json:"legacySessionId,omitempty"`
}
type Saved struct {
	Config
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled,omitempty"`
}
type View struct {
	Config
	HasSecret      bool   `json:"hasSecret"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	ActualModel    string `json:"actualModel,omitempty"`
	ModelError     string `json:"modelError,omitempty"`
	Enabled        bool   `json:"enabled"`
	Received       int    `json:"received"`
	Sent           int    `json:"sent"`
	LastReceivedAt string `json:"lastReceivedAt,omitempty"`
	LastSentAt     string `json:"lastSentAt,omitempty"`
	MemberName     string `json:"memberName,omitempty"`
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func Validate(c Config) error {
	if !validID.MatchString(c.ID) {
		return errors.New("invalid connection ID")
	}
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 120 {
		return errors.New("connection name is required (maximum 120 bytes)")
	}
	switch c.Provider {
	case "telegram", "weixin":
	case "feishu", "lark":
		if strings.TrimSpace(c.AppID) == "" {
			return errors.New("app ID is required")
		}
		if strings.HasPrefix(c.ChatID, "openid:") && (strings.TrimPrefix(c.ChatID, "openid:") == "" || strings.TrimPrefix(c.ChatID, "openid:") != c.SenderID) {
			return errors.New("private chat recipient must match the allowed sender")
		}
	default:
		return errors.New("unsupported bot provider")
	}
	switch c.Mode {
	case "member":
		if !regexp.MustCompile(`^a-[0-9a-f]{16}$`).MatchString(c.MemberID) {
			return errors.New("choose a team member")
		}
	case "", "follow":
		if strings.TrimSpace(c.SessionID) == "" || strings.HasPrefix(c.SessionID, "desktop-bot-") {
			return errors.New("choose a shared local session, not another bot's dedicated session")
		}
	case "dedicated":
		if strings.TrimSpace(c.ModelRef) == "" || strings.TrimSpace(c.WorkspaceID) == "" {
			return errors.New("dedicated bots require a model and workspace")
		}
	default:
		return errors.New("unsupported bot model mode")
	}
	for _, s := range []string{c.SessionID, c.ModelRef, c.WorkspaceID} {
		if len(s) > 512 || strings.ContainsAny(s, "\r\n\x00") {
			return errors.New("invalid bot session or model identity")
		}
	}
	for _, s := range []string{c.ChatID, c.SenderID} {
		if strings.TrimSpace(s) == "" || len(s) > 256 || strings.ContainsAny(s, "\r\n\x00") {
			return errors.New("session, allowed chat and allowed sender are required")
		}
	}
	return nil
}

// Store serializes edits across app instances. Credentials never leave this layer
// through View. The file is user-readable only, like the existing .env storage.
type Store struct{ Dir string }

func (s Store) Read() ([]Saved, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, "connections.json"))
	if os.IsNotExist(err) {
		return []Saved{}, nil
	}
	if err != nil {
		return nil, errors.New("cannot read bot configuration")
	}
	var entries []Saved
	if len(b) > 1<<20 || json.Unmarshal(b, &entries) != nil {
		return nil, errors.New("invalid bot configuration")
	}
	seen := map[string]bool{}
	if len(entries) > 32 {
		return nil, errors.New("too many saved bot connections")
	}
	for _, entry := range entries {
		if Validate(entry.Config) != nil || seen[entry.ID] || strings.TrimSpace(entry.Secret) == "" || len(entry.Secret) > 8192 {
			return nil, errors.New("invalid saved bot connection")
		}
		seen[entry.ID] = true
	}
	if entries == nil {
		entries = []Saved{}
	}
	return entries, nil
}
func (s Store) Edit(fn func([]Saved) ([]Saved, error)) error {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	unlock, err := filelock.Acquire(ctx, filepath.Join(s.Dir, "config.lock"))
	if err != nil {
		return errors.New("bot configuration is busy")
	}
	defer unlock()
	entries, err := s.Read()
	if err != nil {
		return err
	}
	secrets.RememberValues(credentialValues(entries))
	entries, err = fn(entries)
	if err != nil {
		return err
	}
	// Mask both old and replacement credentials before a temporary file exists.
	secrets.RememberValues(credentialValues(entries))
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Dir, ".connections-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = RestrictCredentialFile(f); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(s.Dir, "connections.json"))
}
