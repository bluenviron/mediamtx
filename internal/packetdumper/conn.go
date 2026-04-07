// Package packetdumper provides utilities to dump packets to disk.
package packetdumper

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/google/uuid"
)

var _ net.Conn = (*conn)(nil)

type direction int

const (
	dirInbound direction = iota
	dirOutbound
	dirHandshake
	dirSecret
)

func writeDecryptionSecretsBlock(f io.Writer, data []byte) {
	const (
		dsbSecretTypeTLS = 0x544c534b
		blockType        = 0x0000000A
		fixedHeaderBytes = 4 + 4 + 4 + 4 // type + totalLen + secretsType + secretsLen
		trailerBytes     = 4             // repeated totalLen
		overheadBytes    = fixedHeaderBytes + trailerBytes
	)

	secretsLen := len(data)
	paddedLen := (secretsLen + 3) &^ 3
	padBytes := paddedLen - secretsLen

	totalLen := uint32(overheadBytes + paddedLen)

	buf := make([]byte, totalLen)
	pos := 0

	binary.LittleEndian.PutUint32(buf[pos:], blockType)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], totalLen)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], dsbSecretTypeTLS)
	pos += 4
	binary.LittleEndian.PutUint32(buf[pos:], uint32(secretsLen)) // unpadded length, per spec
	pos += 4
	pos += copy(buf[pos:], data)
	pos += padBytes                                    // zero padding already present (make zeroes)
	binary.LittleEndian.PutUint32(buf[pos:], totalLen) // trailing repeat

	f.Write(buf) //nolint:errcheck
}

type dumpEntry struct {
	ntp       time.Time
	data      []byte
	direction direction
}

// conn is a wrapper around net.Conn that dumps packets to disk.
type conn struct {
	Prefix     string
	Conn       net.Conn
	ServerSide bool

	expectingSecrets   int
	f                  *os.File
	pw                 *pcapgo.NgWriter
	once               sync.Once
	local              *net.TCPAddr
	remote             *net.TCPAddr
	nextLocalSequence  uint32
	nextRemoteSequence uint32
	delayed            []dumpEntry

	queue      chan dumpEntry
	terminated chan struct{}
	done       chan struct{}
}

// Initialize initializes conn.
func (c *conn) Initialize() error {
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

	c.local = c.Conn.LocalAddr().(*net.TCPAddr)
	c.remote = c.Conn.RemoteAddr().(*net.TCPAddr)

	if c.ServerSide {
		c.nextLocalSequence = uint32(2000)
		c.nextRemoteSequence = uint32(1000)
	} else {
		c.nextLocalSequence = uint32(1000)
		c.nextRemoteSequence = uint32(2000)
	}

	c.queue = make(chan dumpEntry, 64)
	c.terminated = make(chan struct{})
	c.done = make(chan struct{})

	go c.run()

	c.enqueue(dumpEntry{ntp: time.Now(), direction: dirHandshake})

	return nil
}

// Close implements net.Conn.
func (c *conn) Close() error {
	c.once.Do(func() {
		close(c.terminated)
	})
	<-c.done
	return c.Conn.Close()
}

func (c *conn) run() {
	defer close(c.done)
	defer c.f.Close()
	defer c.pw.Flush() //nolint:errcheck

	for {
		select {
		case e := <-c.queue:
			c.processEntry(e)

		case <-c.terminated:
			// Drain anything already in the queue before exiting.
			for {
				select {
				case e := <-c.queue:
					c.processEntry(e)
				default:
					return
				}
			}
		}
	}
}

func (c *conn) processEntry(e dumpEntry) {
	if c.expectingSecrets > 0 && e.direction != dirSecret {
		c.delayed = append(c.delayed, e)
		return
	}

	switch e.direction {
	case dirHandshake:
		clientAddr, serverAddr := c.local, c.remote // client side: local initiates
		clientSeq, serverSeq := &c.nextLocalSequence, &c.nextRemoteSequence
		if c.ServerSide {
			clientAddr, serverAddr = c.remote, c.local // server side: remote initiates
			clientSeq, serverSeq = &c.nextRemoteSequence, &c.nextLocalSequence
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

	case dirSecret:
		c.pw.Flush() //nolint:errcheck
		writeDecryptionSecretsBlock(c.f, e.data)

		c.expectingSecrets--
		if c.expectingSecrets == 0 {
			for _, e2 := range c.delayed {
				c.processEntry(e2)
			}
			c.delayed = nil
		}

	case dirInbound:
		tcpFlags := layers.TCP{
			PSH:    true,
			ACK:    true,
			Window: 14600,
			Seq:    c.nextRemoteSequence,
			Ack:    c.nextLocalSequence,
		}
		c.writePacket(e.ntp, c.remote, c.local, tcpFlags, e.data)
		c.nextRemoteSequence += uint32(len(e.data))

	case dirOutbound:
		tcpFlags := layers.TCP{
			PSH:    true,
			ACK:    true,
			Window: 14600,
			Seq:    c.nextLocalSequence,
			Ack:    c.nextRemoteSequence,
		}
		c.writePacket(e.ntp, c.local, c.remote, tcpFlags, e.data)
		c.nextLocalSequence += uint32(len(e.data))
	}
}

func (c *conn) writePacket(
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

func (c *conn) enqueue(e dumpEntry) {
	select {
	case c.queue <- e:
	case <-c.terminated:
	}
}

func (c *conn) Read(p []byte) (n int, err error) {
	n, err = c.Conn.Read(p)

	if n != 0 {
		c.enqueue(dumpEntry{
			ntp:       time.Now(),
			data:      append([]byte(nil), p[:n]...),
			direction: dirInbound,
		})
	}

	return n, err
}

func (c *conn) Write(p []byte) (n int, err error) {
	n, err = c.Conn.Write(p)

	if err == nil {
		c.enqueue(dumpEntry{
			ntp:       time.Now(),
			data:      append([]byte(nil), p...),
			direction: dirOutbound,
		})
	}

	return n, err
}

// LocalAddr implements net.Conn.
func (c *conn) LocalAddr() net.Addr { return c.Conn.LocalAddr() }

// RemoteAddr implements net.Conn.
func (c *conn) RemoteAddr() net.Addr { return c.Conn.RemoteAddr() }

// SetDeadline implements net.Conn.
func (c *conn) SetDeadline(t time.Time) error { return c.Conn.SetDeadline(t) }

// SetReadDeadline implements net.Conn.
func (c *conn) SetReadDeadline(t time.Time) error { return c.Conn.SetReadDeadline(t) }

// SetWriteDeadline implements net.Conn.
func (c *conn) SetWriteDeadline(t time.Time) error { return c.Conn.SetWriteDeadline(t) }
