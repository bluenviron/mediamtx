# HLS

|           | supported codecs             |
| --------- | ---------------------------- |
| **video** | AV1, VP9, H265, H264         |
| **audio** | Opus, MPEG-4 Audio (AAC)     |
| **other** | KLV (MPEG-TS-based HLS only) |

HLS is a protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to generate an HLS stream, that is accessible through a web page:

```
http://localhost:8888/mystream
```

and can also be accessed without using the browsers, by software that supports the HLS protocol (for instance VLC or _MediaMTX_ itself) by using this URL:

```
http://localhost:8888/mystream/index.m3u8
```

Some clients that can read with HLS are [FFmpeg](06-ffmpeg.md), [GStreamer](07-gstreamer.md), [VLC](08-vlc.md) and [web browsers](13-web-browsers.md).

_MediaMTX_ supports generating HLS in several variants (including Low-Latency mode), and provides various parameters to tune HLS generation. These are listed in the [configuration file](../5-references/1-configuration-file.md):

```yml
# Allow clients to read streams with the HLS protocol.
hls: true
# Address of the HLS listener.
hlsAddress: :8888
# Enable HTTPS on the HLS server.
# This is required for Low-Latency HLS to function correctly on Apple devices.
hlsEncryption: false
# Path to the server key. This is needed only when encryption is yes.
# This can be generated with:
# openssl genrsa -out server.key 2048
# openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
hlsServerKey: server.key
# Path to the server certificate.
hlsServerCert: server.crt
# Allowed CORS origins.
# Supports wildcards: ['http://*.example.com']
hlsAllowOrigins: ["*"]
# IPs or CIDRs of proxies placed before the HLS server.
# If the server receives a request from one of these entries, IP in logs
# will be taken from the X-Forwarded-For header.
hlsTrustedProxies: []
# By default, HLS is generated only when requested by a user.
# This option allows to generate it always, avoiding the delay between request and generation.
hlsAlwaysRemux: false
# Variant of the HLS protocol to use. Available options are:
# * mpegts - uses MPEG-TS segments, for maximum compatibility.
# * fmp4 - uses fragmented MP4 segments, more efficient.
# * lowLatency - uses Low-Latency HLS.
hlsVariant: lowLatency
# Number of HLS segments to keep on the server.
# Segments allow to seek through the stream.
# Their number doesn't influence latency.
hlsSegmentCount: 7
# Minimum duration of each segment.
# A player usually puts 3 segments in a buffer before reproducing the stream.
# The final segment duration is also influenced by the interval between IDR frames,
# since the server changes the duration in order to include at least one IDR frame
# in each segment.
hlsSegmentDuration: 1s
# Minimum duration of each part.
# A player usually puts 3 parts in a buffer before reproducing the stream.
# Parts are used in Low-Latency HLS in place of segments.
# Part duration is influenced by the distance between video/audio samples
# and is adjusted in order to produce segments with a similar duration.
hlsPartDuration: 200ms
# Maximum size of each segment.
# This prevents RAM exhaustion.
hlsSegmentMaxSize: 50M
# Directory in which to save segments and non-low-latency playlists.
# This has two purposes: offloading RAM and creating a self-consistent directory
# that can be served by a CDN.
hlsDirectory: ""
# The muxer will be closed when there are no
# reader requests and this amount of time has passed.
hlsMuxerCloseAfter: 60s
# Secret to identify requests coming from a CDN.
# The CDN must insert this secret in every request in the
# 'Authorization: Bearer' header.
hlsCDNSecret: ""
```

HLS can also be used to [scale the server](../2-features/20-scalability.md) through a CDN.
