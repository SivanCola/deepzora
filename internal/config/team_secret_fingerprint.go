package config

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"deepzora/internal/fileutil"
	"deepzora/internal/identitylock"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Fingerprint binds private request content without placing a brute-forceable
// unkeyed digest next to the publicly queryable metadata. The key stays inside
// the same local credential boundary as the immutable slots.
func (s TeamSecretSlots) Fingerprint(ctx context.Context, body []byte) (string, error) {
	if s.Dir == "" {
		return "", errors.New("Team secret storage unavailable")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	release, err := identitylock.Acquire(ctx, filepath.Join(s.Dir, "fingerprint.lock"))
	if err != nil {
		return "", err
	}
	defer release()
	keyPath := filepath.Join(s.Dir, "fingerprint-key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return "", err
		}
		err = fileutil.AtomicWriteFileStrict(keyPath, key, 0600)
	}
	if err != nil || len(key) != 32 {
		return "", errors.New("Team secret request fingerprint unavailable")
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil)), nil
}
