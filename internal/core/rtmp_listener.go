package core

import (
	"net"
	"sync"
)

type rtmpListener struct {
	ln     net.Listener
	wg     *sync.WaitGroup
	parent *rtmpServer
}

func newRTMPListener(
	ln net.Listener,
	wg *sync.WaitGroup,
	parent *rtmpServer,
) *rtmpListener {
	l := &rtmpListener{
		ln:     ln,
		wg:     wg,
		parent: parent,
	}

	l.wg.Add(1)
	go l.run()

	return l
}

func (l *rtmpListener) run() {
	defer l.wg.Done()

	err := l.runInner()

	l.parent.acceptError(err)
}

func (l *rtmpListener) runInner() error {
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			return err
		}

		l.parent.newConn(conn)
	}
}
