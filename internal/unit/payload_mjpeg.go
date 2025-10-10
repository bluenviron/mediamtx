package unit

// PayloadMJPEG is the payload of a MJPEG track.
type PayloadMJPEG []byte

func (PayloadMJPEG) isPayload() {}
