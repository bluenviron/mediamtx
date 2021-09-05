package aac

import (
	"fmt"
)

const (
	mpegAudioTypeAACLLC = 2
)

var sampleRates = []int{
	96000,
	88200,
	64000,
	48000,
	44100,
	32000,
	24000,
	22050,
	16000,
	12000,
	11025,
	8000,
	7350,
}

var channelCounts = []int{
	1,
	2,
	3,
	4,
	5,
	6,
	8,
}

// ADTSPacket is an ADTS packet
type ADTSPacket struct {
	Type         int
	SampleRate   int
	ChannelCount int
	AU           []byte
}

// DecodeADTS decodes an ADTS stream into ADTS packets.
func DecodeADTS(byts []byte) ([]*ADTSPacket, error) {
	// refs: https://wiki.multimedia.cx/index.php/ADTS

	var ret []*ADTSPacket

	for len(byts) > 0 {
		syncWord := (uint16(byts[0]) << 4) | (uint16(byts[1]) >> 4)
		if syncWord != 0xfff {
			return nil, fmt.Errorf("invalid syncword")
		}

		protectionAbsent := byts[1] & 0x01
		if protectionAbsent != 1 {
			return nil, fmt.Errorf("CRC is not supported")
		}

		pkt := &ADTSPacket{}

		pkt.Type = int((byts[2] >> 6) + 1)

		switch pkt.Type {
		case mpegAudioTypeAACLLC:

		default:
			return nil, fmt.Errorf("unsupported object type: %d", pkt.Type)
		}

		sampleRateIndex := (byts[2] >> 2) & 0x0F

		switch {
		case sampleRateIndex <= 12:
			pkt.SampleRate = sampleRates[sampleRateIndex]

		default:
			return nil, fmt.Errorf("invalid sample rate index: %d", sampleRateIndex)
		}

		channelConfig := ((byts[2] & 0x01) << 2) | ((byts[3] >> 6) & 0x03)

		switch {
		case channelConfig >= 1 && channelConfig <= 7:
			pkt.ChannelCount = channelCounts[channelConfig-1]

		default:
			return nil, fmt.Errorf("invalid channel configuration: %d", channelConfig)
		}

		frameLen := int(((uint16(byts[3])&0x03)<<11)|
			(uint16(byts[4])<<3)|
			((uint16(byts[5])>>5)&0x07)) - 7

		// fullness := ((uint16(byts[5]) & 0x1F) << 6) | ((uint16(byts[6]) >> 2) & 0x3F)

		frameCount := byts[6] & 0x03
		if frameCount != 0 {
			return nil, fmt.Errorf("multiple frame count not supported")
		}

		if len(byts[7:]) < frameLen {
			return nil, fmt.Errorf("invalid frame length")
		}

		pkt.AU = byts[7 : 7+frameLen]
		byts = byts[7+frameLen:]

		ret = append(ret, pkt)
	}

	return ret, nil
}

// EncodeADTS encodes ADTS packets into an ADTS stream.
func EncodeADTS(pkts []*ADTSPacket) ([]byte, error) {
	var ret []byte

	for _, pkt := range pkts {
		sampleRateIndex := func() int {
			for i, s := range sampleRates {
				if s == pkt.SampleRate {
					return i
				}
			}
			return -1
		}()

		if sampleRateIndex == -1 {
			return nil, fmt.Errorf("invalid sample rate: %d", pkt.SampleRate)
		}

		channelConfig := func() int {
			for i, co := range channelCounts {
				if co == pkt.ChannelCount {
					return i + 1
				}
			}
			return -1
		}()

		if channelConfig == -1 {
			return nil, fmt.Errorf("invalid channel count: %d", pkt.ChannelCount)
		}

		frameLen := len(pkt.AU) + 7

		fullness := 0x07FF // like ffmpeg does

		header := make([]byte, 7)
		header[0] = 0xFF
		header[1] = 0xF1
		header[2] = uint8(((pkt.Type - 1) << 6) | (sampleRateIndex << 2) | ((channelConfig >> 2) & 0x01))
		header[3] = uint8((channelConfig&0x03)<<6 | (frameLen>>11)&0x03)
		header[4] = uint8((frameLen >> 3) & 0xFF)
		header[5] = uint8((frameLen&0x07)<<5 | ((fullness >> 6) & 0x1F))
		header[6] = uint8((fullness & 0x3F) << 2)
		ret = append(ret, header...)

		ret = append(ret, pkt.AU...)
	}

	return ret, nil
}
