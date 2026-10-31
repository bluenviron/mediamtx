package hooks

import (
	"maps"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
)

// withCloseReason returns env with MTX_SOURCE_CLOSE_REASON added, when the reason is known.
func withCloseReason(env externalcmd.Environment, closeReason defs.PublisherCloseReason) externalcmd.Environment {
	if closeReason == "" {
		return env
	}
	out := make(externalcmd.Environment, len(env)+1)
	maps.Copy(out, env)
	out["MTX_SOURCE_CLOSE_REASON"] = string(closeReason)
	return out
}
