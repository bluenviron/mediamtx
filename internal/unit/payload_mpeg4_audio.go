package unit

// PayloadMPEG4Audio is the payload of a MPEG-4 Audio track.
type PayloadMPEG4Audio [][]byte

func (PayloadMPEG4Audio) isPayload() {}
