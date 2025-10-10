package unit

// PayloadG711 is the payload of a G711 track.
type PayloadG711 []byte

func (PayloadG711) isPayload() {}
