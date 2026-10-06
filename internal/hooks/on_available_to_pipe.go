package hooks

import (
	"bufio"
	"io"
	"net/url"
	"sync"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// OnAvailableToPipeParams are the parameters of OnAvailableToPipe.
type OnAvailableToPipeParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
	Desc            *defs.APIPathSource
	Query           string
	Stream          *stream.Stream
}

type onAvailableToPipeInstance struct {
	params OnAvailableToPipeParams
	env    externalcmd.Environment

	cmd       *externalcmd.Cmd
	reader    *stream.Reader
	pipeW     *io.PipeWriter
	mutex     sync.Mutex
	closed    bool
	stopMutex sync.Mutex
}

func (inst *onAvailableToPipeInstance) createStdin() (io.ReadCloser, error) {
	inst.mutex.Lock()
	defer inst.mutex.Unlock()

	if inst.closed {
		return nil, io.EOF
	}

	// Clean up previous reader/pipe if restarting
	if inst.reader != nil {
		inst.params.Stream.RemoveReader(inst.reader)
		inst.reader = nil
	}
	if inst.pipeW != nil {
		_ = inst.pipeW.Close()
		inst.pipeW = nil
	}

	pr, pw := io.Pipe()
	inst.pipeW = pw

	reader := &stream.Reader{
		Parent: inst.params.Logger,
	}
	bw := bufio.NewWriter(pw)

	err := mpegts.FromStream(inst.params.Stream.OrigDesc, reader, bw, nil, 0)
	if err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return nil, err
	}

	inst.reader = reader
	inst.params.Stream.AddReader(reader)

	go func() {
		// Wait for reader error or termination
		<-reader.Error()
		inst.mutex.Lock()
		if inst.pipeW == pw {
			_ = pw.Close()
		}
		inst.mutex.Unlock()
	}()

	return pr, nil
}

func (inst *onAvailableToPipeInstance) stop() {
	inst.stopMutex.Lock()
	defer inst.stopMutex.Unlock()

	inst.mutex.Lock()
	inst.closed = true
	if inst.pipeW != nil {
		_ = inst.pipeW.Close()
	}
	if inst.reader != nil {
		inst.params.Stream.RemoveReader(inst.reader)
		inst.reader = nil
	}
	inst.mutex.Unlock()

	if inst.cmd != nil {
		inst.cmd.Close()
		inst.params.Logger.Log(logger.Info, "runOnAvailableToPipe command stopped")
	}
}

// OnAvailableToPipe is the OnAvailableToPipe hook.
func OnAvailableToPipe(params OnAvailableToPipeParams) func() {
	if params.Conf.RunOnAvailableToPipe == "" {
		return func() {}
	}

	env := params.ExternalCmdEnv
	env["MTX_QUERY"] = url.QueryEscape(params.Query)
	if params.Desc != nil {
		env["MTX_SOURCE_TYPE"] = string(params.Desc.Type)
		env["MTX_SOURCE_ID"] = params.Desc.ID
	}

	inst := &onAvailableToPipeInstance{
		params: params,
		env:    env,
	}

	params.Logger.Log(logger.Info, "runOnAvailableToPipe command started")
	cmd := &externalcmd.Cmd{
		Pool:    params.ExternalCmdPool,
		Cmdstr:  params.Conf.RunOnAvailableToPipe,
		Restart: params.Conf.RunOnAvailableToPipeRestart,
		Env:     env,
		Stdin:   inst.createStdin,
		OnExit: func(err error) {
			params.Logger.Log(logger.Info, "runOnAvailableToPipe command exited: %v", err)
		},
	}
	inst.cmd = cmd
	cmd.Start()

	return inst.stop
}
