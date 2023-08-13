package core

import (
	"sync"

	"github.com/datarhei/gosrt"
)

type srtListener struct {
	ln     srt.Listener
	wg     *sync.WaitGroup
	parent *srtServer
}

func newSRTListener(
	ln srt.Listener,
	wg *sync.WaitGroup,
	parent *srtServer,
) *srtListener {
	l := &srtListener{
		ln:     ln,
		wg:     wg,
		parent: parent,
	}

	l.wg.Add(1)
	go l.run()

	return l
}

func (l *srtListener) run() {
	defer l.wg.Done()

	err := l.runInner()

	l.parent.acceptError(err)
}

func (l *srtListener) runInner() error {
	for {
		var sconn *srtConn
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
