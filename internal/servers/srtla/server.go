// Package srtla contains an SRTLA (SRT Link Aggregation) receiver server.
package srtla

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net"
	"net/netip"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	srtlaTypeKeepalive = 0x9000
	srtlaTypeACK       = 0x9100
	srtlaTypeReg1      = 0x9200
	srtlaTypeReg2      = 0x9201
	srtlaTypeReg3      = 0x9202
	srtlaTypeRegErr    = 0x9210
	srtlaTypeRegNGP    = 0x9211
	srtlaTypeRegNAK    = 0x9212

	srtTypeACK = 0x8002

	srtlaIDLen   = 256
	srtlaReg1Len = 258 // type(2) + sender_id(256)
	srtlaReg2Len = 258 // type(2) + full_id(256)
	srtlaReg3Len = 2

	maxConnsPerGroup = 8
	maxGroups        = 200
	srtMinPacketSize = 16

	groupTimeout    = 10 * time.Second
	connTimeout     = 10 * time.Second
	cleanupInterval = 1 * time.Second
	recvACKInterval = 10
	readBufferSize  = 2048
)

type connEntry struct {
	addr     *net.UDPAddr
	addrPort netip.AddrPort
	lastSeen time.Time
	seqNums  []uint32
	seqCount int
}

type group struct {
	id             [srtlaIDLen]byte
	conns          []*connEntry
	srtConn    *net.UDPConn
	srtAddrKey string
	lastSeen       time.Time
	lastAddr       *net.UDPAddr
	path           string
	bytesReceived  atomic.Uint64
	bytesForwarded atomic.Uint64
}

type serverParent interface {
	logger.Writer
}

type serverMetrics interface {
	SetSRTLAServer(defs.APISRTLAServer)
}

func interfaceIsEmpty(i any) bool {
	return reflect.ValueOf(i).Kind() != reflect.Pointer || reflect.ValueOf(i).IsNil()
}

// udpAddrToAddrPort converts *net.UDPAddr to netip.AddrPort for canonical comparison.
func udpAddrToAddrPort(addr *net.UDPAddr) netip.AddrPort {
	return netip.AddrPortFrom(addr.AddrPort().Addr().Unmap(), addr.AddrPort().Port())
}

// Server is an SRTLA receiver that bonds multiple UDP connections
// and forwards them to a downstream SRT server.
type Server struct {
	Address    string
	SRTAddress string
	Metrics    serverMetrics
	Parent     serverParent

	ln           *net.UDPConn
	srtAddrPort netip.AddrPort
	wg           sync.WaitGroup
	done         chan struct{}
	mu           sync.Mutex
	closed       bool
	groups       map[[srtlaIDLen]byte]*group
	connIndex    map[netip.AddrPort]*group
	srtAddrIndex map[string]*group
}

// Initialize initializes the SRTLA server.
func (s *Server) Initialize() error {
	addr, err := net.ResolveUDPAddr("udp", s.Address)
	if err != nil {
		return err
	}

	s.ln, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	// Pre-resolve SRT target address.
	srtAddr, err := net.ResolveUDPAddr("udp", s.SRTAddress)
	if err != nil {
		s.ln.Close()
		return err
	}
	if srtAddr.IP == nil || srtAddr.IP.IsUnspecified() {
		if srtAddr.IP != nil && srtAddr.IP.To4() == nil {
			srtAddr.IP = net.IPv6loopback
		} else {
			srtAddr.IP = net.IPv4(127, 0, 0, 1)
		}
	}
	s.srtAddrPort = udpAddrToAddrPort(srtAddr)

	s.done = make(chan struct{})
	s.groups = make(map[[srtlaIDLen]byte]*group)
	s.connIndex = make(map[netip.AddrPort]*group)
	s.srtAddrIndex = make(map[string]*group)

	s.Log(logger.Info, "listener opened on %s (UDP)", s.Address)

	s.wg.Add(2)
	go s.readLoop()
	go s.cleanupLoop()

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetSRTLAServer(s)
	}

	return nil
}

// Close closes the SRTLA server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")

	if !interfaceIsEmpty(s.Metrics) {
		s.Metrics.SetSRTLAServer(nil)
	}

	close(s.done)
	s.ln.Close()

	s.mu.Lock()
	s.closed = true
	for _, g := range s.groups {
		if g.srtConn != nil {
			g.srtConn.Close()
		}
	}
	// Clear all maps so post-close method calls are true no-ops.
	s.groups = make(map[[srtlaIDLen]byte]*group)
	s.connIndex = make(map[netip.AddrPort]*group)
	s.srtAddrIndex = make(map[string]*group)
	s.mu.Unlock()

	s.wg.Wait()
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[SRTLA] "+format, args...)
}

func (s *Server) readLoop() {
	defer s.wg.Done()

	buf := make([]byte, readBufferSize)

	for {
		n, remoteAddr, err := s.ln.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				s.Log(logger.Error, "read error: %v", err)
				return
			}
		}

		if n < 2 {
			continue
		}

		s.handlePacket(buf[:n], remoteAddr)
	}
}

func (s *Server) handlePacket(data []byte, addr *net.UDPAddr) {
	pktType := binary.BigEndian.Uint16(data[:2])

	switch pktType {
	case srtlaTypeReg1:
		s.handleReg1(data, addr)
	case srtlaTypeReg2:
		s.handleReg2(data, addr)
	case srtlaTypeKeepalive:
		s.handleKeepalive(addr)
	default:
		if pktType&0xF000 == 0x9000 {
			return
		}
		s.handleData(data, addr)
	}
}

func (s *Server) handleReg1(data []byte, addr *net.UDPAddr) {
	if len(data) != srtlaReg1Len {
		return
	}

	addrPort := udpAddrToAddrPort(addr)

	s.mu.Lock()

	if _, exists := s.connIndex[addrPort]; exists {
		s.mu.Unlock()
		s.sendRegErr(addr)
		return
	}

	if len(s.groups) >= maxGroups {
		s.mu.Unlock()
		s.sendRegErr(addr)
		return
	}

	// sender_id occupies the first half; receiver generates the second half.
	var fullID [srtlaIDLen]byte
	copy(fullID[:srtlaIDLen/2], data[2:2+srtlaIDLen/2])

	_, err := rand.Read(fullID[srtlaIDLen/2:])
	if err != nil {
		s.mu.Unlock()
		s.sendRegErr(addr)
		return
	}

	now := time.Now()
	g := &group{
		id:       fullID,
		lastSeen: now,
		lastAddr: addr,
		conns: []*connEntry{{
			addr:     addr,
			addrPort: addrPort,
			lastSeen: now,
			seqNums:  make([]uint32, 0, recvACKInterval),
		}},
		// srtConn is nil — created lazily on first data packet.
	}

	s.groups[fullID] = g
	s.connIndex[addrPort] = g
	s.mu.Unlock()

	if err := s.sendReg2(addr, fullID); err != nil {
		s.mu.Lock()
		delete(s.groups, fullID)
		delete(s.connIndex, addrPort)
		s.mu.Unlock()
		s.Log(logger.Warn, "failed to send REG2 to %s: %v", addr, err)
		return
	}

	s.Log(logger.Debug, "new group registered from %s", addr)
}

func (s *Server) handleReg2(data []byte, addr *net.UDPAddr) {
	if len(data) != srtlaReg2Len {
		return
	}

	var fullID [srtlaIDLen]byte
	copy(fullID[:], data[2:2+srtlaIDLen])

	addrPort := udpAddrToAddrPort(addr)

	s.mu.Lock()

	g, ok := s.groups[fullID]
	if !ok {
		s.mu.Unlock()
		s.sendRegNGP(addr)
		return
	}

	if existingGroup, exists := s.connIndex[addrPort]; exists && existingGroup != g {
		s.mu.Unlock()
		s.sendRegErr(addr)
		return
	}

	now := time.Now()

	for _, c := range g.conns {
		if c.addrPort == addrPort {
			g.lastAddr = addr
			c.lastSeen = now
			s.mu.Unlock()
			if err := s.sendReg3(addr); err != nil {
				s.Log(logger.Warn, "failed to send REG3 to %s: %v", addr, err)
			}
			return
		}
	}

	if len(g.conns) >= maxConnsPerGroup {
		s.mu.Unlock()
		s.sendRegErr(addr)
		return
	}

	g.conns = append(g.conns, &connEntry{
		addr:     addr,
		addrPort: addrPort,
		lastSeen: now,
		seqNums:  make([]uint32, 0, recvACKInterval),
	})
	g.lastSeen = now
	g.lastAddr = addr
	s.connIndex[addrPort] = g
	s.mu.Unlock()

	if err := s.sendReg3(addr); err != nil {
		s.mu.Lock()
		if len(g.conns) > 0 {
			g.conns = g.conns[:len(g.conns)-1]
		}
		delete(s.connIndex, addrPort)
		s.mu.Unlock()
		s.Log(logger.Warn, "failed to send REG3 to %s: %v", addr, err)
		return
	}

	s.Log(logger.Debug, "connection added to group from %s (total: %d)", addr, len(g.conns))
}

func (s *Server) handleKeepalive(addr *net.UDPAddr) {
	addrPort := udpAddrToAddrPort(addr)

	s.mu.Lock()
	g, ok := s.connIndex[addrPort]
	if !ok {
		s.mu.Unlock()
		return
	}

	now := time.Now()
	g.lastSeen = now
	for _, c := range g.conns {
		if c.addrPort == addrPort {
			c.lastSeen = now
			break
		}
	}
	s.mu.Unlock()

	if err := s.sendKeepalive(addr); err != nil {
		s.Log(logger.Debug, "failed to send keepalive to %s: %v", addr, err)
	}
}

func (s *Server) handleData(data []byte, addr *net.UDPAddr) {
	if len(data) < srtMinPacketSize {
		return
	}

	addrPort := udpAddrToAddrPort(addr)

	s.mu.Lock()
	g, ok := s.connIndex[addrPort]
	if !ok {
		s.mu.Unlock()
		return
	}

	now := time.Now()
	g.lastSeen = now
	g.lastAddr = addr

	var conn *connEntry
	for _, c := range g.conns {
		if c.addrPort == addrPort {
			c.lastSeen = now
			conn = c
			break
		}
	}

	if conn == nil {
		s.mu.Unlock()
		return
	}

	// SRT data packets: bit 7 of byte 0 is 0; control packets: bit 7 is 1.
	var ackAddr *net.UDPAddr
	if data[0]&0x80 == 0 {
		seqNum := binary.BigEndian.Uint32(data[:4])
		conn.seqNums = append(conn.seqNums, seqNum)
		conn.seqCount++

		if conn.seqCount >= recvACKInterval {
			ackAddr = addr
			seqNums := make([]uint32, len(conn.seqNums))
			copy(seqNums, conn.seqNums)
			conn.seqNums = conn.seqNums[:0]
			conn.seqCount = 0

			defer func() {
				if err := s.sendSRTLAACK(ackAddr, seqNums); err != nil {
					s.Log(logger.Debug, "failed to send SRTLA ACK to %s: %v", ackAddr, err)
				}
			}()
		}
	}

	srtConn := g.srtConn
	if srtConn == nil {
		s.mu.Unlock()
		newConn, err := s.createSRTConn(g)
		if err != nil {
			s.Log(logger.Error, "failed to create SRT connection for group: %v", err)
			return
		}
		srtConn = newConn
	} else {
		s.mu.Unlock()
	}

	g.bytesReceived.Add(uint64(len(data)))
	n2, err := srtConn.Write(data)
	if err != nil {
		s.Log(logger.Debug, "failed to forward data to SRT: %v", err)
	}
	if n2 > 0 {
		g.bytesForwarded.Add(uint64(n2))
	}
}

// createSRTConn lazily creates the backend SRT connection for a group.
// Must be called WITHOUT holding s.mu.
func (s *Server) createSRTConn(g *group) (*net.UDPConn, error) {
	srtUDPAddr := net.UDPAddrFromAddrPort(s.srtAddrPort)
	srtConn, err := net.DialUDP("udp", nil, srtUDPAddr)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		srtConn.Close()
		return nil, net.ErrClosed
	}
	// Double-check: another goroutine may have created it concurrently.
	if g.srtConn != nil {
		s.mu.Unlock()
		srtConn.Close()
		return g.srtConn, nil
	}
	g.srtConn = srtConn
	g.srtAddrKey = canonicalLocalAddr(srtConn)
	s.srtAddrIndex[g.srtAddrKey] = g
	s.wg.Add(1)
	s.mu.Unlock()

	go s.srtReadLoop(g)

	return srtConn, nil
}

// canonicalLocalAddr returns a canonical string for the local address of a connected UDP socket.
// Uses netip.AddrPort with unmapped IPv4 for consistent correlation with SRT RemoteAddr.
func canonicalLocalAddr(conn *net.UDPConn) string {
	addr := conn.LocalAddr().(*net.UDPAddr)
	ap := netip.AddrPortFrom(addr.AddrPort().Addr().Unmap(), addr.AddrPort().Port())
	return ap.String()
}

func (s *Server) srtReadLoop(g *group) {
	defer s.wg.Done()

	buf := make([]byte, readBufferSize)

	for {
		n, err := g.srtConn.Read(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				return
			}
		}

		if n < srtMinPacketSize {
			continue
		}

		pktType := binary.BigEndian.Uint16(buf[:2])
		pktData := make([]byte, n)
		copy(pktData, buf[:n])

		s.mu.Lock()
		if pktType == srtTypeACK {
			addrs := make([]*net.UDPAddr, 0, len(g.conns))
			for _, c := range g.conns {
				addrs = append(addrs, c.addr)
			}
			s.mu.Unlock()
			for _, a := range addrs {
				if _, err := s.ln.WriteToUDP(pktData, a); err != nil {
					s.Log(logger.Debug, "failed to send SRT ACK to %s: %v", a, err)
				}
			}
		} else {
			dst := g.lastAddr
			s.mu.Unlock()
			if dst != nil {
				if _, err := s.ln.WriteToUDP(pktData, dst); err != nil {
					s.Log(logger.Debug, "failed to route SRT packet to %s: %v", dst, err)
				}
			}
		}
	}
}

func (s *Server) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *Server) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for id, g := range s.groups {
		if now.Sub(g.lastSeen) > groupTimeout {
			for _, c := range g.conns {
				delete(s.connIndex, c.addrPort)
			}
			if g.srtConn != nil {
				delete(s.srtAddrIndex, g.srtAddrKey)
				g.srtConn.Close()
			}
			delete(s.groups, id)
			s.Log(logger.Debug, "group timed out and removed (path: %s)", g.path)
			continue
		}

		alive := g.conns[:0]
		for _, c := range g.conns {
			if now.Sub(c.lastSeen) > connTimeout {
				delete(s.connIndex, c.addrPort)
				s.Log(logger.Debug, "connection timed out: %s", c.addr)
			} else {
				alive = append(alive, c)
			}
		}
		g.conns = alive

		// Repair lastAddr if the timed-out conn was the current target.
		if len(g.conns) > 0 {
			lastAddrStillAlive := false
			for _, c := range g.conns {
				if g.lastAddr != nil && c.addrPort == udpAddrToAddrPort(g.lastAddr) {
					lastAddrStillAlive = true
					break
				}
			}
			if !lastAddrStillAlive {
				g.lastAddr = g.conns[0].addr
			}
		}

		if len(g.conns) == 0 {
			if g.srtConn != nil {
				delete(s.srtAddrIndex, g.srtAddrKey)
				g.srtConn.Close()
			}
			delete(s.groups, id)
			s.Log(logger.Debug, "group removed (path: %s, no connections remaining)", g.path)
		}
	}
}

// SetGroupPath sets the stream path for the SRTLA group identified by the SRT connection address.
func (s *Server) SetGroupPath(srtConnAddr string, path string) {
	key := s.normalizeSRTAddr(srtConnAddr)

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.srtAddrIndex[key]
	if !ok {
		return
	}
	g.path = path
	s.Log(logger.Debug, "group path set to '%s'", path)
}

// CloseGroupByAddr closes the SRTLA group associated with the given SRT connection address.
func (s *Server) CloseGroupByAddr(srtConnAddr string) {
	key := s.normalizeSRTAddr(srtConnAddr)

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.srtAddrIndex[key]
	if !ok {
		return
	}

	for _, c := range g.conns {
		delete(s.connIndex, c.addrPort)
	}
	delete(s.srtAddrIndex, key)
	delete(s.groups, g.id)
	if g.srtConn != nil {
		g.srtConn.Close()
	}

	s.Log(logger.Debug, "group closed by SRT (path: %s)", g.path)
}

// normalizeSRTAddr converts a raw address string to canonical form for srtAddrIndex lookup.
func (s *Server) normalizeSRTAddr(raw string) string {
	ap, err := netip.ParseAddrPort(raw)
	if err != nil {
		return raw
	}
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()).String()
}

// APISRTLAGroupsList returns info about all active SRTLA groups for metrics.
func (s *Server) APISRTLAGroupsList() []defs.APISRTLAGroup {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]defs.APISRTLAGroup, 0, len(s.groups))
	for _, g := range s.groups {
		items = append(items, defs.APISRTLAGroup{
			ID:             hex.EncodeToString(g.id[:]),
			Path:           g.path,
			ConnsActive:    len(g.conns),
			BytesReceived:  g.bytesReceived.Load(),
			BytesForwarded: g.bytesForwarded.Load(),
		})
	}
	return items
}

func (s *Server) sendReg2(addr *net.UDPAddr, id [srtlaIDLen]byte) error {
	pkt := make([]byte, srtlaReg2Len)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeReg2)
	copy(pkt[2:], id[:])
	_, err := s.ln.WriteToUDP(pkt, addr)
	return err
}

func (s *Server) sendReg3(addr *net.UDPAddr) error {
	pkt := make([]byte, srtlaReg3Len)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeReg3)
	_, err := s.ln.WriteToUDP(pkt, addr)
	return err
}

func (s *Server) sendRegErr(addr *net.UDPAddr) {
	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeRegErr)
	if _, err := s.ln.WriteToUDP(pkt, addr); err != nil {
		s.Log(logger.Debug, "failed to send REG_ERR to %s: %v", addr, err)
	}
}

func (s *Server) sendRegNGP(addr *net.UDPAddr) {
	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeRegNGP)
	if _, err := s.ln.WriteToUDP(pkt, addr); err != nil {
		s.Log(logger.Debug, "failed to send REG_NGP to %s: %v", addr, err)
	}
}

func (s *Server) sendKeepalive(addr *net.UDPAddr) error {
	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeKeepalive)
	_, err := s.ln.WriteToUDP(pkt, addr)
	return err
}

func (s *Server) sendSRTLAACK(addr *net.UDPAddr, seqNums []uint32) error {
	// Sender parses ACKs as uint32_t[] starting at index 1 (byte 4), so header must be 4 bytes.
	pkt := make([]byte, 4+4*len(seqNums))
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeACK)
	for i, seq := range seqNums {
		binary.BigEndian.PutUint32(pkt[4+4*i:], seq)
	}
	_, err := s.ln.WriteToUDP(pkt, addr)
	return err
}
