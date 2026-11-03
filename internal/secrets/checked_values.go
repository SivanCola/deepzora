package secrets

import (
	"errors"
	"sync"
)

var checkedValues struct {
	sync.Mutex
	failures map[string]bool
}

// WatchValuesChecked seals new secret-bearing executions and model requests
// when the authoritative namespace cannot be read. Previously held values
// remain masked for in-flight work and retained history.
func WatchValuesChecked(key string, read func() ([]Labeled, error)) {
	WatchValues(key, func() []Labeled {
		values, err := read()
		checkedValues.Lock()
		if checkedValues.failures == nil {
			checkedValues.failures = map[string]bool{}
		}
		checkedValues.failures[key] = err != nil
		checkedValues.Unlock()
		return values
	})
}
func BoundaryError() error {
	checkedValues.Lock()
	defer checkedValues.Unlock()
	for _, failed := range checkedValues.failures {
		if failed {
			return errors.New("secret masking source unavailable; new outbound requests are blocked until storage is repaired")
		}
	}
	return nil
}

func UnwatchValues(key string) {
	registered.Lock()
	delete(registered.sources, key)
	registered.Unlock()
	checkedValues.Lock()
	delete(checkedValues.failures, key)
	checkedValues.Unlock()
}

// ForgetValues is only for an independently confirmed retirement action.
// The owner removes these values from its persisted source before calling it.
func ForgetValues(values []Labeled) {
	registered.Lock()
	defer registered.Unlock()
	for _, value := range values {
		delete(registered.held, value)
	}
	registered.snapshot = nil
	registered.epoch++
}
