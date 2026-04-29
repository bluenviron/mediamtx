package hooks

import (
	"net/url"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// OnReadParams are the parameters of OnRead.
type OnReadParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
	Reader          defs.APIPathReader
	Query           string
}

// OnRead is the OnRead hook.
func OnRead(params OnReadParams) func() {
	var env externalcmd.Environment
	var onReadCmd *externalcmd.Cmd

	if params.Conf.RunOnRead != "" || params.Conf.RunOnUnread != "" {
		env = params.ExternalCmdEnv
		desc := params.Reader
		env["MTX_QUERY"] = url.QueryEscape(params.Query)
		env["MTX_READER_TYPE"] = string(desc.Type)
		env["MTX_READER_ID"] = desc.ID
	}

	if params.Conf.RunOnRead != "" {
		params.Logger.Log(logger.Info, "runOnRead command started")
		onReadCmd = &externalcmd.Cmd{
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.Conf.RunOnRead,
			Restart: params.Conf.RunOnReadRestart,
			Env:     env,
			OnExit: func(err error) {
				params.Logger.Log(logger.Info, "runOnRead command exited: %v", err)
			},
		}
		onReadCmd.Start()
	}

	return func() {
		if onReadCmd != nil {
			onReadCmd.Close()
			params.Logger.Log(logger.Info, "runOnRead command stopped")
		}

		if params.Conf.RunOnUnread != "" {
			params.Logger.Log(logger.Info, "runOnUnread command launched")
			cmd := &externalcmd.Cmd{
				Pool:    params.ExternalCmdPool,
				Cmdstr:  params.Conf.RunOnUnread,
				Restart: false,
				Env:     env,
			}
			cmd.Start()
		}
	}
}
