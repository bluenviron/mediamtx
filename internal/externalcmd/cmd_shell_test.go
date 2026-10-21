//go:build !windows

package externalcmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/require"
)

func TestExpandCommandPartsShell(t *testing.T) {
	script := "# keep $comment intact\naudio_packets=$(printf 7)\naudio_args='-c:a aac'\n" +
		"url='rtsp://user:literal$secret@127.0.0.1/stream'\n" +
		"printf '%s' \"$audio_packets $audio_args $MTX_PATH $url\""
	for _, shell := range []string{"sh", "/bin/sh", "bash", "/bin/bash", "dash", "/bin/dash"} {
		t.Run(shell, func(t *testing.T) {
			parts := []string{shell, "-c", script, "$MTX_PATH"}
			expandCommandParts(parts, Environment{"MTX_PATH": "camera", "secret": "wrong", "audio_args": "wrong"})
			require.Equal(t, script, parts[2])
			require.Equal(t, "camera", parts[3])
		})
	}
}

func TestExpandCommandPartsDirect(t *testing.T) {
	t.Setenv("PARENT_VAR", "parent value")
	parts := []string{"printf", "$MTX_PATH", "$PARENT_VAR"}
	expandCommandParts(parts, Environment{"MTX_PATH": "camera's path"})
	require.Equal(t, []string{"printf", "camera's path", "parent value"}, parts)
}

func TestCmdShellOwnsExpansion(t *testing.T) {
	t.Setenv("audio_args", "incorrect parent variable")
	pool := &Pool{}
	pool.Initialize()
	out := filepath.Join(t.TempDir(), "out")
	script := "# $comment must remain a comment\n" +
		"audio_packets=$(printf 7)\naudio_args='-c:a aac'\n" +
		"url='rtsp://user:literal$secret@127.0.0.1/stream'\n" +
		"printf '%s|%s|%s|%s' \"$audio_packets\" \"$audio_args\" \"$MTX_PATH\" \"$url\" > \"$1\""
	command := &Cmd{
		Pool: pool, Cmdstr: shellquote.Join("/bin/sh", "-c", script, "hook", out),
		Env: Environment{"MTX_PATH": "camera's path"},
	}
	command.Start()
	done := make(chan struct{})
	go func() { pool.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		command.Close()
		t.Fatal("shell hook did not complete")
	}
	byts, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, "7|-c:a aac|camera's path|rtsp://user:literal$secret@127.0.0.1/stream", string(byts))
}
