package core

import (
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// keepalive is a synthetic reader that keeps a stream alive
// without being an actual viewer.
type keepalive struct {
	id          uuid.UUID
	pathName    string
	created     time.Time
	creatorUser string // username that created this keepalive
	creatorIP   net.IP // IP address that created this keepalive
	onClose     func() // callback to remove from path when closed
}

func newKeepalive(pathName string, user string, ip net.IP) *keepalive {
	return &keepalive{
		id:          uuid.New(),
		pathName:    pathName,
		created:     time.Now(),
		creatorUser: user,
		creatorIP:   ip,
	}
}

// Close implements defs.Reader.
// This is called when the keepalive is explicitly closed via the API.
func (k *keepalive) Close() {
	if k.onClose != nil {
		k.onClose()
	}
}

// APIReaderDescribe implements defs.Reader.
func (k *keepalive) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "keepalive",
		ID:   k.id.String(),
	}
}

// Log implements logger.Writer.
func (k *keepalive) Log(level logger.Level, format string, args ...interface{}) {
	// no-op, keepalives don't need logging
}

func (k *keepalive) apiDescribe() *defs.APIKeepalive {
	return &defs.APIKeepalive{
		ID:          k.id,
		Created:     k.created,
		Path:        k.pathName,
		CreatorUser: k.creatorUser,
		CreatorIP:   k.creatorIP.String(),
	}
}
