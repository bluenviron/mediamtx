package hooks

import (
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
		env["MTX_QUERY"] = params.Query
	}

	if params.Conf.RunOnDemand != "" {
		params.Logger.Log(logger.Info, "runOnDemand command started")

		onDemandCmd = externalcmd.NewCmd(
			params.ExternalCmdPool,
			params.Conf.RunOnDemand,
			params.Conf.RunOnDemandRestart,
			env,
			func(err error) {
				params.Logger.Log(logger.Info, "runOnDemand command exited: %v", err)
			})
	}

	return func(reason string) {
		if onDemandCmd != nil {
			onDemandCmd.Close()
			params.Logger.Log(logger.Info, "runOnDemand command stopped: %v", reason)
		}

		if params.Conf.RunOnUnDemand != "" {
			params.Logger.Log(logger.Info, "runOnUnDemand command launched")
			externalcmd.NewCmd(
				params.ExternalCmdPool,
				params.Conf.RunOnUnDemand,
				false,
				env,
				nil)
		}
	}
}
