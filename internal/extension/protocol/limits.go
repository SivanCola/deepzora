package protocol

// Frozen wire limits. These constants are part of the protocol contract:
// they are frozen into the generated schema document and changing one changes
// the schema hash.
const (
	// FrameBytes caps one JSON-RPC frame on the extension transport.
	FrameBytes = 8 << 20
	// ExternalizeFieldBytes is the threshold above which an externalizable
	// payload must move into a content ref instead of traveling inline.
	ExternalizeFieldBytes = 64 << 10
	// ContentRefChunkBytes caps one host/content/read chunk.
	ContentRefChunkBytes = 256 << 10
	// ContentRefObjectBytes caps one externalized object.
	ContentRefObjectBytes = 8 << 20
)

// Limits is the JSON-serializable form of the frozen constants, embedded in
// the generated schema document.
type Limits struct {
	FrameBytes            int `json:"frameBytes"`
	ExternalizeFieldBytes int `json:"externalizeFieldBytes"`
	ContentRefChunkBytes  int `json:"contentRefChunkBytes"`
	ContentRefObjectBytes int `json:"contentRefObjectBytes"`
}

// FrozenLimits returns the immutable limit set for this protocol build.
func FrozenLimits() Limits {
	return Limits{
		FrameBytes:            FrameBytes,
		ExternalizeFieldBytes: ExternalizeFieldBytes,
		ContentRefChunkBytes:  ContentRefChunkBytes,
		ContentRefObjectBytes: ContentRefObjectBytes,
	}
}

// RequiresExternalization reports whether an inline externalizable payload
// exceeds the frozen threshold and must move into a content ref.
func RequiresExternalization(payloadBytes int) bool {
	return payloadBytes > ExternalizeFieldBytes
}
