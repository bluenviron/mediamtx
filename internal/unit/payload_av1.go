package unit

// PayloadAV1 is the payload of an AV1 track.
// This must have at least 1 element, each with at least 1 byte.
type PayloadAV1 [][]byte

func (PayloadAV1) isPayload() {}
