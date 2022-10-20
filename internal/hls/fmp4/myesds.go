package fmp4

import (
	gomp4 "github.com/abema/go-mp4"
)

type myEsds struct {
	gomp4.FullBox `mp4:"0,extend"`
	Data          []byte `mp4:"1,size=8"`
}

func (*myEsds) GetType() gomp4.BoxType {
	return gomp4.StrToBoxType("esds")
}

func init() { //nolint:gochecknoinits
	gomp4.AddBoxDef(&myEsds{}, 0)
}
