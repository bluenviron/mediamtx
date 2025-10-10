package unit

// PayloadAC3 is the payload of an AC3 track.
type PayloadAC3 [][]byte

func (PayloadAC3) isPayload() {}
