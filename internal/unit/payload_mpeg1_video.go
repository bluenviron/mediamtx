package unit

// PayloadMPEG1Video is the payload of a MPEG-1/2 Video track.
// This must have at least 1 byte.
type PayloadMPEG1Video []byte

func (PayloadMPEG1Video) isPayload() {}
