package hooks

import (
	"net/url"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnDemandParams are the parameters of OnDemand.
type OnDemandParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
	Query           string
}

// OnDemand is the OnDemand hook.
func OnDemand(params OnDemandParams) func(string) {
	var env externalcmd.Environment
	var onDemandCmd *externalcmd.Cmd

	if params.Conf.RunOnDemand != "" || params.Conf.RunOnUnDemand != "" {
		env = params.ExternalCmdEnv
		env["MTX_QUERY"] = url.QueryEscape(params.Query)
	}

	if params.Conf.RunOnDemand != "" {
		onDemandCmd = &externalcmd.Cmd{
			Label:   "runOnDemand",
			Parent:  params.Logger,
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.Conf.RunOnDemand,
			Restart: params.Conf.RunOnDemandRestart,
			Env:     env,
		}
		onDemandCmd.Start()
	}

	return func(reason string) {
		if onDemandCmd != nil {
			onDemandCmd.Close(reason)
		}

		if params.Conf.RunOnUnDemand != "" {
			cmd := &externalcmd.Cmd{
				Label:   "runOnUnDemand",
				Parent:  params.Logger,
				Pool:    params.ExternalCmdPool,
				Cmdstr:  params.Conf.RunOnUnDemand,
				Restart: false,
				Env:     env,
			}
			cmd.Start()
		}
	}
}
