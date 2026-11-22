package rawconn_test

import "github.com/bluenviron/mediamtx/internal/protocols/moq/rawconn"

var _ rawconn.Conn = (*rawconn.WebTransport)(nil)
