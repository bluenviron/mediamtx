// Package udp contains utilities to work with the UDP protocol.
package udp

import (
	"fmt"
	"net"
	"net/url"
	"syscall"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/multicast"
	"github.com/bluenviron/gortsplib/v5/pkg/readbuffer"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

type packetConn interface {
	net.PacketConn
	SetReadBuffer(bytes int) error
	SyscallConn() (syscall.RawConn, error)
}

type udpConn struct {
	pc       net.PacketConn
	sourceIP net.IP
}

func (r *udpConn) Close() error {
	return r.pc.Close()
}

func (r *udpConn) Read(p []byte) (int, error) {
	for {
		n, addr, err := r.pc.ReadFrom(p)

		if r.sourceIP != nil && addr != nil && !addr.(*net.UDPAddr).IP.Equal(r.sourceIP) {
			continue
		}

		return n, err
	}
}

func (r *udpConn) Write(_ []byte) (int, error) {
	panic("unimplemented")
}

func (r *udpConn) LocalAddr() net.Addr {
	panic("unimplemented")
}

func (r *udpConn) RemoteAddr() net.Addr {
	panic("unimplemented")
}

func (r *udpConn) SetDeadline(_ time.Time) error {
	panic("unimplemented")
}

func (r *udpConn) SetReadDeadline(t time.Time) error {
	return r.pc.SetReadDeadline(t)
}

func (r *udpConn) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented")
}

func defaultInterfaceForMulticast(multicastAddr *net.UDPAddr) (*net.Interface, error) {
	conn, err := net.Dial("udp4", multicastAddr.String())
	if err != nil {
		return nil, err
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	conn.Close()

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		var addrs []net.Addr
		addrs, err = iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip != nil && ip.Equal(localAddr.IP) {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("could not find any interface for using multicast address %s", multicastAddr)
}

// Listen creates a UDP listener on the given URL.
func Listen(u *url.URL, udpReadBufferSize int) (net.Conn, error) {
	q := u.Query()
	var sourceIP net.IP

	if src := q.Get("source"); src != "" {
		sourceIP = net.ParseIP(src)
		if sourceIP == nil {
			return nil, fmt.Errorf("invalid source IP")
		}
	}

	addr, err := net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		return nil, err
	}

	var pc packetConn

	if ip4 := addr.IP.To4(); ip4 != nil && addr.IP.IsMulticast() {
		var intf *net.Interface

		if intfName := q.Get("interface"); intfName != "" {
			intf, err = net.InterfaceByName(intfName)
			if err != nil {
				return nil, err
			}
		} else {
			intf, err = defaultInterfaceForMulticast(addr)
			if err != nil {
				return nil, err
			}
		}

		pc, err = multicast.NewSingleConn(intf, addr.String(), net.ListenPacket)
		if err != nil {
			return nil, err
		}
	} else {
		var tmp net.PacketConn
		tmp, err = net.ListenPacket(restrictnetwork.Restrict("udp", addr.String()))
		if err != nil {
			return nil, err
		}
		pc = tmp.(*net.UDPConn)
	}

	if udpReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(pc, udpReadBufferSize)
		if err != nil {
			pc.Close()
			return nil, err
		}
	}

	return &udpConn{pc: pc, sourceIP: sourceIP}, nil
}
