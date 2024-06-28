package gstpipe

import "github.com/bluenviron/mediamtx/internal/defs"

type rtpSourceStats struct {
	// Estimated amount of packets lost
	packetsLost int
	// Total number of packets received
	packetsReceived uint64
	// Bitrate in bits per second
	bitrate uint64
	// Estimated jitter (in clock rate units)
	jitter uint
}

type jitterBufferStats struct {
	// The number of packets considered lost
	numLost uint64
	// The number of packets arriving too late
	numLate uint64
	// The number of duplicate packets
	numDuplicates uint64
	// The average jitter in nanoseconds
	avgJitter uint64
	// The number of retransmissions requested
	rtxCount uint64
	// The number of successful retransmissions
	rtxSuccessCount uint64
	// Average number of RTX per packet
	rtxPerPacket float64
	// Average round trip time per RTX
	rtxRtt uint64
}

type rtpSessionStats struct {
	// The number of retransmission events dropped (due to bandwidth constraints)
	rtxDropCount uint64
	// Number of NACKs sent
	sentNackCount uint64
	// Number of NACKs received
	recvNackCount uint64
}

type gstPipeStat struct {
	path            string
	active          bool
	jitterStats     jitterBufferStats
	rtpSourceStats  rtpSourceStats
	rtpSessionStats rtpSessionStats
}

func (c *gstPipeStat) apiItem() *defs.APIGstPipe {

	apiJitterStat := &defs.APIGstJitterBufferStats{
		NumLost:         c.jitterStats.numLost,
		NumLate:         c.jitterStats.numLate,
		NumDuplicates:   c.jitterStats.numDuplicates,
		AvgJitter:       c.jitterStats.avgJitter,
		RtxCount:        c.jitterStats.rtxCount,
		RtxSuccessCount: c.jitterStats.rtxSuccessCount,
		RtxPerPacket:    c.jitterStats.rtxPerPacket,
		RtxRtt:          c.jitterStats.rtxRtt,
	}

	rtpSourceStats := &defs.APIGstRtpSourceStats{
		PacketsLost:     c.rtpSourceStats.packetsLost,
		PacketsReceived: c.rtpSourceStats.packetsReceived,
		Bitrate:         c.rtpSourceStats.bitrate,
		Jitter:          c.rtpSourceStats.jitter,
	}

	rtpSessionStats := &defs.APIGstRtpSessionStats{
		RtxDropCount:  c.rtpSessionStats.rtxDropCount,
		SentNackCount: c.rtpSessionStats.sentNackCount,
		RecvNackCount: c.rtpSessionStats.recvNackCount,
	}

	return &defs.APIGstPipe{
		Path:            c.path,
		Active:          c.active,
		JitterStats:     apiJitterStat,
		RtpSourceStats:  rtpSourceStats,
		RtpSessionStats: rtpSessionStats,
	}
}
