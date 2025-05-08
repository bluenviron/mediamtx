package rtmp

import (
	"net"
	"sync"
)

type listener struct {
	ln     net.Listener
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
		conn, err := l.ln.Accept()
		if err != nil {
			return err
		}

		l.parent.newConn(conn)
	}
}
