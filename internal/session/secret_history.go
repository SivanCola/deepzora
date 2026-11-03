package session

import (
	"bytes"
	"context"
	"deepzora/internal/secrets"
	"deepzora/internal/sessioncontent"
	"errors"
	"io"
	"path/filepath"
	"sync"
)

const secretHistoryProjectionBudget = 64 << 20

type secretHistoryKey struct {
	root, digest, index string
	bytes               int64
	mask                *secrets.Mask
}

var secretHistoryCache struct {
	sync.Mutex
	items map[secretHistoryKey]sessioncontent.Ref
}

func projectHistoryContent(ctx context.Context, root string, ref sessioncontent.Ref) (sessioncontent.Ref, error) {
	mask := secrets.Masker()
	if mask == nil {
		return ref, nil
	}
	key := secretHistoryKey{root: root, digest: ref.Digest, index: ref.IndexDigest, bytes: ref.Bytes, mask: mask}
	secretHistoryCache.Lock()
	cached, ok := secretHistoryCache.items[key]
	secretHistoryCache.Unlock()
	if ok {
		return cached, nil
	}
	store := contentStoreForSessionDir(root)
	file, err := store.Open(ctx, ref)
	if err != nil {
		return ref, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, secretHistoryProjectionBudget+1))
	if err != nil {
		return ref, err
	}
	if len(body) > secretHistoryProjectionBudget {
		return ref, errors.New("sensitive history body exceeds the projection budget; no unmasked bytes were returned")
	}
	masked := mask.JSONStrings(body)
	projected := ref
	if !bytes.Equal(body, masked) {
		projected, err = store.Put(ctx, bytes.NewReader(masked), sessioncontent.Metadata{MediaType: "application/json"})
		if err != nil {
			return ref, err
		}
	}
	secretHistoryCache.Lock()
	if len(secretHistoryCache.items) >= 256 {
		secretHistoryCache.items = nil
	}
	if secretHistoryCache.items == nil {
		secretHistoryCache.items = map[secretHistoryKey]sessioncontent.Ref{}
	}
	secretHistoryCache.items[key] = projected
	secretHistoryCache.Unlock()
	return projected, nil
}
func (q *Query) checkSecretContentGrant(ctx context.Context, filesystem *FilesystemPersistence, ref SessionRef, contentRef sessioncontent.Ref) error {
	projected, err := projectHistoryContent(ctx, filepath.Join(filesystem.Root, ref.SessionID), contentRef)
	if err != nil {
		return err
	}
	if projected != contentRef {
		return errors.New("stale_secret_projection: reload the history page before reading its content")
	}
	return nil
}
