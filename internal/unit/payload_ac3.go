package unit

// PayloadAC3 is the payload of an AC3 track.
// This must have at least 1 element, each with at least 1 byte.
type PayloadAC3 [][]byte

func (PayloadAC3) isPayload() {}
