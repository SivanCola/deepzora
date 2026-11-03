package teamroster

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"deepzora/internal/fileutil"
)

const SecretFile = "secrets.json"
const SecretEnvPrefix = "DEEPZORA_SECRET_"

var secretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,39}$`)
var secretSlotName = regexp.MustCompile(`^ts-[0-9a-f]{32}$`)
var ErrUnknownSecret = errors.New("no such Team secret")

type TeamSecret struct {
	Name      string    `json:"name"`
	Purpose   string    `json:"purpose"`
	Agents    []string  `json:"agents"`
	Slot      string    `json:"slot"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type TeamSecretRequest struct {
	ID      string    `json:"id"`
	Agent   string    `json:"agent"`
	Name    string    `json:"name"`
	Purpose string    `json:"purpose"`
	At      time.Time `json:"at"`
}
type RetiredTeamSecret struct {
	Name string    `json:"name"`
	Slot string    `json:"slot"`
	At   time.Time `json:"at"`
}
type SecretReceipt struct{ Key, Digest string }
type TeamSecretBook struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	Revision        uint64              `json:"revision"`
	Secrets         []TeamSecret        `json:"secrets"`
	Requests        []TeamSecretRequest `json:"requests"`
	Retired         []RetiredTeamSecret `json:"retired"`
	Receipts        []SecretReceipt     `json:"receipts"`
	PendingDeletion []string            `json:"pendingDeletion,omitempty"`
}

func NormalizeSecretName(name string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	if !secretName.MatchString(name) {
		return ""
	}
	return name
}
func (s Store) TeamSecrets() (TeamSecretBook, error) {
	book := TeamSecretBook{SchemaVersion: 1, Secrets: []TeamSecret{}, Requests: []TeamSecretRequest{}, Retired: []RetiredTeamSecret{}, Receipts: []SecretReceipt{}}
	raw, err := os.ReadFile(filepath.Join(s.Dir, SecretFile))
	if errors.Is(err, os.ErrNotExist) {
		return book, nil
	}
	if err != nil {
		return book, err
	}
	if err := json.Unmarshal(raw, &book); err != nil {
		return book, ErrCorrupt
	}
	if book.SchemaVersion != 1 {
		return book, ErrNewerSchema
	}
	if err := book.validate(); err != nil {
		return book, errors.Join(ErrCorrupt, err)
	}
	return book, nil
}
func (b *TeamSecretBook) validate() error {
	if b.Secrets == nil {
		b.Secrets = []TeamSecret{}
	}
	if b.Requests == nil {
		b.Requests = []TeamSecretRequest{}
	}
	if b.Retired == nil {
		b.Retired = []RetiredTeamSecret{}
	}
	if b.Receipts == nil {
		b.Receipts = []SecretReceipt{}
	}
	if len(b.Secrets) > 128 || len(b.Requests) > 1024 || len(b.Retired) > 2048 || len(b.PendingDeletion) > 2048 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, sc := range b.Secrets {
		if NormalizeSecretName(sc.Name) != sc.Name || sc.Slot == "" || seen[sc.Name] || len(sc.Purpose) > 800 {
			return ErrInvalid
		}
		seen[sc.Name] = true
		if sc.Agents == nil {
			return ErrInvalid
		}
		for _, id := range sc.Agents {
			if !agentID.MatchString(id) {
				return ErrInvalid
			}
		}
	}
	for _, q := range b.Requests {
		if q.ID == "" || !agentID.MatchString(q.Agent) || NormalizeSecretName(q.Name) != q.Name {
			return ErrInvalid
		}
	}
	for _, retired := range b.Retired {
		if NormalizeSecretName(retired.Name) != retired.Name || retired.Slot == "" {
			return ErrInvalid
		}
	}
	for _, slot := range b.PendingDeletion {
		if !secretSlotName.MatchString(slot) || slices.ContainsFunc(b.Secrets, func(sc TeamSecret) bool { return sc.Slot == slot }) || slices.ContainsFunc(b.Retired, func(sc RetiredTeamSecret) bool { return sc.Slot == slot }) {
			return ErrInvalid
		}
	}
	return nil
}

// MutateTeamSecrets never stores a value or a digest of plaintext in metadata.
// The caller's request digest binds secret input with the credential store's
// private keyed fingerprint, rather than a publicly brute-forceable hash.
func (s Store) MutateTeamSecrets(ctx context.Context, revision uint64, key, digest string, mutate func(*TeamSecretBook) error) (TeamSecretBook, error) {
	var result TeamSecretBook
	err := s.withLock(ctx, s.Dir, SecretFile, func() error {
		book, err := s.TeamSecrets()
		if err != nil {
			return err
		}
		if key == "" || len(key) > 200 || digest == "" {
			return ErrInvalid
		}
		for _, receipt := range book.Receipts {
			if receipt.Key == key {
				if receipt.Digest != digest {
					return ErrRequestReused
				}
				result = book
				return nil
			}
		}
		if book.Revision != revision {
			return ErrConflict
		}
		if err := mutate(&book); err != nil {
			return err
		}
		if err := book.validate(); err != nil {
			return err
		}
		book.Revision++
		book.Receipts = append(book.Receipts, SecretReceipt{key, digest})
		if len(book.Receipts) > 512 {
			book.Receipts = book.Receipts[len(book.Receipts)-512:]
		}
		raw, err := json.Marshal(book)
		if err != nil {
			return err
		}
		if err := fileutil.AtomicWriteFileStrict(filepath.Join(s.Dir, SecretFile), raw, 0600); err != nil {
			return err
		}
		result = book
		return nil
	})
	return result, err
}
func (b *TeamSecretBook) Retire(sc TeamSecret, now time.Time) {
	if !slices.ContainsFunc(b.Retired, func(old RetiredTeamSecret) bool { return old.Slot == sc.Slot }) {
		b.Retired = append(b.Retired, RetiredTeamSecret{Name: sc.Name, Slot: sc.Slot, At: now})
	}
}
func NewSecretRequestID() string { return "sr-" + randomHex() }
