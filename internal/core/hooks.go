package core

import (
	"net"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func onInitHook(path *path) func() {
	var onInitCmd *externalcmd.Cmd

	if path.conf.RunOnInit != "" {
		path.Log(logger.Info, "runOnInit command started")
		onInitCmd = externalcmd.NewCmd(
			path.externalCmdPool,
			path.conf.RunOnInit,
			path.conf.RunOnInitRestart,
			path.externalCmdEnv(),
			func(err error) {
				path.Log(logger.Info, "runOnInit command exited: %v", err)
			})
	}

	return func() {
		if onInitCmd != nil {
			onInitCmd.Close()
			path.Log(logger.Info, "runOnInit command stopped")
		}
	}
}

func onConnectHook(c *conn, desc defs.APIPathSourceOrReader) func() {
	var env externalcmd.Environment
	var onConnectCmd *externalcmd.Cmd

	if c.runOnConnect != "" || c.runOnDisconnect != "" {
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		env = externalcmd.Environment{
			"RTSP_PORT":     port,
			"MTX_CONN_TYPE": desc.Type,
			"MTX_CONN_ID":   desc.ID,
		}
	}

	if c.runOnConnect != "" {
		c.logger.Log(logger.Info, "runOnConnect command started")

		onConnectCmd = externalcmd.NewCmd(
			c.externalCmdPool,
			c.runOnConnect,
			c.runOnConnectRestart,
			env,
			func(err error) {
				c.logger.Log(logger.Info, "runOnConnect command exited: %v", err)
			})
	}

	return func() {
		if onConnectCmd != nil {
			onConnectCmd.Close()
			c.logger.Log(logger.Info, "runOnConnect command stopped")
		}

		if c.runOnDisconnect != "" {
			c.logger.Log(logger.Info, "runOnDisconnect command launched")
			externalcmd.NewCmd(
				c.externalCmdPool,
				c.runOnDisconnect,
				false,
				env,
				nil)
		}
	}
}

func onDemandHook(path *path, query string) func(string) {
	var env externalcmd.Environment
	var onDemandCmd *externalcmd.Cmd

	if path.conf.RunOnDemand != "" || path.conf.RunOnUnDemand != "" {
		env = path.externalCmdEnv()
		env["MTX_QUERY"] = query
	}

	if path.conf.RunOnDemand != "" {
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

		if path.conf.RunOnUnDemand != "" {
			path.Log(logger.Info, "runOnUnDemand command launched")
			externalcmd.NewCmd(
				path.externalCmdPool,
				path.conf.RunOnUnDemand,
				false,
				env,
				nil)
		}
	}
}

func onReadyHook(path *path) func() {
	var env externalcmd.Environment
	var onReadyCmd *externalcmd.Cmd

	if path.conf.RunOnReady != "" || path.conf.RunOnNotReady != "" {
		env = path.externalCmdEnv()
		desc := path.source.APISourceDescribe()
		env["MTX_QUERY"] = path.publisherQuery
		env["MTX_SOURCE_TYPE"] = desc.Type
		env["MTX_SOURCE_ID"] = desc.ID
	}

	if path.conf.RunOnReady != "" {
		path.Log(logger.Info, "runOnReady command started")
		onReadyCmd = externalcmd.NewCmd(
			path.externalCmdPool,
			path.conf.RunOnReady,
			path.conf.RunOnReadyRestart,
			env,
			func(err error) {
				path.Log(logger.Info, "runOnReady command exited: %v", err)
			})
	}

	return func() {
		if onReadyCmd != nil {
			onReadyCmd.Close()
			path.Log(logger.Info, "runOnReady command stopped")
		}

		if path.conf.RunOnNotReady != "" {
			path.Log(logger.Info, "runOnNotReady command launched")
			externalcmd.NewCmd(
				path.externalCmdPool,
				path.conf.RunOnNotReady,
				false,
				env,
				nil)
		}
	}
}

func onReadHook(
	externalCmdPool *externalcmd.Pool,
	pathConf *conf.Path,
	path *path,
	reader defs.APIPathSourceOrReader,
	query string,
	l logger.Writer,
) func() {
	var env externalcmd.Environment
	var onReadCmd *externalcmd.Cmd

	if pathConf.RunOnRead != "" || pathConf.RunOnUnread != "" {
		env = path.externalCmdEnv()
		desc := reader
		env["MTX_QUERY"] = query
		env["MTX_READER_TYPE"] = desc.Type
		env["MTX_READER_ID"] = desc.ID
	}

	if pathConf.RunOnRead != "" {
		l.Log(logger.Info, "runOnRead command started")
		onReadCmd = externalcmd.NewCmd(
			externalCmdPool,
			pathConf.RunOnRead,
			pathConf.RunOnReadRestart,
			env,
			func(err error) {
				l.Log(logger.Info, "runOnRead command exited: %v", err)
			})
	}

	return func() {
		if onReadCmd != nil {
			onReadCmd.Close()
			l.Log(logger.Info, "runOnRead command stopped")
		}

		if pathConf.RunOnUnread != "" {
			l.Log(logger.Info, "runOnUnread command launched")
			externalcmd.NewCmd(
				externalCmdPool,
				pathConf.RunOnUnread,
				false,
				env,
				nil)
		}
	}
}
