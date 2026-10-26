package attachment

import (
	"encoding/base64"
)

func DataURL(mime string, raw []byte) string {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}
