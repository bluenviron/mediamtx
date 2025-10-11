package unit

// PayloadMPEG1Audio is the payload of a MPEG-1 Audio track.
type PayloadMPEG1Audio [][]byte

func (PayloadMPEG1Audio) isPayload() {}
