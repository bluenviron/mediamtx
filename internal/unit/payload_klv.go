package unit

// PayloadKLV is the payload of a KLV track.
type PayloadKLV []byte

func (PayloadKLV) isPayload() {}
