package unit

// PayloadLPCM is the payload of a LPCM track.
type PayloadLPCM []byte

func (PayloadLPCM) isPayload() {}
