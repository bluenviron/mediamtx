package hooks

import (
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnReadyParams are the parameters of OnReady.
type OnReadyParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
	Desc            defs.APIPathSourceOrReader
	Query           string
}

// OnReady is the OnReady hook.
func OnReady(params OnReadyParams) func() {
	var env externalcmd.Environment
	var onReadyCmd *externalcmd.Cmd

	if params.Conf.RunOnReady != "" || params.Conf.RunOnNotReady != "" {
		env = params.ExternalCmdEnv
		env["MTX_QUERY"] = params.Query
		env["MTX_SOURCE_TYPE"] = params.Desc.Type
		env["MTX_SOURCE_ID"] = params.Desc.ID
	}

	if params.Conf.RunOnReady != "" {
		params.Logger.Log(logger.Info, "runOnReady command started")
		onReadyCmd = externalcmd.NewCmd(
			params.ExternalCmdPool,
			params.Conf.RunOnReady,
			params.Conf.RunOnReadyRestart,
			env,
			func(err error) {
				params.Logger.Log(logger.Info, "runOnReady command exited: %v", err)
			})
	}

	return func() {
		if onReadyCmd != nil {
			onReadyCmd.Close()
			params.Logger.Log(logger.Info, "runOnReady command stopped")
		}

		if params.Conf.RunOnNotReady != "" {
			params.Logger.Log(logger.Info, "runOnNotReady command launched")
			externalcmd.NewCmd(
				params.ExternalCmdPool,
				params.Conf.RunOnNotReady,
				false,
				env,
				nil)
		}
	}
}
