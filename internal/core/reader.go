package core

import (
	"github.com/aler9/gortsplib"
)

// reader is an entity that can read a stream.
type reader interface {
	Close()
	OnReaderAccepted()
	OnReaderFrame(int, gortsplib.StreamType, []byte)
	OnReaderAPIDescribe() interface{}
}
