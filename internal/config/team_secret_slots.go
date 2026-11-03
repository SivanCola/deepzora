package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"deepzora/internal/fileutil"
)

var teamSecretSlotName = regexp.MustCompile(`^ts-[0-9a-f]{32}$`)

// TeamSecretSlots deliberately does not use the .env credential backend. Values
// are immutable private files and are never registered as process environment.
// This is local unencrypted storage, not isolation from same-user programs.
type TeamSecretSlots struct{ Dir string }

func DefaultTeamSecretSlots() TeamSecretSlots {
	return TeamSecretSlots{Dir: filepath.Join(filepath.Dir(UserCredentialsPath()), "team-secret-slots")}
}
func (s TeamSecretSlots) Save(ctx context.Context, value string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.Dir == "" || len(value) < 4 || len(value) > 16<<10 {
		return "", errors.New("invalid Team secret slot value")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return "", err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	slot := "ts-" + hex.EncodeToString(id)
	if err := fileutil.AtomicWriteFileStrict(filepath.Join(s.Dir, slot), []byte(value), 0600); err != nil {
		return "", err
	}
	return slot, nil
}
func (s TeamSecretSlots) Read(slot string) (string, error) {
	if s.Dir == "" || !teamSecretSlotName.MatchString(slot) {
		return "", errors.New("invalid Team secret slot reference")
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return "", errors.New("Team secret storage unavailable")
	}
	defer root.Close()
	info, err := root.Lstat(slot)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<10 {
		return "", errors.New("Team secret slot unavailable")
	}
	body, err := root.ReadFile(slot)
	if err != nil {
		return "", errors.New("Team secret slot unavailable")
	}
	if len(body) < 4 {
		return "", errors.New("Team secret slot invalid")
	}
	return string(body), nil
}
