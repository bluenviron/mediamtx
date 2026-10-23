package certloader_test

import (
	"crypto/tls"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCertReload(t *testing.T) {
	testData, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	serverCertPath := test.CreateTempFile(t, test.TLSCertPub)
	serverKeyPath := test.CreateTempFile(t, test.TLSCertKey)

	loader := &certloader.CertLoader{
		CertPath: serverCertPath,
		KeyPath:  serverKeyPath,
		Parent:   test.NilLogger,
	}
	err = loader.Initialize()
	require.NoError(t, err)
	defer loader.Close()

	cert, err := loader.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, &testData, cert)

	testData, err = tls.X509KeyPair(test.TLSCertPubAlt, test.TLSCertKeyAlt)
	require.NoError(t, err)

	err = os.WriteFile(serverCertPath, test.TLSCertPubAlt, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(serverKeyPath, test.TLSCertKeyAlt, 0o644)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	cert, err = loader.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, &testData, cert)
}

func TestCloseDuringReload(t *testing.T) {
	serverCertPath := test.CreateTempFile(t, test.TLSCertPub)
	serverKeyPath := test.CreateTempFile(t, test.TLSCertKey)

	logEntered := make(chan struct{}, 1)
	logRelease := make(chan struct{})
	var blockLog atomic.Bool

	loader := &certloader.CertLoader{
		CertPath: serverCertPath,
		KeyPath:  serverKeyPath,
		Parent: test.Logger(func(logger.Level, string, ...any) {
			if !blockLog.Load() {
				return
			}
			select {
			case logEntered <- struct{}{}:
			default:
			}
			<-logRelease
		}),
	}
	err := loader.Initialize()
	require.NoError(t, err)

	closeDone := make(chan struct{})
	var closeOnce sync.Once
	startClose := func() {
		closeOnce.Do(func() {
			go func() {
				loader.Close()
				close(closeDone)
			}()
		})
	}

	defer func() {
		close(logRelease)
		startClose()

		select {
		case <-closeDone:
		case <-time.After(5 * time.Second):
			t.Errorf("Close() did not return")
		}
	}()

	blockLog.Store(true)
	err = os.WriteFile(serverCertPath, test.TLSCertPubAlt, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(serverKeyPath, test.TLSCertKeyAlt, 0o644)
	require.NoError(t, err)

	select {
	case <-logEntered:
	case <-time.After(5 * time.Second):
		t.Errorf("timed out")
		return
	}

	startClose()

	select {
	case <-time.After(500 * time.Millisecond):
	case <-closeDone:
		t.Errorf("Close() returned while a reload was in progress")
	}
}
