package botchannels

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

func credentialValues(entries []Saved) []secrets.Labeled {
	values := make([]secrets.Labeled, 0, len(entries))
	for _, entry := range entries {
		values = append(values, secrets.Labeled{Label: "BOT_" + entry.ID, Value: entry.Secret})
	}
	return values
}

// InstallSecretValues protects the entire namespace, including migration
// backups and partially written temporary files. Read committed snapshots on
// every masking boundary so another app process's edits are observed too.
func (s Store) InstallSecretValues() func() {
	secrets.RegisterHostSecretPath(s.Dir)
	secretWatches.Lock()
	defer secretWatches.Unlock()
	if secretWatches.refs == nil {
		secretWatches.refs = map[string]int{}
	}
	key := "bot-channels:" + s.Dir
	if secretWatches.refs[s.Dir] == 0 {
		secrets.WatchValuesChecked(key, func() ([]secrets.Labeled, error) {
			var values []secrets.Labeled
			for _, name := range []string{"connections.json", "connections.pre-members.json"} {
				entries, err := readCredentialSnapshot(filepath.Join(s.Dir, name))
				if err != nil {
					return values, err
				}
				values = append(values, credentialValues(entries)...)
			}
			return values, nil
		})
	}
	secretWatches.refs[s.Dir]++
	return sync.OnceFunc(func() {
		secretWatches.Lock()
		defer secretWatches.Unlock()
		secretWatches.refs[s.Dir]--
		if secretWatches.refs[s.Dir] == 0 {
			delete(secretWatches.refs, s.Dir)
			secrets.UnwatchValues(key)
		}
	})
}

func readCredentialSnapshot(path string) ([]Saved, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("cannot read bot credential snapshot")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	var entries []Saved
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &entries) != nil {
		return nil, errors.New("invalid bot credential snapshot")
	}
	return entries, nil
}
