package unit

// PayloadH265 is the payload of a H265 track.
type PayloadH265 [][]byte

func (PayloadH265) isPayload() {}
