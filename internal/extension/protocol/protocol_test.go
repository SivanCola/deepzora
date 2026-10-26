package protocol

import (
	"errors"
	"testing"
)

func TestProtocolConstants(t *testing.T) {
	if ProtocolID != "deepzora.extension.v2" {
		t.Fatalf("ProtocolID = %q", ProtocolID)
	}
	if ProtocolMajor != 2 || ProtocolVersion != "2" {
		t.Fatalf("ProtocolMajor/ProtocolVersion = %d/%q", ProtocolMajor, ProtocolVersion)
	}
}

func TestCompareProtocolVersion(t *testing.T) {
	if err := CompareProtocolVersion(ProtocolID, "2"); err != nil {
		t.Fatalf("exact match rejected: %v", err)
	}
	tests := []struct {
		name    string
		id      string
		version string
		reason  ErrorReason
	}{
		{"wrong id", "deepzora.extension.v1", "2", ErrUnsupportedVersion},
		{"empty id", "", "2", ErrUnsupportedVersion},
		{"wrong major", ProtocolID, "1", ErrUnsupportedVersion},
		{"zero major", ProtocolID, "0", ErrUnsupportedVersion},
		{"non-numeric version", ProtocolID, "v2", ErrProtocolError},
		{"empty version", ProtocolID, "", ErrProtocolError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompareProtocolVersion(tt.id, tt.version)
			if err == nil {
				t.Fatal("expected an error")
			}
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error type = %T, want *ProtocolError", err)
			}
			if protocolErr.Reason != tt.reason {
				t.Fatalf("reason = %q, want %q", protocolErr.Reason, tt.reason)
			}
		})
	}
}

func TestFrozenLimitsSanity(t *testing.T) {
	if FrameBytes != 8<<20 {
		t.Fatalf("FrameBytes = %d, want %d", FrameBytes, 8<<20)
	}
	if ExternalizeFieldBytes != 64<<10 {
		t.Fatalf("ExternalizeFieldBytes = %d, want %d", ExternalizeFieldBytes, 64<<10)
	}
	if ContentRefChunkBytes != 256<<10 {
		t.Fatalf("ContentRefChunkBytes = %d, want %d", ContentRefChunkBytes, 256<<10)
	}
	if ContentRefObjectBytes != 8<<20 {
		t.Fatalf("ContentRefObjectBytes = %d, want %d", ContentRefObjectBytes, 8<<20)
	}
	if ExternalizeFieldBytes >= ContentRefChunkBytes || ContentRefChunkBytes >= ContentRefObjectBytes {
		t.Fatal("limits must be strictly ordered: field < chunk < object")
	}
	if ContentRefObjectBytes > FrameBytes {
		t.Fatal("a content object must fit inside one frame ceiling")
	}
	limits := FrozenLimits()
	if limits.FrameBytes != FrameBytes || limits.ExternalizeFieldBytes != ExternalizeFieldBytes ||
		limits.ContentRefChunkBytes != ContentRefChunkBytes || limits.ContentRefObjectBytes != ContentRefObjectBytes {
		t.Fatalf("FrozenLimits = %+v does not match the constants", limits)
	}
}

func TestRequiresExternalization(t *testing.T) {
	if RequiresExternalization(ExternalizeFieldBytes) {
		t.Fatal("payload at the threshold must stay inline")
	}
	if !RequiresExternalization(ExternalizeFieldBytes + 1) {
		t.Fatal("payload above the threshold must externalize")
	}
}
