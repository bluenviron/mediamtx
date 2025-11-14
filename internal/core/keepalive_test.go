package core

import (
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func TestKeepaliveNew(t *testing.T) {
	pathName := "test/path"
	user := "testuser"
	ip := net.ParseIP("192.168.1.1")

	ka := newKeepalive(pathName, user, ip)

	require.NotEqual(t, uuid.Nil, ka.id)
	require.Equal(t, pathName, ka.pathName)
	require.Equal(t, user, ka.creatorUser)
	require.Equal(t, ip, ka.creatorIP)
	require.WithinDuration(t, time.Now(), ka.created, 1*time.Second)
}

func TestKeepaliveAPIReaderDescribe(t *testing.T) {
	ka := newKeepalive("test/path", "user", net.ParseIP("192.168.1.1"))

	desc := ka.APIReaderDescribe()

	require.Equal(t, "keepalive", desc.Type)
	require.Equal(t, ka.id.String(), desc.ID)
}

func TestKeepaliveClose(t *testing.T) {
	ka := newKeepalive("test/path", "user", net.ParseIP("192.168.1.1"))

	// test that Close doesn't panic when onClose is nil
	require.NotPanics(t, func() {
		ka.Close()
	})

	// test that onClose is called
	called := false
	ka.onClose = func() {
		called = true
	}
	ka.Close()
	require.True(t, called)
}

func TestKeepaliveLog(t *testing.T) {
	ka := newKeepalive("test/path", "user", net.ParseIP("192.168.1.1"))

	// test that Log doesn't panic
	require.NotPanics(t, func() {
		ka.Log(logger.Info, "test message %s", "arg")
	})
}

func TestKeepaliveAPIDescribe(t *testing.T) {
	pathName := "test/path"
	user := "testuser"
	ip := net.ParseIP("192.168.1.1")

	ka := newKeepalive(pathName, user, ip)

	apiDesc := ka.apiDescribe()

	require.Equal(t, ka.id, apiDesc.ID)
	require.Equal(t, pathName, apiDesc.Path)
	require.Equal(t, user, apiDesc.CreatorUser)
	require.Equal(t, ip.String(), apiDesc.CreatorIP)
	require.WithinDuration(t, ka.created, apiDesc.Created, 1*time.Millisecond)
}

func TestKeepaliveImplementsReader(t *testing.T) {
	ka := newKeepalive("test/path", "user", net.ParseIP("192.168.1.1"))

	// test that keepalive implements defs.Reader interface
	var _ defs.Reader = ka

	// test that keepalive implements logger.Writer interface
	var _ logger.Writer = ka
}
