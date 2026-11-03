package team

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func groupPostDigest(text string, mentions []string, everyone bool) string {
	value, _ := json.Marshal(struct {
		Text     string
		Mentions []string
		Everyone bool
	}{text, mentions, everyone})
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
