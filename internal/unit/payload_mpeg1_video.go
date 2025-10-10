package unit

// PayloadMPEG1Video is the payload of a MPEG-1 Video track.
type PayloadMPEG1Video []byte

func (PayloadMPEG1Video) isPayload() {}
