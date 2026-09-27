package moq_test

import "github.com/bluenviron/mediamtx/internal/protocols/moq"

var _ moq.Conn = &moq.ConnWebTransport{}
