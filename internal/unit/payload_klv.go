package unit

// PayloadKLV is the payload of a KLV track.
// This must have at least 1 byte.
type PayloadKLV []byte

func (PayloadKLV) isPayload() {}
