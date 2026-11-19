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
		onReadCmd = &externalcmd.Cmd{
			Label:   "runOnRead",
			Parent:  params.Logger,
			Pool:    params.ExternalCmdPool,
			Cmdstr:  params.Conf.RunOnRead,
			Restart: params.Conf.RunOnReadRestart,
			Env:     env,
		}
		onReadCmd.Start()
	}

	return func() {
		if onReadCmd != nil {
			onReadCmd.Close("")
		}

		if params.Conf.RunOnUnread != "" {
			cmd := &externalcmd.Cmd{
				Label:   "runOnUnread",
				Parent:  params.Logger,
				Pool:    params.ExternalCmdPool,
				Cmdstr:  params.Conf.RunOnUnread,
				Restart: false,
				Env:     env,
			}
			cmd.Start()
		}
	}
}
