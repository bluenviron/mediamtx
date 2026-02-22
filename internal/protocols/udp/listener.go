// Package udp contains utilities to work with the UDP protocol.
package udp

import (
	"fmt"
	"net"
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

// Listener is a listener on a UDP socket.
type Listener struct {
	Address           string
	Source            string
	IntfName          string
	UDPReadBufferSize int
	ListenPacket      func(network, address string) (net.PacketConn, error)

	pc       packetConn
	sourceIP net.IP
}

// Initialize initializes the listener.
func (l *Listener) Initialize() error {
	if l.ListenPacket == nil {
		l.ListenPacket = net.ListenPacket
	}

	if l.Source != "" {
		l.sourceIP = net.ParseIP(l.Source)
		if l.sourceIP == nil {
			return fmt.Errorf("invalid source IP")
		}
	}

	addr, err := net.ResolveUDPAddr("udp", l.Address)
	if err != nil {
		return err
	}

	if ip4 := addr.IP.To4(); ip4 != nil && addr.IP.IsMulticast() {
		var intf *net.Interface

		if l.IntfName != "" {
			intf, err = net.InterfaceByName(l.IntfName)
			if err != nil {
				return err
			}
		} else {
			intf, err = defaultInterfaceForMulticast(addr)
			if err != nil {
				return err
			}
		}

		l.pc, err = multicast.NewSingleConn(intf, addr.String(), l.ListenPacket)
		if err != nil {
			return err
		}
	} else {
		var tmp net.PacketConn
		tmp, err = l.ListenPacket(restrictnetwork.Restrict("udp", addr.String()))
		if err != nil {
			return err
		}
		l.pc = tmp.(packetConn)
	}

	if l.UDPReadBufferSize != 0 {
		err = readbuffer.SetReadBuffer(l.pc, l.UDPReadBufferSize)
		if err != nil {
			l.pc.Close()
			return err
		}
	}

	return nil
}

// Close closes the listener.
func (l *Listener) Close() error {
	return l.pc.Close()
}

// Read implements net.Conn.
func (l *Listener) Read(p []byte) (int, error) {
	for {
		n, addr, err := l.pc.ReadFrom(p)

		if l.sourceIP != nil && addr != nil && !addr.(*net.UDPAddr).IP.Equal(l.sourceIP) {
			continue
		}

		return n, err
	}
}

// Write implements net.Conn.
func (l *Listener) Write(_ []byte) (int, error) {
	panic("unimplemented")
}

// LocalAddr implements net.Conn.
func (l *Listener) LocalAddr() net.Addr {
	panic("unimplemented")
}

// RemoteAddr implements net.Conn.
func (l *Listener) RemoteAddr() net.Addr {
	panic("unimplemented")
}

// SetDeadline implements net.Conn.
func (l *Listener) SetDeadline(_ time.Time) error {
	panic("unimplemented")
}

// SetReadDeadline implements net.Conn.
func (l *Listener) SetReadDeadline(t time.Time) error {
	return l.pc.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn.
func (l *Listener) SetWriteDeadline(_ time.Time) error {
	panic("unimplemented")
}
