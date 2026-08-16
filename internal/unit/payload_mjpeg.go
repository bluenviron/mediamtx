package unit

// PayloadMJPEG is the payload of a MJPEG track.
// This must have a valid JPEG image in it, that must also satisfy the RTP/M-JPEG constraints
// (width and height must be less than 2040 and must be a multiple of 8).
type PayloadMJPEG []byte

func (PayloadMJPEG) isPayload() {}
