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
		req, err := l.ln.Accept2()
		if err != nil {
			return err
		}

		l.parent.newConnRequest(req)
	}
}
