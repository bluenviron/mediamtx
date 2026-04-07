package packetdumper

import (
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/google/uuid"
)

var _ net.PacketConn = (*packetConn)(nil)

type extendedPacketConn interface {
	net.PacketConn
	SetReadBuffer(bytes int) error
	SyscallConn() (syscall.RawConn, error)
}

type packetDumpEntry struct {
	ntp      time.Time
	data     []byte
	src, dst *net.UDPAddr
}

// packetConn is a wrapper around net.PacketConn that dumps packets to disk.
type packetConn struct {
	Prefix     string
	PacketConn net.PacketConn

	f    *os.File
	pw   *pcapgo.NgWriter
	once sync.Once

	queue      chan packetDumpEntry
	terminated chan struct{}
	done       chan struct{}
}

// Initialize initializes packetConn.
func (c *packetConn) Initialize() error {
	var err error
	c.f, err = os.Create(fmt.Sprintf("%s_%d_%s.pcapng", c.Prefix, time.Now().UnixNano(), uuid.New().String()))
	if err != nil {
		return err
	}

	c.pw, err = pcapgo.NewNgWriter(c.f, layers.LinkTypeEthernet)
	if err != nil {
		c.f.Close()
		return err
	}

	c.queue = make(chan packetDumpEntry, 64)
	c.terminated = make(chan struct{})
	c.done = make(chan struct{})

	go c.run()

	return nil
}

// Close implements net.PacketConn.
func (c *packetConn) Close() error {
	c.once.Do(func() {
		close(c.terminated)
	})
	<-c.done
	return c.PacketConn.Close()
}

func (c *packetConn) run() {
	defer close(c.done)
	defer c.f.Close()

	for {
		select {
		case e := <-c.queue:
			c.writePacket(e.ntp, e.src, e.dst, e.data)

		case <-c.terminated:
			for {
				select {
				case e := <-c.queue:
					c.writePacket(e.ntp, e.src, e.dst, e.data)
				default:
					c.pw.Flush() //nolint:errcheck
					return
				}
			}
		}
	}
}

func (c *packetConn) writePacket(ntp time.Time, src, dst *net.UDPAddr, payload []byte) {
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
		DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
		EthernetType: layers.EthernetTypeIPv6,
	}

	ipv6 := &layers.IPv6{
		Version:    6,
		SrcIP:      src.IP.To16(),
		DstIP:      dst.IP.To16(),
		NextHeader: layers.IPProtocolUDP,
		HopLimit:   64,
	}

	udp := &layers.UDP{
		SrcPort: layers.UDPPort(src.Port),
		DstPort: layers.UDPPort(dst.Port),
	}
	udp.SetNetworkLayerForChecksum(ipv6) //nolint:errcheck

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	gopacket.SerializeLayers(buf, opts, eth, ipv6, udp, gopacket.Payload(payload)) //nolint:errcheck

	raw := buf.Bytes()
	c.pw.WritePacket(gopacket.CaptureInfo{ //nolint:errcheck
		Timestamp:     ntp,
		CaptureLength: len(raw),
		Length:        len(raw),
	}, raw)
}

func (c *packetConn) enqueue(e packetDumpEntry) {
	select {
	case c.queue <- e:
	case <-c.terminated:
	}
}

// ReadFrom implements net.PacketConn.
func (c *packetConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = c.PacketConn.ReadFrom(p)

	if n != 0 {
		local := c.PacketConn.LocalAddr().(*net.UDPAddr)
		remote := addr.(*net.UDPAddr)

		c.enqueue(packetDumpEntry{
			ntp:  time.Now(),
			data: append([]byte(nil), p[:n]...),
			src:  remote,
			dst:  local,
		})
	}

	return n, addr, err
}

// WriteTo implements net.PacketConn.
func (c *packetConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	n, err = c.PacketConn.WriteTo(p, addr)

	if err == nil {
		local := c.PacketConn.LocalAddr().(*net.UDPAddr)
		remote := addr.(*net.UDPAddr)

		c.enqueue(packetDumpEntry{
			ntp:  time.Now(),
			data: append([]byte(nil), p...),
			src:  local,
			dst:  remote,
		})
	}

	return n, err
}

// LocalAddr implements net.PacketConn.
func (c *packetConn) LocalAddr() net.Addr { return c.PacketConn.LocalAddr() }

// SetDeadline implements net.PacketConn.
func (c *packetConn) SetDeadline(t time.Time) error { return c.PacketConn.SetDeadline(t) }

// SetReadDeadline implements net.PacketConn.
func (c *packetConn) SetReadDeadline(t time.Time) error { return c.PacketConn.SetReadDeadline(t) }

// SetWriteDeadline implements net.PacketConn.
func (c *packetConn) SetWriteDeadline(t time.Time) error { return c.PacketConn.SetWriteDeadline(t) }

// SetReadBuffer implements extendedPacketConn.
func (c *packetConn) SetReadBuffer(bytes int) error {
	return c.PacketConn.(extendedPacketConn).SetReadBuffer(bytes)
}

// SyscallConn implements extendedPacketConn.
func (c *packetConn) SyscallConn() (syscall.RawConn, error) {
	return c.PacketConn.(extendedPacketConn).SyscallConn()
}
