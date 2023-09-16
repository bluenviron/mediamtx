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

func (c *conn) open(desc apiPathSourceOrReader) {
	if c.runOnConnect != "" {
		c.logger.Log(logger.Info, "runOnConnect command started")

		_, port, _ := net.SplitHostPort(c.rtspAddress)
		env := externalcmd.Environment{
			"RTSP_PORT":     port,
			"MTX_CONN_TYPE": desc.Type,
			"MTX_CONN_ID":   desc.ID,
		}

		c.onConnectCmd = externalcmd.NewCmd(
			c.externalCmdPool,
			c.runOnConnect,
			c.runOnConnectRestart,
			env,
			func(err error) {
				c.logger.Log(logger.Info, "runOnConnect command exited: %v", err)
			})
	}
}

func (c *conn) close(desc apiPathSourceOrReader) {
	if c.onConnectCmd != nil {
		c.onConnectCmd.Close()
		c.logger.Log(logger.Info, "runOnConnect command stopped")
	}

	if c.runOnDisconnect != "" {
		c.logger.Log(logger.Info, "runOnDisconnect command launched")

		_, port, _ := net.SplitHostPort(c.rtspAddress)
		env := externalcmd.Environment{
			"RTSP_PORT":     port,
			"MTX_CONN_TYPE": desc.Type,
			"MTX_CONN_ID":   desc.ID,
		}

		externalcmd.NewCmd(
			c.externalCmdPool,
			c.runOnDisconnect,
			false,
			env,
			nil)
	}
}
