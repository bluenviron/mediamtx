package unit

// PayloadOpus is the payload of a Opus track.
// This must have at least 1 element, each with at least 1 byte.
type PayloadOpus [][]byte

func (PayloadOpus) isPayload() {}
