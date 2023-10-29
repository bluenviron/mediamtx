package core

import (
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/description"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// source is an entity that can provide a stream.
// it can be:
// - publisher
// - staticSourceHandler
// - redirectSource
type source interface {
	logger.Writer
	APISourceDescribe() defs.APIPathSourceOrReader
}

func mediaDescription(media *description.Media) string {
	ret := make([]string, len(media.Formats))
	for i, forma := range media.Formats {
		ret[i] = forma.Codec()
	}
	return strings.Join(ret, "/")
}

func mediasDescription(medias []*description.Media) []string {
	ret := make([]string, len(medias))
	for i, media := range medias {
		ret[i] = mediaDescription(media)
	}
	return ret
}

func mediaInfo(medias []*description.Media) string {
	return fmt.Sprintf("%d %s (%s)",
		len(medias),
		func() string {
			if len(medias) == 1 {
				return "track"
			}
			return "tracks"
		}(),
		strings.Join(mediasDescription(medias), ", "))
}

func sourceOnReadyHook(path *path) func() {
	var env externalcmd.Environment
	var onReadyCmd *externalcmd.Cmd

	if path.conf.RunOnReady != "" {
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
