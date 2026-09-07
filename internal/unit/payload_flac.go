package unit

// PayloadFLAC is the payload of a FLAC track.
// This must have at least 1 byte.
type PayloadFLAC []byte

func (PayloadFLAC) isPayload() {}
