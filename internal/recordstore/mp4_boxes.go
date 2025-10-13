package recordstore

import (
	amp4 "github.com/abema/go-mp4"
)

func boxTypeMtxi() amp4.BoxType { return amp4.StrToBoxType("mtxi") }

func init() { //nolint:gochecknoinits
	amp4.AddBoxDef(&Mtxi{}, 0)
}

// Mtxi is a MediaMTX segment info.
type Mtxi struct {
	amp4.FullBox  `mp4:"0,extend"`
	StreamID      [16]byte `mp4:"1,size=8"`
	SegmentNumber uint64   `mp4:"2,size=64"`
	DTS           int64    `mp4:"3,size=64"`
	NTP           int64    `mp4:"4,size=64"`
}

// GetType implements amp4.IBox.
func (*Mtxi) GetType() amp4.BoxType {
	return boxTypeMtxi()
}
