package botchannels

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Receipts belongs to the exclusively locked provider consumer. Recording before
// dispatch is deliberate: a crash may require an explicit user resend, but an old
// /stop or /approve must never act on a later task after reconnect or rebinding.
// Text additionally uses the controller's durable submission identity.
type Receipts struct {
	mu   sync.Mutex
	Path string
}

func (r *Receipts) Claim(id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	b, err := os.ReadFile(r.Path)
	if err != nil && !os.IsNotExist(err) {
		return false, errors.New("cannot read bot receipts")
	}
	if len(b) > 0 && (len(b) > 2<<20 || json.Unmarshal(b, &ids) != nil) {
		return false, errors.New("invalid bot receipts")
	}
	sum := sha256.Sum256([]byte(id))
	key := hex.EncodeToString(sum[:])
	if slices.Contains(ids, key) {
		return false, nil
	}
	// Fail closed rather than evicting deduplication identities and replaying an
	// old command. A fresh connection credential/account gets a separate journal.
	if len(ids) >= 20000 {
		return false, errors.New("bot receipt journal is full; archive this connection before continuing")
	}
	ids = append(ids, key)
	data, _ := json.Marshal(ids)
	f, err := os.CreateTemp(filepath.Dir(r.Path), ".receipts-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	if err = os.Rename(f.Name(), r.Path); err != nil {
		return false, err
	}
	return true, nil
}
