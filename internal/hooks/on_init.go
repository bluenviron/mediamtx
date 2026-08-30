package hooks

import (
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnInitParams are the parameters of OnInit.
type OnInitParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
}

// OnInit is the OnInit hook.
func OnInit(params OnInitParams) func() {
	var onInitCmd *externalcmd.Cmd

	if params.Conf.RunOnInit != "" {
		params.Logger.Log(logger.Info, "runOnInit command started")
		onInitCmd = &externalcmd.Cmd{
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.Conf.RunOnInit,
			Restart: params.Conf.RunOnInitRestart,
			Env:     params.ExternalCmdEnv,
			OnExit: func(err error) {
				params.Logger.Log(logger.Info, "runOnInit command exited: %v", err)
			},
		}
		onInitCmd.Start()
	}

	return func() {
		if onInitCmd != nil {
			onInitCmd.Close()
			params.Logger.Log(logger.Info, "runOnInit command stopped")
		}
	}
}
