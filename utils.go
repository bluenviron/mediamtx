package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/aler9/gortsplib"
)

func parseIpCidrList(in []string) ([]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}

	var ret []interface{}
	for _, t := range in {
		_, ipnet, err := net.ParseCIDR(t)
		if err == nil {
			ret = append(ret, ipnet)
			continue
		}

		ip := net.ParseIP(t)
		if ip != nil {
			ret = append(ret, ip)
			continue
		}

		return nil, fmt.Errorf("unable to parse ip/network '%s'", t)
	}
	return ret, nil
}

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

type multiBuffer struct {
	buffers [][]byte
	curBuf  int
}

func newMultiBuffer(count int, size int) *multiBuffer {
	buffers := make([][]byte, count)
	for i := 0; i < count; i++ {
		buffers[i] = make([]byte, size)
	}

	return &multiBuffer{
		buffers: buffers,
	}
}

func (mb *multiBuffer) next() []byte {
	ret := mb.buffers[mb.curBuf]
	mb.curBuf += 1
	if mb.curBuf >= len(mb.buffers) {
		mb.curBuf = 0
	}
	return ret
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

var rePathName = regexp.MustCompile("^[0-9a-zA-Z_\\-/]+$")

func checkPathName(name string) error {
	if !rePathName.MatchString(name) {
		return fmt.Errorf("can contain only alfanumeric characters, underscore, minus or slash")
	}

	if name[0] == '/' {
		return fmt.Errorf("can't begin with a slash")
	}

	if name[len(name)-1] == '/' {
		return fmt.Errorf("can't end with a slash")
	}

	return nil
}

func startExternalCommand(cmdstr string, pathName string) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// in Windows the shell is not used and command is started directly
		// variables are replaced manually in order to allow
		// compatibility with linux commands
		cmdstr = strings.ReplaceAll(cmdstr, "$RTSP_SERVER_PATH", pathName)
		args := strings.Fields(cmdstr)
		cmd = exec.Command(args[0], args[1:]...)

	} else {
		cmd = exec.Command("/bin/sh", "-c", "exec "+cmdstr)
	}

	// variables are available through environment variables
	cmd.Env = append(os.Environ(),
		"RTSP_SERVER_PATH="+pathName,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func isBindError(err error) bool {
	if nerr, ok := err.(*net.OpError); ok {
		if serr, ok := nerr.Err.(*os.SyscallError); ok {
			if serr.Syscall == "bind" {
				return true
			}
		}
	}
	return false
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
				c.p.serverRtp.write(frame, &net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtpPort,
				})

			} else {
				c.p.serverRtcp.write(frame, &net.UDPAddr{
					IP:   c.ip(),
					Zone: c.zone(),
					Port: track.rtcpPort,
				})
			}

		} else {
			c.tcpFrame <- &gortsplib.InterleavedFrame{
				TrackId:    trackId,
				StreamType: streamType,
				Content:    frame,
			}
		}
	}
}
