package unit

// PayloadH265 is the payload of a H265 track.
// This must have at least 1 element, each with at least 1 byte.
type PayloadH265 [][]byte

func (PayloadH265) isPayload() {}
