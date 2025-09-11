package rtmp

import (
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

// Conn is implemented by Client and ServerConn.
type Conn interface {
	BytesReceived() uint64
	BytesSent() uint64
	Read() (message.Message, error)
	Write(msg message.Message) error
}
