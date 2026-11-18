package hooks

import (
	"net/url"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnOnlineParams are the parameters of OnOnline.
type OnOnlineParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
	Desc            *defs.APIPathSource
	Query           string
}

// OnOnline is the OnOnline hook.
func OnOnline(params OnOnlineParams) func() {
	var env externalcmd.Environment
	var onOnlineCmd *externalcmd.Cmd

	if params.Conf.RunOnOnline != "" || params.Conf.RunOnOffline != "" {
		env = params.ExternalCmdEnv
		env["MTX_QUERY"] = url.QueryEscape(params.Query)
		if params.Desc != nil {
			env["MTX_SOURCE_TYPE"] = string(params.Desc.Type)
			env["MTX_SOURCE_ID"] = params.Desc.ID
		}
	}

	if params.Conf.RunOnOnline != "" {
		onOnlineCmd = &externalcmd.Cmd{
			Label:   "runOnOnline",
			Parent:  params.Logger,
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.Conf.RunOnOnline,
			Restart: params.Conf.RunOnOnlineRestart,
			Env:     env,
		}
		onOnlineCmd.Start()
	}

	return func() {
		if onOnlineCmd != nil {
			onlineCmd := onOnlineCmd
			onOnlineCmd = nil
			onlineCmd.Close("")
		}

		if params.Conf.RunOnOffline != "" {
			cmd := &externalcmd.Cmd{
				Label:   "runOnOffline",
				Parent:  params.Logger,
				Pool:    params.ExternalCmdPool,
				Cmdstr:  params.Conf.RunOnOffline,
				Restart: false,
				Env:     env,
			}
			cmd.Start()
		}
	}
}
