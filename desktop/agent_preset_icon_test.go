package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"deepzora/internal/agentpreset"
)

func TestAgentPresetCustomIconAuthoring(t *testing.T) {
	a := newManualSessionTestApp(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 128, 128))); err != nil {
		t.Fatal(err)
	}
	preset := agentpreset.Default()
	preset.IconImage = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
	saved, err := a.SaveAgentPreset("", "user", "avatar", agentpreset.Render(preset))
	if err != nil || saved.IconImage != preset.IconImage {
		t.Fatalf("save did not return the custom image: %v", err)
	}
	list, err := a.ListAgentPresets("")
	if err != nil || len(list.Presets) != 2 || list.Presets[1].IconImage != preset.IconImage {
		t.Fatalf("list did not return the persisted custom image: %v", err)
	}
	if _, err := a.SaveAgentPreset("", "user", "avatar-copy", agentpreset.Render(saved)); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteAgentPreset("", "user", "avatar"); err != nil {
		t.Fatal(err)
	}
	list, err = a.ListAgentPresets("")
	if err != nil || len(list.Presets) != 2 || list.Presets[1].IconImage != preset.IconImage {
		t.Fatalf("copy lost its image when the original was deleted: %v", err)
	}
}
