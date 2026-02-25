# RTSP cameras and servers

Most IP cameras expose their video stream by using a RTSP server that is embedded into the camera itself. In particular, cameras that are compliant with ONVIF profile S or T meet this requirement. You can use _MediaMTX_ to connect to one or several existing RTSP servers and read their media streams:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
```

The resulting stream will be available on path `/proxied`.

It is possible to tune the connection by using some additional parameters:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
    # Transport protocol used to pull the stream. available values are "automatic", "udp", "multicast", "tcp".
    rtspTransport: automatic
    # Support sources that don't provide server ports or use random server ports. This is a security issue
    # and must be used only when interacting with sources that require it.
    rtspAnyPort: no
    # Range header to send to the source, in order to start streaming from the specified offset.
    # available values:
    # * clock: Absolute time
    # * npt: Normal Play Time
    # * smpte: SMPTE timestamps relative to the start of the recording
    rtspRangeType:
    # Available values:
    # * clock: UTC ISO 8601 combined date and time string, e.g. 20230812T120000Z
    # * npt: duration such as "300ms", "1.5m" or "2h45m", valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"
    # * smpte: duration such as "300ms", "1.5m" or "2h45m", valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"
    rtspRangeStart:
    # Size of the UDP buffer of the RTSP client.
    # This can be increased to mitigate packet losses.
    # It defaults to the default value of the operating system.
    rtspUDPReadBufferSize: 0
    # Range of ports used as source port in outgoing UDP packets.
    rtspUDPSourcePortRange: [10000, 65535]
```

All available parameters are listed in the [configuration file](../5-references/1-configuration-file.md).

Advanced RTSP features are described in [RTSP-specific features](../4-other/23-rtsp-specific-features.md).
