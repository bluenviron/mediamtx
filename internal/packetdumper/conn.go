// Package packetdumper provides utilities to dump packets to disk.
package packetdumper

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/google/uuid"
)

var _ net.Conn = (*Conn)(nil)

type direction int

const (
	dirRead direction = iota
	dirWrite
	dirHandshake
)

type dumpEntry struct {
	ntp       time.Time
	data      []byte
	direction direction
}

// Conn is a wrapper around net.Conn that dumps packets to disk.
type Conn struct {
	Prefix     string
	Conn       net.Conn
	ServerSide bool

	f    *os.File
	pw   *pcapgo.NgWriter
	once sync.Once

	queue      chan dumpEntry
	terminated chan struct{}
	done       chan struct{}
}

// Initialize initializes Conn.
func (c *Conn) Initialize() error {
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

	c.queue = make(chan dumpEntry, 64)
	c.terminated = make(chan struct{})
	c.done = make(chan struct{})

	go c.run()

	c.enqueue(dumpEntry{ntp: time.Now(), direction: dirHandshake})

	return nil
}

// Close implements net.Conn.
func (c *Conn) Close() error {
	c.once.Do(func() {
		close(c.terminated)
	})
	<-c.done
	return c.Conn.Close()
}

func (c *Conn) run() {
	defer close(c.done)
	defer c.f.Close()

	local := c.Conn.LocalAddr().(*net.TCPAddr)
	remote := c.Conn.RemoteAddr().(*net.TCPAddr)

	nextLocalSequence := uint32(1000)
	nextRemoteSequence := uint32(2000)

	for {
		select {
		case e := <-c.queue:
			c.processEntry(e, local, remote, &nextLocalSequence, &nextRemoteSequence)

		case <-c.terminated:
			// Drain anything already in the queue before exiting.
			for {
				select {
				case e := <-c.queue:
					c.processEntry(e, local, remote, &nextLocalSequence, &nextRemoteSequence)
				default:
					c.pw.Flush() //nolint:errcheck
					return
				}
			}
		}
	}
}

func (c *Conn) processEntry(
	e dumpEntry,
	local, remote *net.TCPAddr,
	nextLocalSequence, nextRemoteSequence *uint32,
) {
	switch e.direction {
	case dirHandshake:
		clientAddr, serverAddr := local, remote // client side: local initiates
		clientSeq, serverSeq := nextLocalSequence, nextRemoteSequence
		if c.ServerSide {
			clientAddr, serverAddr = remote, local // server side: remote initiated
			clientSeq, serverSeq = nextRemoteSequence, nextLocalSequence
		}

		// SYN (client -> server)
		c.writePacket(e.ntp, clientAddr, serverAddr,
			layers.TCP{SYN: true, Window: 65535, Seq: *clientSeq, Ack: 0}, nil)
		*clientSeq++

		// SYN-ACK (server -> client)
		c.writePacket(e.ntp, serverAddr, clientAddr,
			layers.TCP{SYN: true, ACK: true, Window: 65535, Seq: *serverSeq, Ack: *clientSeq}, nil)
		*serverSeq++

		// ACK (client -> server)
		c.writePacket(e.ntp, clientAddr, serverAddr,
			layers.TCP{ACK: true, Window: 65535, Seq: *clientSeq, Ack: *serverSeq}, nil)

	case dirRead:
		tcpFlags := layers.TCP{
			PSH:    true,
			ACK:    true,
			Window: 14600,
			Seq:    *nextRemoteSequence,
			Ack:    *nextLocalSequence,
		}
		c.writePacket(e.ntp, remote, local, tcpFlags, e.data)
		*nextRemoteSequence += uint32(len(e.data))

	case dirWrite:
		tcpFlags := layers.TCP{
			PSH:    true,
			ACK:    true,
			Window: 14600,
			Seq:    *nextLocalSequence,
			Ack:    *nextRemoteSequence,
		}
		c.writePacket(e.ntp, local, remote, tcpFlags, e.data)
		*nextLocalSequence += uint32(len(e.data))
	}
}

func (c *Conn) writePacket(
	ntp time.Time,
	src, dst *net.TCPAddr,
	tcpFlags layers.TCP,
	payload []byte,
) {
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
		DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 0},
		EthernetType: layers.EthernetTypeIPv6,
	}

	ipv6 := &layers.IPv6{
		Version:    6,
		SrcIP:      src.IP.To16(),
		DstIP:      dst.IP.To16(),
		NextHeader: layers.IPProtocolTCP,
		HopLimit:   64,
	}

	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(src.Port),
		DstPort: layers.TCPPort(dst.Port),
		Seq:     tcpFlags.Seq,
		Ack:     tcpFlags.Ack,
		Window:  tcpFlags.Window,
		SYN:     tcpFlags.SYN,
		ACK:     tcpFlags.ACK,
		PSH:     tcpFlags.PSH,
		FIN:     tcpFlags.FIN,
	}
	tcp.SetNetworkLayerForChecksum(ipv6) //nolint:errcheck

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	gopacket.SerializeLayers(buf, opts, eth, ipv6, tcp, gopacket.Payload(payload)) //nolint:errcheck

	raw := buf.Bytes()

	c.pw.WritePacket(gopacket.CaptureInfo{ //nolint:errcheck
		Timestamp:     ntp,
		CaptureLength: len(raw),
		Length:        len(raw),
	}, raw)
}

func (c *Conn) enqueue(e dumpEntry) {
	select {
	case c.queue <- e:
	case <-c.terminated:
	}
}

func (c *Conn) Read(p []byte) (n int, err error) {
	n, err = c.Conn.Read(p)

	if n != 0 {
		c.enqueue(dumpEntry{
			ntp:       time.Now(),
			data:      append([]byte(nil), p[:n]...),
			direction: dirRead,
		})
	}

	return n, err
}

func (c *Conn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)

	if err == nil {
		c.enqueue(dumpEntry{
			ntp:       time.Now(),
			data:      append([]byte(nil), p...),
			direction: dirWrite,
		})
	}

	return n, err
}

// LocalAddr implements net.Conn.
func (c *Conn) LocalAddr() net.Addr { return c.Conn.LocalAddr() }

// RemoteAddr implements net.Conn.
func (c *Conn) RemoteAddr() net.Addr { return c.Conn.RemoteAddr() }

// SetDeadline implements net.Conn.
func (c *Conn) SetDeadline(t time.Time) error { return c.Conn.SetDeadline(t) }

// SetReadDeadline implements net.Conn.
func (c *Conn) SetReadDeadline(t time.Time) error { return c.Conn.SetReadDeadline(t) }

// SetWriteDeadline implements net.Conn.
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.Conn.SetWriteDeadline(t) }
