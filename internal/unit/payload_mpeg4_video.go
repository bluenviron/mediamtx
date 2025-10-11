package unit

// PayloadMPEG4Video is the payload of a MPEG-4 Video track.
type PayloadMPEG4Video []byte

func (PayloadMPEG4Video) isPayload() {}
