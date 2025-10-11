package unit

// PayloadMPEG4AudioLATM is the payload of a MPEG-4 Audio LATM track.
type PayloadMPEG4AudioLATM []byte

func (PayloadMPEG4AudioLATM) isPayload() {}
