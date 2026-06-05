package unit

// PayloadFLAC is the payload of a FLAC track.
type PayloadFLAC []byte

func (PayloadFLAC) isPayload() {}
