package unit

// PayloadVP8 is the payload of a VP8 track.
// This must have at least 1 byte.
type PayloadVP8 []byte

func (PayloadVP8) isPayload() {}
