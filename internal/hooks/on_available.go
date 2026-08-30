package hooks

import (
	"net/url"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnAvailableParams are the parameters of OnAvailable.
type OnAvailableParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
	Desc            *defs.APIPathSource
	Query           string
}

// OnAvailable is the OnAvailable hook.
func OnAvailable(params OnAvailableParams) func() {
	var env externalcmd.Environment
	var onAvailableCmd *externalcmd.Cmd

	if params.Conf.RunOnAvailable != "" || params.Conf.RunOnUnavailable != "" {
		env = params.ExternalCmdEnv
		env["MTX_QUERY"] = url.QueryEscape(params.Query)
		if params.Desc != nil {
			env["MTX_SOURCE_TYPE"] = string(params.Desc.Type)
			env["MTX_SOURCE_ID"] = params.Desc.ID
		}
	}

	if params.Conf.RunOnAvailable != "" {
		params.Logger.Log(logger.Info, "runOnAvailable command started")
		onAvailableCmd = &externalcmd.Cmd{
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.Conf.RunOnAvailable,
			Restart: params.Conf.RunOnAvailableRestart,
			Env:     env,
			OnExit: func(err error) {
				params.Logger.Log(logger.Info, "runOnAvailable command exited: %v", err)
			},
		}
		onAvailableCmd.Start()
	}

	return func() {
		if onAvailableCmd != nil {
			onAvailableCmd.Close()
			params.Logger.Log(logger.Info, "runOnAvailable command stopped")
		}

		if params.Conf.RunOnUnavailable != "" {
			params.Logger.Log(logger.Info, "runOnUnavailable command launched")
			cmd := &externalcmd.Cmd{
				Pool:    params.ExternalCmdPool,
				Cmdstr:  params.Conf.RunOnUnavailable,
				Restart: false,
				Env:     env,
			}
			cmd.Start()
		}
	}
}
