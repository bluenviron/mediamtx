package unit

// PayloadLPCM is the payload of a LPCM track.
// This must have at least 1 byte.
type PayloadLPCM []byte

func (PayloadLPCM) isPayload() {}
