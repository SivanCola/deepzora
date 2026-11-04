// Package botcredentials protects the shared local Bot credential namespace
// for every runtime, without depending on Desktop's channel implementation.
package botcredentials

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"deepzora/internal/secrets"
)

var secretWatches struct {
	sync.Mutex
	refs map[string]int
}

type credential struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

func credentialValues(entries []credential) []secrets.Labeled {
	values := make([]secrets.Labeled, 0, len(entries))
	for _, entry := range entries {
		values = append(values, secrets.Labeled{Label: "BOT_" + entry.ID, Value: entry.Secret})
	}
	return values
}

// Install protects the entire namespace, including migration
// backups and partially written temporary files. Read committed snapshots on
// every masking boundary so another app process's edits are observed too.
func Install(dir string) func() {
	secrets.RegisterHostSecretPath(dir)
	secretWatches.Lock()
	defer secretWatches.Unlock()
	if secretWatches.refs == nil {
		secretWatches.refs = map[string]int{}
	}
	key := "bot-channels:" + dir
	if secretWatches.refs[dir] == 0 {
		secrets.WatchValuesChecked(key, func() ([]secrets.Labeled, error) {
			var values []secrets.Labeled
			for _, name := range []string{"connections.json", "connections.pre-members.json"} {
				entries, err := readCredentialSnapshot(filepath.Join(dir, name))
				if err != nil {
					return values, err
				}
				values = append(values, credentialValues(entries)...)
			}
			return values, nil
		})
	}
	secretWatches.refs[dir]++
	return sync.OnceFunc(func() {
		secretWatches.Lock()
		defer secretWatches.Unlock()
		secretWatches.refs[dir]--
		if secretWatches.refs[dir] == 0 {
			delete(secretWatches.refs, dir)
			secrets.UnwatchValues(key)
		}
	})
}

func readCredentialSnapshot(path string) ([]credential, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("cannot read bot credential snapshot")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	var entries []credential
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &entries) != nil {
		return nil, errors.New("invalid bot credential snapshot")
	}
	return entries, nil
}
