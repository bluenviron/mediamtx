package core

import (
	"github.com/aler9/gortsplib"
)

// reader is an entity that can read a stream.
type reader interface {
	close()
	onReaderAccepted()
	onReaderFrame(int, gortsplib.StreamType, []byte)
	onReaderAPIDescribe() interface{}
}
