package webrtc

import (
	"sync/atomic"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
)

type statsInterceptor struct {
	rtcpPacketsSent     atomic.Uint64
	rtcpPacketsReceived atomic.Uint64
}

func (*statsInterceptor) Close() error {
	return nil
}

func (s *statsInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(bytes []byte,
		attributes interceptor.Attributes,
	) (int, interceptor.Attributes, error) {
		n, attrs, err := reader.Read(bytes, attributes)

		pkts, err2 := attrs.GetRTCPPackets(bytes)
		if err2 == nil {
			s.rtcpPacketsReceived.Add(uint64(len(pkts)))
		}

		return n, attrs, err
	})
}

func (s *statsInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	return interceptor.RTCPWriterFunc(func(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
		s.rtcpPacketsSent.Add(uint64(len(pkts)))
		return writer.Write(pkts, attributes)
	})
}

func (s *statsInterceptor) BindLocalStream(_ *interceptor.StreamInfo,
	writer interceptor.RTPWriter,
) interceptor.RTPWriter {
	return writer
}

func (*statsInterceptor) UnbindLocalStream(_ *interceptor.StreamInfo) {}

func (s *statsInterceptor) BindRemoteStream(_ *interceptor.StreamInfo,
	reader interceptor.RTPReader,
) interceptor.RTPReader {
	return reader
}

func (*statsInterceptor) UnbindRemoteStream(_ *interceptor.StreamInfo) {}

type statsInterceptorFactory struct {
	onCreate func(s *statsInterceptor)
}

func (f *statsInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	s := &statsInterceptor{}

	f.onCreate(s)

	return s, nil
}
