package memory

import (
	"context"
	"deepzora/internal/fileutil"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const userReceiptsFile = ".team-receipts.json"

var ErrRequestReused = errors.New("memory request id reused with different content")

type userReceipt struct {
	Key, Digest string
	Result      Memory
}
type userReceipts struct {
	SchemaVersion int
	Receipts      []userReceipt
}

// ApplyUserChange commits the body and retry receipt in the same recoverable
// namespace transaction. A retry never creates a second fact or archives twice.
func (s Store) ApplyUserChange(ctx context.Context, key, digest string, change BatchChange) (Memory, error) {
	if key == "" || len(key) > 200 || digest == "" {
		return Memory{}, errors.New("memory request id and digest required")
	}
	var result Memory
	err := s.applyNamespaceTransaction(ctx, func(staged Store) error {
		receipts := userReceipts{SchemaVersion: 1}
		raw, err := os.ReadFile(filepath.Join(staged.Dir, userReceiptsFile))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil {
			if err := json.Unmarshal(raw, &receipts); err != nil {
				return err
			}
			if receipts.SchemaVersion != 1 {
				return errors.New("unknown memory receipt schema")
			}
		}
		for _, receipt := range receipts.Receipts {
			if receipt.Key == key {
				if receipt.Digest != digest {
					return ErrRequestReused
				}
				result = receipt.Result
				return nil
			}
		}
		result, err = staged.applyStagedChange(change, true)
		if err != nil {
			return err
		}
		receipts.Receipts = append(receipts.Receipts, userReceipt{Key: key, Digest: digest, Result: result})
		if len(receipts.Receipts) > 512 {
			receipts.Receipts = receipts.Receipts[len(receipts.Receipts)-512:]
		}
		raw, err = json.Marshal(receipts)
		if err != nil {
			return err
		}
		return fileutil.AtomicWriteFileStrict(filepath.Join(staged.Dir, userReceiptsFile), raw, 0600)
	})
	return result, err
}
