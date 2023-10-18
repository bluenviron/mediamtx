package core

import (
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// reader is an entity that can read a stream.
type reader interface {
	close()
	apiReaderDescribe() apiPathSourceOrReader
}

func readerMediaInfo(r *asyncwriter.Writer, stream *stream.Stream) string {
	return mediaInfo(stream.MediasForReader(r))
}

func readerOnReadHook(
	externalCmdPool *externalcmd.Pool,
	pathConf *conf.Path,
	path *path,
	reader apiPathSourceOrReader,
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
