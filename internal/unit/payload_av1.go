package unit

// PayloadAV1 is the payload of an AV1 track.
type PayloadAV1 [][]byte

func (PayloadAV1) isPayload() {}
