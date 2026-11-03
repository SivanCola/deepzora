package team

import (
	"deepzora/internal/secrets"
	"deepzora/internal/teamroster"
	"errors"
	"sync"
)

var secretWatches struct {
	sync.Mutex
	refs map[string]int
}

func InstallSecretValues(store teamroster.Store, slots SecretSlots) func() {
	secretWatches.Lock()
	defer secretWatches.Unlock()
	if secretWatches.refs == nil {
		secretWatches.refs = map[string]int{}
	}
	if secretWatches.refs[store.Dir] == 0 {
		WatchSecretValues(store, slots)
	}
	secretWatches.refs[store.Dir]++
	return func() {
		secretWatches.Lock()
		defer secretWatches.Unlock()
		secretWatches.refs[store.Dir]--
		if secretWatches.refs[store.Dir] == 0 {
			delete(secretWatches.refs, store.Dir)
			secrets.UnwatchValues("team:" + store.Dir)
		}
	}
}

// WatchSecretValues reads content on each snapshot, never trusting mtime or
// size. Both live and retired slots participate after restart. Only metadata
// grants may authorize a command; this source supplies masking, not authority.
func WatchSecretValues(store teamroster.Store, slots SecretSlots) {
	secrets.WatchValuesChecked("team:"+store.Dir, func() ([]secrets.Labeled, error) {
		book, err := store.TeamSecrets()
		if err != nil {
			return nil, err
		}
		values := make([]secrets.Labeled, 0, len(book.Secrets)+len(book.Retired))
		var failures []error
		add := func(name, slot string) {
			value, err := slots.Read(slot)
			if err != nil {
				failures = append(failures, err)
				return
			}
			values = append(values, secrets.Labeled{Label: name, Value: value})
		}
		for _, sc := range book.Secrets {
			add(sc.Name, sc.Slot)
		}
		for _, sc := range book.Retired {
			add(sc.Name, sc.Slot)
		}
		return values, errors.Join(failures...)
	})
}
