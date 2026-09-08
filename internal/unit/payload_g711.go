package unit

// PayloadG711 is the payload of a G711 track.
// This must have at least 1 byte.
type PayloadG711 []byte

func (PayloadG711) isPayload() {}
