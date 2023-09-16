package core

import (
	"net"

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

	onConnectCmd *externalcmd.Cmd
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

func (c *conn) open() {
	if c.runOnConnect != "" {
		c.logger.Log(logger.Info, "runOnConnect command started")

		_, port, _ := net.SplitHostPort(c.rtspAddress)
		c.onConnectCmd = externalcmd.NewCmd(
			c.externalCmdPool,
			c.runOnConnect,
			c.runOnConnectRestart,
			externalcmd.Environment{
				"MTX_PATH":  "",
				"RTSP_PATH": "", // deprecated
				"RTSP_PORT": port,
			},
			func(err error) {
				c.logger.Log(logger.Info, "runOnConnect command exited: %v", err)
			})
	}
}

func (c *conn) close() {
	if c.onConnectCmd != nil {
		c.onConnectCmd.Close()
		c.logger.Log(logger.Info, "runOnConnect command stopped")
	}

	if c.runOnDisconnect != "" {
		c.logger.Log(logger.Info, "runOnDisconnect command launched")
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		externalcmd.NewCmd(
			c.externalCmdPool,
			c.runOnDisconnect,
			false,
			externalcmd.Environment{
				"MTX_PATH":  "",
				"RTSP_PATH": "", // deprecated
				"RTSP_PORT": port,
			},
			nil)
	}
}
