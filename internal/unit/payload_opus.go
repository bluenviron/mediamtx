package unit

// PayloadOpus is the payload of a Opus track.
type PayloadOpus [][]byte

func (PayloadOpus) isPayload() {}
