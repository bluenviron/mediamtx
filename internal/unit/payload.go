package unit

// Payload is a codec-dependent payload.
// It has some requirements that are described in specific payloads.
type Payload interface {
	isPayload()
}
