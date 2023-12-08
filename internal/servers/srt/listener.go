package srt

import (
	"sync"

	srt "github.com/datarhei/gosrt"
)

type listener struct {
	ln     srt.Listener
	wg     *sync.WaitGroup
	parent *Server
}

func (l *listener) initialize() {
	l.wg.Add(1)
	go l.run()
}

func (l *listener) run() {
	defer l.wg.Done()

	err := l.runInner()

	l.parent.acceptError(err)
}

func (l *listener) runInner() error {
	for {
		var sconn *conn
		conn, _, err := l.ln.Accept(func(req srt.ConnRequest) srt.ConnType {
			sconn = l.parent.newConnRequest(req)
			if sconn == nil {
				return srt.REJECT
			}

			// currently it's the same to return SUBSCRIBE or PUBLISH
			return srt.SUBSCRIBE
		})
		if err != nil {
			return err
		}

		if conn == nil {
			continue
		}

		sconn.setConn(conn)
	}
}
