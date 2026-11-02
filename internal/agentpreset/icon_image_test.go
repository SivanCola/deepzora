package agentpreset

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func presetTestImage(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func TestIconImageStoredAndCopiedWithDefinition(t *testing.T) {
	dir := t.TempDir()
	p := Default()
	p.ID, p.Name, p.Icon, p.IconImage = "avatar", "Avatar", "search", presetTestImage(t, 128, 128)
	p.IconColor = "#60a5fa"
	if _, err := Write(dir, p.ID, Render(p)); err != nil {
		t.Fatal(err)
	}
	loaded, err := Resolve(Dirs{User: dir}, p.ID)
	if err != nil || loaded.IconImage != p.IconImage || loaded.Icon != p.Icon || loaded.IconColor != p.IconColor {
		t.Fatalf("image did not survive save/reload: %v", err)
	}
	copyDir := t.TempDir()
	if _, err := Write(copyDir, "avatar-copy", Render(loaded)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	copied, err := Resolve(Dirs{User: copyDir}, "avatar-copy")
	if err != nil || copied.IconImage != p.IconImage {
		t.Fatalf("copy depends on the original directory: %v", err)
	}
	copied.IconImage = ""
	if _, err := Write(copyDir, "avatar-copy", Render(copied)); err != nil {
		t.Fatal(err)
	}
	reset, err := Resolve(Dirs{User: copyDir}, "avatar-copy")
	if err != nil || reset.IconImage != "" || reset.Icon != "search" {
		t.Fatalf("reset did not retain glyph fallback: %v", err)
	}
}

func TestIconColorRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"red", "#123", "#1234567", "url(https://example.com)", "#12xx12"} {
		preset := Default()
		preset.IconColor = value
		if _, err := Parse("color", Render(preset)); err == nil {
			t.Fatalf("accepted icon color %q", value)
		}
	}
}

func TestIconImageRejectsInvalidDataBeforeWriting(t *testing.T) {
	valid := presetTestImage(t, 8, 8)
	for name, value := range map[string]string{
		"path": "/tmp/avatar.png", "remote": "https://example.com/avatar.png",
		"svg": "data:image/svg+xml;base64,PHN2Zy8+", "oversize": strings.Repeat("x", 128*1024+1),
		"base64": "data:image/png;base64,???", "not-png": "data:image/png;base64,aGVsbG8=",
		"wide": presetTestImage(t, 128, 64), "dimensions": presetTestImage(t, 129, 129),
		"corrupt": valid[:len(valid)-12], "newline": valid + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := Default()
			p.IconImage = value
			if _, err := Write(dir, "invalid", Render(p)); err == nil {
				t.Fatal("invalid image was accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, "invalid.md")); !os.IsNotExist(err) {
				t.Fatalf("invalid image left a preset file: %v", err)
			}
		})
	}
}
