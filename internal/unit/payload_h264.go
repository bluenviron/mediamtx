package unit

// PayloadH264 is the payload of a H264 track.
type PayloadH264 [][]byte

func (PayloadH264) isPayload() {}
