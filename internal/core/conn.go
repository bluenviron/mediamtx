package core

import (
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type conn struct {
	rtspAddress         string
	runOnConnect        string
	runOnConnectRestart bool
	runOnDisconnect     string
	externalCmdPool     *externalcmd.Pool
	logger              logger.Writer

	onDisconnectHook func()
}

func newConn(
	rtspAddress string,
	runOnConnect string,
	runOnConnectRestart bool,
	runOnDisconnect string,
	externalCmdPool *externalcmd.Pool,
	logger logger.Writer,
) *conn {
	return &conn{
		rtspAddress:         rtspAddress,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		runOnDisconnect:     runOnDisconnect,
		externalCmdPool:     externalCmdPool,
		logger:              logger,
	}
}

func (c *conn) open(desc defs.APIPathSourceOrReader) {
	c.onDisconnectHook = onConnectHook(c, desc)
}

func (c *conn) close() {
	c.onDisconnectHook()
}
