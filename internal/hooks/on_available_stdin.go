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

// OnAvailableStdinParams are the parameters of OnAvailableStdin.
type OnAvailableStdinParams struct {
	Logger          logger.Writer
	ExternalCmdPool *externalcmd.Pool
	Conf            *conf.Path
	ExternalCmdEnv  externalcmd.Environment
	Desc            *defs.APIPathSource
	Query           string
	Stream          *stream.Stream
}

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...any) {}

type onAvailableStdinInstance struct {
	params OnAvailableStdinParams
	env    externalcmd.Environment

	cmd       *externalcmd.Cmd
	reader    *stream.Reader
	pipeW     *io.PipeWriter
	mutex     sync.Mutex
	closed    bool
	stopMutex sync.Mutex
}

func (inst *onAvailableStdinInstance) createStdin() (io.ReadCloser, error) {
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

func (inst *onAvailableStdinInstance) stop() {
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
		inst.params.Logger.Log(logger.Info, "runOnAvailableStdin command stopped")
	}
}

// OnAvailableStdin is the OnAvailableStdin hook.
func OnAvailableStdin(params OnAvailableStdinParams) func() {
	if params.Conf.RunOnAvailableStdin == "" {
		return func() {}
	}

	env := params.ExternalCmdEnv
	env["MTX_QUERY"] = url.QueryEscape(params.Query)
	if params.Desc != nil {
		env["MTX_SOURCE_TYPE"] = string(params.Desc.Type)
		env["MTX_SOURCE_ID"] = params.Desc.ID
	}

	// check codec compatibility upfront, otherwise the command would be restarted forever.
	err := mpegts.FromStream(params.Stream.OrigDesc, &stream.Reader{Parent: nilLogger{}},
		bufio.NewWriter(io.Discard), nil, 0)
	if err != nil {
		params.Logger.Log(logger.Warn, "runOnAvailableStdin command not started: %v", err)
		return func() {}
	}

	inst := &onAvailableStdinInstance{
		params: params,
		env:    env,
	}

	params.Logger.Log(logger.Info, "runOnAvailableStdin command started")
	cmd := &externalcmd.Cmd{
		Pool:    params.ExternalCmdPool,
		Cmdstr:  params.Conf.RunOnAvailableStdin,
		Restart: params.Conf.RunOnAvailableStdinRestart,
		Env:     env,
		Stdin:   inst.createStdin,
		OnExit: func(err error) {
			params.Logger.Log(logger.Info, "runOnAvailableStdin command exited: %v", err)
		},
	}
	inst.cmd = cmd
	cmd.Start()

	return inst.stop
}
