package main

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/base"
)

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}

func splitPath(path string) (string, string, error) {
	pos := func() int {
		for i := len(path) - 1; i >= 0; i-- {
			if path[i] == '/' {
				return i
			}
		}
		return -1
	}()

	if pos < 0 {
		return "", "", fmt.Errorf("the path must contain a base path and a control path (%s)", path)
	}

	basePath := path[:pos]
	controlPath := path[pos+1:]

	if len(basePath) == 0 {
		return "", "", fmt.Errorf("empty base path (%s)", basePath)
	}

	if len(controlPath) == 0 {
		return "", "", fmt.Errorf("empty control path (%s)", controlPath)
	}

	return basePath, controlPath, nil
}

func removeQueryFromPath(path string) string {
	i := strings.Index(path, "?")
	if i >= 0 {
		return path[:i]
	}
	return path
}

type udpPublisherAddr struct {
	ip   [net.IPv6len]byte // use a fixed-size array to enable the equality operator
	port int
}

func makeUDPPublisherAddr(ip net.IP, port int) udpPublisherAddr {
	ret := udpPublisherAddr{
		port: port,
	}

	if len(ip) == net.IPv4len {
		copy(ret.ip[0:], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}) // v4InV6Prefix
		copy(ret.ip[12:], ip)
	} else {
		copy(ret.ip[:], ip)
	}

	return ret
}

type udpPublisher struct {
	client     *client
	trackId    int
	streamType gortsplib.StreamType
}

type udpPublishersMap struct {
	mutex sync.RWMutex
	ma    map[udpPublisherAddr]*udpPublisher
}

func newUdpPublisherMap() *udpPublishersMap {
	return &udpPublishersMap{
		ma: make(map[udpPublisherAddr]*udpPublisher),
	}
}

func (m *udpPublishersMap) clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ma = make(map[udpPublisherAddr]*udpPublisher)
}

func (m *udpPublishersMap) add(addr udpPublisherAddr, pub *udpPublisher) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ma[addr] = pub
}

func (m *udpPublishersMap) remove(addr udpPublisherAddr) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.ma, addr)
}

func (m *udpPublishersMap) get(addr udpPublisherAddr) *udpPublisher {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	el, ok := m.ma[addr]
	if !ok {
		return nil
	}
	return el
}

type readersMap struct {
	mutex sync.RWMutex
	ma    map[*client]struct{}
}

func newReadersMap() *readersMap {
	return &readersMap{
		ma: make(map[*client]struct{}),
	}
}

func (m *readersMap) clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ma = make(map[*client]struct{})
}

func (m *readersMap) add(reader *client) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.ma[reader] = struct{}{}
}

func (m *readersMap) remove(reader *client) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.ma, reader)
}

func (m *readersMap) forwardFrame(path *path, trackId int, streamType gortsplib.StreamType, frame []byte) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		if c.path != path {
			continue
		}

		track, ok := c.streamTracks[trackId]
		if !ok {
			continue
		}

		if c.streamProtocol == gortsplib.StreamProtocolUDP {
			if streamType == gortsplib.StreamTypeRtp {
				c.p.serverUdpRtp.write(frame, &net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtpPort,
				})

			} else {
				c.p.serverUdpRtcp.write(frame, &net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtcpPort,
				})
			}

		} else {
			c.tcpFrame <- &base.InterleavedFrame{
				TrackId:    trackId,
				StreamType: streamType,
				Content:    frame,
			}
		}
	}
}
