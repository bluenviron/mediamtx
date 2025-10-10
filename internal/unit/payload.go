package unit

// Payload is a codec-dependent payload.
type Payload interface {
	isPayload()
}
