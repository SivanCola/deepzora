package teamroster

import (
	"context"
	"errors"
)

// MutationReceipt is committed with a Desktop configuration change. Keeping
// a bounded retry window does not permit old retries to execute again: the
// original expected revision still fences requests older than that window.
type MutationReceipt struct {
	Key              string `json:"key"`
	Digest           string `json:"digest"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	Target           string `json:"target"`
}

var errMutationReplay = errors.New("team mutation replay")

func (s Store) Mutate(ctx context.Context, revision uint64, key, digest string, mutate func(*Roster) (string, error)) (Roster, string, error) {
	if len(key) < 8 || len(key) > 128 || len(digest) != 64 {
		return Roster{}, "", ErrInvalid
	}
	var target string
	var replay Roster
	r, err := s.Update(ctx, func(r *Roster) error {
		for _, receipt := range r.Mutations {
			if receipt.Key != key {
				continue
			}
			if receipt.Digest != digest || receipt.ExpectedRevision != revision {
				return ErrRequestReused
			}
			target, replay = receipt.Target, *r
			return errMutationReplay
		}
		if r.Revision != revision {
			return ErrConflict
		}
		var err error
		target, err = mutate(r)
		if err != nil {
			return err
		}
		r.Mutations = append(r.Mutations, MutationReceipt{Key: key, Digest: digest, ExpectedRevision: revision, Target: target})
		if len(r.Mutations) > 512 {
			r.Mutations = r.Mutations[len(r.Mutations)-512:]
		}
		return nil
	})
	if errors.Is(err, errMutationReplay) {
		return replay, target, nil
	}
	return r, target, err
}
