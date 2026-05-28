// Package srtla contains an SRTLA (SRT Link Aggregation) receiver server.
package srtla

import (
	"crypto/rand"
	"encoding/binary"
	"net"
	"sync"
	"time"

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
	lastSeen time.Time
	seqNums  []uint32
	seqCount int
}

type group struct {
	id       [srtlaIDLen]byte
	conns    []*connEntry
	srtConn  *net.UDPConn
	lastSeen time.Time
	lastAddr *net.UDPAddr
}

type serverParent interface {
	logger.Writer
}

// Server is an SRTLA receiver that bonds multiple UDP connections
// and forwards them to a downstream SRT server.
type Server struct {
	Address    string
	SRTAddress string
	Parent     serverParent

	ln        *net.UDPConn
	wg        sync.WaitGroup
	done      chan struct{}
	mu        sync.Mutex
	groups    map[[srtlaIDLen]byte]*group
	connIndex map[string]*group
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

	s.done = make(chan struct{})
	s.groups = make(map[[srtlaIDLen]byte]*group)
	s.connIndex = make(map[string]*group)

	s.Log(logger.Info, "listener opened on %s (UDP)", s.Address)

	s.wg.Add(2)
	go s.readLoop()
	go s.cleanupLoop()

	return nil
}

// Close closes the SRTLA server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")
	close(s.done)
	s.ln.Close()

	s.mu.Lock()
	for _, g := range s.groups {
		g.srtConn.Close()
	}
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.connIndex[addr.String()]; exists {
		s.sendRegErr(addr)
		return
	}

	if len(s.groups) >= maxGroups {
		s.sendRegErr(addr)
		return
	}

	// sender_id occupies the first half; receiver generates the second half.
	var fullID [srtlaIDLen]byte
	copy(fullID[:srtlaIDLen/2], data[2:2+srtlaIDLen/2])

	_, err := rand.Read(fullID[srtlaIDLen/2:])
	if err != nil {
		s.sendRegErr(addr)
		return
	}

	srtAddr, err := net.ResolveUDPAddr("udp", s.SRTAddress)
	if err != nil {
		s.sendRegErr(addr)
		return
	}
	// If SRTAddress has no explicit host (e.g. ":8890"), dial loopback.
	if srtAddr.IP == nil || srtAddr.IP.IsUnspecified() {
		srtAddr.IP = net.IPv4(127, 0, 0, 1)
	}

	srtConn, err := net.DialUDP("udp", nil, srtAddr)
	if err != nil {
		s.sendRegErr(addr)
		return
	}

	now := time.Now()
	g := &group{
		id:       fullID,
		srtConn:  srtConn,
		lastSeen: now,
		lastAddr: addr,
		conns: []*connEntry{{
			addr:     addr,
			lastSeen: now,
			seqNums:  make([]uint32, 0, recvACKInterval),
		}},
	}

	s.groups[fullID] = g
	s.connIndex[addr.String()] = g

	s.wg.Add(1)
	go s.srtReadLoop(g)

	s.sendReg2(addr, fullID)

	s.Log(logger.Debug, "new group registered from %s", addr)
}

func (s *Server) handleReg2(data []byte, addr *net.UDPAddr) {
	if len(data) != srtlaReg2Len {
		return
	}

	var fullID [srtlaIDLen]byte
	copy(fullID[:], data[2:2+srtlaIDLen])

	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.groups[fullID]
	if !ok {
		s.sendRegNGP(addr)
		return
	}

	addrStr := addr.String()
	if existingGroup, exists := s.connIndex[addrStr]; exists && existingGroup != g {
		s.sendRegErr(addr)
		return
	}

	for _, c := range g.conns {
		if c.addr.String() == addrStr {
			s.sendReg3(addr)
			return
		}
	}

	if len(g.conns) >= maxConnsPerGroup {
		s.sendRegErr(addr)
		return
	}

	now := time.Now()
	g.conns = append(g.conns, &connEntry{
		addr:     addr,
		lastSeen: now,
		seqNums:  make([]uint32, 0, recvACKInterval),
	})
	g.lastSeen = now
	g.lastAddr = addr
	s.connIndex[addrStr] = g

	s.sendReg3(addr)

	s.Log(logger.Debug, "connection added to group from %s (total: %d)", addr, len(g.conns))
}

func (s *Server) handleKeepalive(addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.connIndex[addr.String()]
	if !ok {
		return
	}

	now := time.Now()
	g.lastSeen = now
	for _, c := range g.conns {
		if c.addr.String() == addr.String() {
			c.lastSeen = now
			break
		}
	}

	s.sendKeepalive(addr)
}

func (s *Server) handleData(data []byte, addr *net.UDPAddr) {
	if len(data) < srtMinPacketSize {
		return
	}

	s.mu.Lock()
	g, ok := s.connIndex[addr.String()]
	if !ok {
		s.mu.Unlock()
		return
	}

	now := time.Now()
	g.lastSeen = now
	g.lastAddr = addr

	var conn *connEntry
	for _, c := range g.conns {
		if c.addr.String() == addr.String() {
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
	if data[0]&0x80 == 0 {
		seqNum := binary.BigEndian.Uint32(data[:4])
		conn.seqNums = append(conn.seqNums, seqNum)
		conn.seqCount++

		if conn.seqCount >= recvACKInterval {
			s.sendSRTLAACK(addr, conn.seqNums)
			conn.seqNums = conn.seqNums[:0]
			conn.seqCount = 0
		}
	}

	srtConn := g.srtConn
	s.mu.Unlock()

	_, _ = srtConn.Write(data)
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

		s.mu.Lock()
		if pktType == srtTypeACK {
			for _, c := range g.conns {
				_, _ = s.ln.WriteToUDP(buf[:n], c.addr)
			}
		} else if g.lastAddr != nil {
			_, _ = s.ln.WriteToUDP(buf[:n], g.lastAddr)
		}
		s.mu.Unlock()
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
				delete(s.connIndex, c.addr.String())
			}
			g.srtConn.Close()
			delete(s.groups, id)
			s.Log(logger.Debug, "group timed out and removed")
			continue
		}

		alive := g.conns[:0]
		for _, c := range g.conns {
			if now.Sub(c.lastSeen) > connTimeout {
				delete(s.connIndex, c.addr.String())
				s.Log(logger.Debug, "connection timed out: %s", c.addr)
			} else {
				alive = append(alive, c)
			}
		}
		g.conns = alive

		if len(g.conns) == 0 {
			g.srtConn.Close()
			delete(s.groups, id)
			s.Log(logger.Debug, "group removed (no connections remaining)")
		}
	}
}

func (s *Server) sendReg2(addr *net.UDPAddr, id [srtlaIDLen]byte) {
	pkt := make([]byte, srtlaReg2Len)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeReg2)
	copy(pkt[2:], id[:])
	_, _ = s.ln.WriteToUDP(pkt, addr)
}

func (s *Server) sendReg3(addr *net.UDPAddr) {
	pkt := make([]byte, srtlaReg3Len)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeReg3)
	_, _ = s.ln.WriteToUDP(pkt, addr)
}

func (s *Server) sendRegErr(addr *net.UDPAddr) {
	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeRegErr)
	_, _ = s.ln.WriteToUDP(pkt, addr)
}

func (s *Server) sendRegNGP(addr *net.UDPAddr) {
	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeRegNGP)
	_, _ = s.ln.WriteToUDP(pkt, addr)
}

func (s *Server) sendKeepalive(addr *net.UDPAddr) {
	pkt := make([]byte, 2)
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeKeepalive)
	_, _ = s.ln.WriteToUDP(pkt, addr)
}

func (s *Server) sendSRTLAACK(addr *net.UDPAddr, seqNums []uint32) {
	// Sender parses ACKs as uint32_t[] starting at index 1 (byte 4), so header must be 4 bytes.
	pkt := make([]byte, 4+4*len(seqNums))
	binary.BigEndian.PutUint16(pkt[:2], srtlaTypeACK)
	for i, seq := range seqNums {
		binary.BigEndian.PutUint32(pkt[4+4*i:], seq)
	}
	_, _ = s.ln.WriteToUDP(pkt, addr)
}
