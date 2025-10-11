package unit

// PayloadVP8 is the payload of a VP8 track.
type PayloadVP8 []byte

func (PayloadVP8) isPayload() {}
