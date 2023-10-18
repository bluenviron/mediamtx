package core

import (
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// publisher is an entity that can publish a stream.
type publisher interface {
	source
	close()
}

func publisherOnDemandHook(path *path, query string) func(string) {
	var onDemandCmd *externalcmd.Cmd

	if path.conf.RunOnDemand != "" {
		env := path.externalCmdEnv()
		env["MTX_QUERY"] = query

		path.Log(logger.Info, "runOnDemand command started")

		onDemandCmd = externalcmd.NewCmd(
			path.externalCmdPool,
			path.conf.RunOnDemand,
			path.conf.RunOnDemandRestart,
			env,
			func(err error) {
				path.Log(logger.Info, "runOnDemand command exited: %v", err)
			})
	}

	return func(reason string) {
		if onDemandCmd != nil {
			onDemandCmd.Close()
			path.Log(logger.Info, "runOnDemand command stopped: %v", reason)
		}
	}
}
