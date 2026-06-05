package proxy

import (
	"fmt"
	"net"
	"testing"

	"github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
)

func trustedProxies(cidrs ...string) conf.IPNetworks {
	networks := make(conf.IPNetworks, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		networks = append(networks, conf.IPNetwork(*ipnet))
	}
	return networks
}

func TestListenerTrustedWithHeader(t *testing.T) {
	for _, version := range []byte{1, 2} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer ln.Close()

			wrapped := &Listener{Wrapped: ln, TrustedProxies: trustedProxies("127.0.0.1/32")}
			wrapped.Initialize()

			done := make(chan struct{})

			go func() {
				defer close(done)

				conn, err2 := wrapped.Accept()
				require.NoError(t, err2)
				defer conn.Close()

				require.Equal(t, "192.168.1.100:1234", conn.RemoteAddr().String())
			}()

			clientConn, err := net.Dial("tcp", ln.Addr().String())
			require.NoError(t, err)
			defer clientConn.Close()

			header := &proxyproto.Header{
				Version:           version,
				Command:           proxyproto.PROXY,
				TransportProtocol: proxyproto.TCPv4,
				SourceAddr:        &net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 1234},
				DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1935},
			}
			_, err = header.WriteTo(clientConn)
			require.NoError(t, err)

			<-done
		})
	}
}

func TestListenerTrustedWithoutHeader(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	wrapped := &Listener{Wrapped: ln, TrustedProxies: trustedProxies("127.0.0.1/32")}
	wrapped.Initialize()

	done := make(chan struct{})

	go func() {
		defer close(done)

		conn, err2 := wrapped.Accept()
		require.NoError(t, err2)
		defer conn.Close()

		buf := make([]byte, 7)
		_, err2 = conn.Read(buf)
		require.NoError(t, err2)
		require.Equal(t, []byte("testing"), buf)
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("testing"))
	require.NoError(t, err)

	<-done
}

func TestListenerUntrustedIgnoresHeader(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	wrapped := &Listener{Wrapped: ln, TrustedProxies: trustedProxies("10.0.0.0/8")}
	wrapped.Initialize()

	done := make(chan struct{})

	go func() {
		defer close(done)

		conn, err2 := wrapped.Accept()
		require.NoError(t, err2)
		defer conn.Close()

		require.Equal(t, "127.0.0.1", conn.RemoteAddr().(*net.TCPAddr).IP.String())
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	header := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 1234},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1935},
	}
	_, err = header.WriteTo(clientConn)
	require.NoError(t, err)

	<-done
}
