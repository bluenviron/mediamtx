package unit

// PayloadVP9 is the payload of a VP9 track.
// This must have at least 1 byte.
type PayloadVP9 []byte

func (PayloadVP9) isPayload() {}
