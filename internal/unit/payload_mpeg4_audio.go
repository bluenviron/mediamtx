package unit

// PayloadMPEG4Audio is the payload of a MPEG-4 Audio track.
// This must have at least 1 element, each with at least 1 byte.
type PayloadMPEG4Audio [][]byte

func (PayloadMPEG4Audio) isPayload() {}
