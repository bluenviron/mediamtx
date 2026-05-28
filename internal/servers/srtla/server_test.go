package srtla

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/stretchr/testify/require"
)

type testLogger struct{}

func (l *testLogger) Log(_ logger.Level, _ string, _ ...any) {}

func allocateUDPListener(t *testing.T) *net.UDPConn {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	return conn
}

func TestServerRegistration(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	client, err := net.ListenUDP("udp", clientAddr)
	require.NoError(t, err)
	defer client.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	// REG1: type(2) + sender_id(128 bytes, first half of 256-byte ID)
	reg1 := make([]byte, srtlaReg1Len)
	binary.BigEndian.PutUint16(reg1[:2], srtlaTypeReg1)
	for i := 0; i < srtlaIDLen/2; i++ {
		reg1[2+i] = byte(i)
	}
	_, err = client.WriteToUDP(reg1, srtlaAddr)
	require.NoError(t, err)

	buf := make([]byte, 512)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := client.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtlaReg2Len, n)
	require.Equal(t, uint16(srtlaTypeReg2), binary.BigEndian.Uint16(buf[:2]))

	var fullID [srtlaIDLen]byte
	copy(fullID[:], buf[2:2+srtlaIDLen])

	for i := 0; i < srtlaIDLen/2; i++ {
		require.Equal(t, byte(i), fullID[i], "sender_id byte %d mismatch", i)
	}

	s.mu.Lock()
	require.Len(t, s.groups, 1)
	g := s.groups[fullID]
	require.NotNil(t, g)
	require.Len(t, g.conns, 1)
	s.mu.Unlock()
}

func TestServerDataForwarding(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	doRegistration(t, client, srtlaAddr)

	// SRT data packet: bit 7 of byte[0] = 0 means data; first 4 bytes = sequence number.
	dataPkt := make([]byte, 64)
	binary.BigEndian.PutUint32(dataPkt[:4], 0x00000042)
	copy(dataPkt[4:], []byte("SRT-PAYLOAD-DATA"))

	_, err = client.WriteToUDP(dataPkt, srtlaAddr)
	require.NoError(t, err)

	buf := make([]byte, 256)
	srtBackend.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := srtBackend.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, 64, n)
	require.Equal(t, uint32(0x00000042), binary.BigEndian.Uint32(buf[:4]))
	require.Equal(t, []byte("SRT-PAYLOAD-DATA"), buf[4:20])
}

func TestServerResponseRouting(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	doRegistration(t, client, srtlaAddr)

	dataPkt := make([]byte, srtMinPacketSize)
	binary.BigEndian.PutUint32(dataPkt[:4], 0x00000001)
	_, err = client.WriteToUDP(dataPkt, srtlaAddr)
	require.NoError(t, err)

	buf := make([]byte, 256)
	srtBackend.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, fromAddr, err := srtBackend.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtMinPacketSize, n)

	// SRT control packet (bit 7 = 1, type 0x8003 = NAK) → routed to lastAddr only.
	responsePkt := make([]byte, srtMinPacketSize)
	binary.BigEndian.PutUint16(responsePkt[:2], 0x8003)
	copy(responsePkt[4:], []byte("RESPONSE"))
	_, err = srtBackend.WriteToUDP(responsePkt, fromAddr)
	require.NoError(t, err)

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err = client.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtMinPacketSize, n)
	require.Equal(t, uint16(0x8003), binary.BigEndian.Uint16(buf[:2]))
}

func TestServerACKBroadcast(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client1, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client1.Close()

	fullID := doRegistration(t, client1, srtlaAddr)

	client2, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client2.Close()

	doRegistrationJoin(t, client2, srtlaAddr, fullID)

	dataPkt := make([]byte, srtMinPacketSize)
	binary.BigEndian.PutUint32(dataPkt[:4], 0x00000001)
	_, err = client1.WriteToUDP(dataPkt, srtlaAddr)
	require.NoError(t, err)

	buf := make([]byte, 256)
	srtBackend.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, fromAddr, err := srtBackend.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtMinPacketSize, n)

	// SRT ACK (0x8002) must be broadcast to ALL connections in the group.
	ackPkt := make([]byte, srtMinPacketSize)
	binary.BigEndian.PutUint16(ackPkt[:2], srtTypeACK)
	copy(ackPkt[4:], []byte("ACK-PAYLOAD!"))
	_, err = srtBackend.WriteToUDP(ackPkt, fromAddr)
	require.NoError(t, err)

	client1.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err = client1.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtMinPacketSize, n)
	require.Equal(t, uint16(srtTypeACK), binary.BigEndian.Uint16(buf[:2]))

	client2.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err = client2.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtMinPacketSize, n)
	require.Equal(t, uint16(srtTypeACK), binary.BigEndian.Uint16(buf[:2]))
}

func TestServerSRTLAACKGeneration(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	doRegistration(t, client, srtlaAddr)

	for i := 0; i < recvACKInterval; i++ {
		pkt := make([]byte, srtMinPacketSize)
		binary.BigEndian.PutUint32(pkt[:4], uint32(100+i))
		_, err = client.WriteToUDP(pkt, srtlaAddr)
		require.NoError(t, err)
	}

	srtBackend.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 256)
	for {
		_, _, err := srtBackend.ReadFromUDP(buf)
		if err != nil {
			break
		}
	}

	// SRTLA ACK format: 4-byte header (type u16 + 2 padding) + 4 bytes per seq number.
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := client.ReadFromUDP(buf)
	require.NoError(t, err)

	expectedLen := 4 + 4*recvACKInterval
	require.Equal(t, expectedLen, n)
	require.Equal(t, uint16(srtlaTypeACK), binary.BigEndian.Uint16(buf[:2]))

	for i := 0; i < recvACKInterval; i++ {
		seq := binary.BigEndian.Uint32(buf[4+4*i:])
		require.Equal(t, uint32(100+i), seq, "seq[%d]", i)
	}
}

func TestServerKeepalive(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	doRegistration(t, client, srtlaAddr)

	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeKeepalive)
	_, err = client.WriteToUDP(pkt, srtlaAddr)
	require.NoError(t, err)

	buf := make([]byte, 64)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := client.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, uint16(srtlaTypeKeepalive), binary.BigEndian.Uint16(buf[:2]))
}

func TestServerReg1OnlyNoBackendSocket(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	fullID := doRegistration(t, client, srtlaAddr)

	s.mu.Lock()
	g := s.groups[fullID]
	require.NotNil(t, g)
	require.Nil(t, g.srtConn, "srtConn must NOT be created after REG1+REG2 alone")
	require.Empty(t, s.srtAddrIndex, "srtAddrIndex must be empty before data")
	s.mu.Unlock()
}

func TestServerDataBeforeReg2Ignored(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	unregisteredClient, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer unregisteredClient.Close()

	dataPkt := make([]byte, srtMinPacketSize)
	binary.BigEndian.PutUint32(dataPkt[:4], 0x00000001)
	_, err = unregisteredClient.WriteToUDP(dataPkt, srtlaAddr)
	require.NoError(t, err)

	srtBackend.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = srtBackend.ReadFromUDP(make([]byte, 256))
	require.Error(t, err, "data from unknown addr must NOT be forwarded to SRT backend")
}

func TestServerKeepaliveBeforeRegistrationIgnored(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeKeepalive)
	_, err = client.WriteToUDP(pkt, srtlaAddr)
	require.NoError(t, err)

	client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err = client.ReadFromUDP(make([]byte, 64))
	require.Error(t, err, "keepalive from unknown addr must NOT get response")
}

func TestServerClosesClearsState(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)

	srtlaAddr := s.ln.LocalAddr().(*net.UDPAddr)

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer client.Close()

	doRegistration(t, client, srtlaAddr)

	s.mu.Lock()
	require.Len(t, s.groups, 1)
	s.mu.Unlock()

	s.Close()

	s.mu.Lock()
	require.Empty(t, s.groups, "groups must be empty after Close()")
	require.Empty(t, s.connIndex, "connIndex must be empty after Close()")
	require.Empty(t, s.srtAddrIndex, "srtAddrIndex must be empty after Close()")
	s.mu.Unlock()
}

func TestServerSetGroupPathPostClose(t *testing.T) {
	srtBackend := allocateUDPListener(t)
	defer srtBackend.Close()

	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: srtBackend.LocalAddr().String(),
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	s.Close()

	require.NotPanics(t, func() {
		s.SetGroupPath("127.0.0.1:12345", "/live/test")
	})
	require.NotPanics(t, func() {
		s.CloseGroupByAddr("127.0.0.1:12345")
	})
}

func TestServerIPv6LoopbackResolution(t *testing.T) {
	srtBackend, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	if err != nil {
		t.Skip("IPv6 loopback not available")
	}
	defer srtBackend.Close()

	port := srtBackend.LocalAddr().(*net.UDPAddr).Port

	s := &Server{
		Address:    "[::1]:0",
		SRTAddress: "[::]:"+fmt.Sprint(port),
		Parent:     &testLogger{},
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	require.Equal(t, netip.MustParseAddr("::1"), s.srtAddrPort.Addr())
}

func TestServerIPv4LoopbackResolution(t *testing.T) {
	s := &Server{
		Address:    "127.0.0.1:0",
		SRTAddress: ":8890",
		Parent:     &testLogger{},
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	require.Equal(t, netip.MustParseAddr("127.0.0.1"), s.srtAddrPort.Addr())
}

func doRegistration(t *testing.T, client *net.UDPConn, srtlaAddr *net.UDPAddr) [srtlaIDLen]byte {
	t.Helper()

	reg1 := make([]byte, srtlaReg1Len)
	binary.BigEndian.PutUint16(reg1[:2], srtlaTypeReg1)
	for i := 0; i < srtlaIDLen/2; i++ {
		reg1[2+i] = byte(i)
	}
	_, err := client.WriteToUDP(reg1, srtlaAddr)
	require.NoError(t, err)

	buf := make([]byte, 512)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := client.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtlaReg2Len, n)
	require.Equal(t, uint16(srtlaTypeReg2), binary.BigEndian.Uint16(buf[:2]))

	var fullID [srtlaIDLen]byte
	copy(fullID[:], buf[2:2+srtlaIDLen])
	return fullID
}

func doRegistrationJoin(t *testing.T, client *net.UDPConn, srtlaAddr *net.UDPAddr, fullID [srtlaIDLen]byte) {
	t.Helper()

	reg2 := make([]byte, srtlaReg2Len)
	binary.BigEndian.PutUint16(reg2[:2], srtlaTypeReg2)
	copy(reg2[2:], fullID[:])
	_, err := client.WriteToUDP(reg2, srtlaAddr)
	require.NoError(t, err)

	buf := make([]byte, 64)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := client.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, srtlaReg3Len, n)
	require.Equal(t, uint16(srtlaTypeReg3), binary.BigEndian.Uint16(buf[:2]))
}
