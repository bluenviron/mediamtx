package unit

// PayloadMPEG1Audio is the payload of a MPEG-1/2 Audio track.
// This must have at least 1 element, each with at least 1 byte.
type PayloadMPEG1Audio [][]byte

func (PayloadMPEG1Audio) isPayload() {}
