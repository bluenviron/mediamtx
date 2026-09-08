package unit

// PayloadH264 is the payload of a H264 track.
// This must have at least 1 element, each with at least 1 byte.
type PayloadH264 [][]byte

func (PayloadH264) isPayload() {}
