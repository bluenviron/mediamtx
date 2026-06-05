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
		params.Logger.Log(logger.Info, "runOnDemand command started")

		onDemandCmd = &externalcmd.Cmd{
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.Conf.RunOnDemand,
			Restart: params.Conf.RunOnDemandRestart,
			Env:     env,
			OnExit: func(err error) {
				params.Logger.Log(logger.Info, "runOnDemand command exited: %v", err)
			},
		}
		onDemandCmd.Start()
	}

	return func(reason string) {
		if onDemandCmd != nil {
			onDemandCmd.Close()
			params.Logger.Log(logger.Info, "runOnDemand command stopped: %v", reason)
		}

		if params.Conf.RunOnUnDemand != "" {
			params.Logger.Log(logger.Info, "runOnUnDemand command launched")
			cmd := &externalcmd.Cmd{
				Pool:    params.ExternalCmdPool,
				Cmdstr:  params.Conf.RunOnUnDemand,
				Restart: false,
				Env:     env,
			}
			cmd.Start()
		}
	}
}
