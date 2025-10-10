package unit

// PayloadVP9 is the payload of a VP9 track.
type PayloadVP9 []byte

func (PayloadVP9) isPayload() {}
