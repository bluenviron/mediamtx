// Package quicp contains utilities for the QUIC protocol.
package quicp

// minInitialPacketSize is the minimum size of a QUIC packet (RFC 9000, section 14).
const minInitialPacketSize = 1200

// InitialPacketSize converts udpMaxPayloadSize into a value suitable for quic.Config.InitialPacketSize.
// 0 means default.
func InitialPacketSize(udpMaxPayloadSize int) uint16 {
	if udpMaxPayloadSize <= 0 {
		return 0
	}
	if udpMaxPayloadSize < minInitialPacketSize {
		return minInitialPacketSize
	}
	return uint16(udpMaxPayloadSize) //nolint:gosec
}
