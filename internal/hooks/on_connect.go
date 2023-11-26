package hooks

import (
	"net"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnConnectParams are the parameters of OnConnect.
type OnConnectParams struct {
	Logger              logger.Writer
	ExternalCmdPool     *externalcmd.Pool
	RunOnConnect        string
	RunOnConnectRestart bool
	RunOnDisconnect     string
	RTSPAddress         string
	Desc                defs.APIPathSourceOrReader
}

// OnConnect is the OnConnect hook.
func OnConnect(params OnConnectParams) func() {
	var env externalcmd.Environment
	var onConnectCmd *externalcmd.Cmd

	if params.RunOnConnect != "" || params.RunOnDisconnect != "" {
		_, port, _ := net.SplitHostPort(params.RTSPAddress)
		env = externalcmd.Environment{
			"RTSP_PORT":     port,
			"MTX_CONN_TYPE": params.Desc.Type,
			"MTX_CONN_ID":   params.Desc.ID,
		}
	}

	if params.RunOnConnect != "" {
		params.Logger.Log(logger.Info, "runOnConnect command started")

		onConnectCmd = externalcmd.NewCmd(
			params.ExternalCmdPool,
			params.RunOnConnect,
			params.RunOnConnectRestart,
			env,
			func(err error) {
				params.Logger.Log(logger.Info, "runOnConnect command exited: %v", err)
			})
	}

	return func() {
		if onConnectCmd != nil {
			onConnectCmd.Close()
			params.Logger.Log(logger.Info, "runOnConnect command stopped")
		}

		if params.RunOnDisconnect != "" {
			params.Logger.Log(logger.Info, "runOnDisconnect command launched")
			externalcmd.NewCmd(
				params.ExternalCmdPool,
				params.RunOnDisconnect,
				false,
				env,
				nil)
		}
	}
}
