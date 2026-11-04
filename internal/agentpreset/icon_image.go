package agentpreset

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"strings"
)

// Embedded images travel with the Markdown definition and never reference a
// user's original file or require a network fetch. Bound decoding as well as
// storage; only the small square PNG produced by the editor is accepted.
// ValidateIconImage applies the same portable PNG budget to copied Team icons.
func ValidateIconImage(value string) error { return validateIconImage(value) }

func validateIconImage(value string) error {
	if value == "" {
		return nil
	}
	const prefix = "data:image/png;base64,"
	if len(value) > 128*1024 || !strings.HasPrefix(value, prefix) || strings.ContainsAny(value, "\r\n") {
		return errors.New("icon-image: expected an embedded PNG data URL up to 128 KiB")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return errors.New("icon-image: invalid base64 data")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Width > 128 || config.Height != config.Width {
		return errors.New("icon-image: expected a square PNG up to 128 by 128 pixels")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return errors.New("icon-image: invalid PNG image")
	}
	return nil
}
