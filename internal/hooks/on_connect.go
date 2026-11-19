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
	Desc                defs.APIPathReader
}

// OnConnect is the OnConnect hook.
func OnConnect(params OnConnectParams) func() {
	var env externalcmd.Environment
	var onConnectCmd *externalcmd.Cmd

	if params.RunOnConnect != "" || params.RunOnDisconnect != "" {
		_, port, _ := net.SplitHostPort(params.RTSPAddress)
		env = externalcmd.Environment{
			"RTSP_PORT":     port,
			"MTX_CONN_TYPE": string(params.Desc.Type),
			"MTX_CONN_ID":   params.Desc.ID,
		}
	}

	if params.RunOnConnect != "" {
		onConnectCmd = &externalcmd.Cmd{
			Label:   "runOnConnect",
			Parent:  params.Logger,
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.RunOnConnect,
			Restart: params.RunOnConnectRestart,
			Env:     env,
		}
		onConnectCmd.Start()
	}

	return func() {
		if onConnectCmd != nil {
			onConnectCmd.Close("")
		}

		if params.RunOnDisconnect != "" {
			cmd := &externalcmd.Cmd{
				Label:   "runOnDisconnect",
				Parent:  params.Logger,
				Pool:    params.ExternalCmdPool,
				Cmdstr:  params.RunOnDisconnect,
				Restart: false,
				Env:     env,
			}
			cmd.Start()
		}
	}
}
