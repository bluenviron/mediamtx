# RTSP-specific features

RTSP is a protocol that can be used for publishing and reading streams. Regarding specific tasks, see [Publish](publish#rtsp-clients) and [Read](read#rtsp). Features in these page are shared among both tasks.

## Transport protocols

A RTSP session is splitted in two parts: the handshake, which is always performed with the TCP protocol, and data streaming, which can be performed with an arbitrary underlying transport protocol, which is chosen by the client during the handshake:

- UDP: the most performant, but require clients to access two additional UDP ports on the server, which is often impossible due to blocking or remapping by NATs/firewalls in between.
- UDP-multicast: allows to save bandwidth when clients are all in the same LAN, by sending packets once to a fixed multicast IP.
- TCP: the most versatile.

To change the transport protocol, you have to tune the configuration of the client you are using to publish or read streams. In most clients, the default transport protocol is UDP.

FFmpeg allows to change the transport protocol with the `-rtsp_transport` flag:

```sh
ffmpeg -rtsp_transport tcp -i rtsp://localhost:8554/mystream -c copy output.mp4
```

Available options are:

- `-rtsp_transport tcp` to pick the TCP transport protocol
- `-rtsp_transport udp` to pick the UDP transport protocol
- `-rtsp_transport udp_multicast` to pick the UDP-multicast transport protocol

GStreamer allows to change the transport protocol with the `protocols` property of `rtspsrc` and `rtspclientsink`:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream protocols=tcp
```

Available options are:

- `protocols=tcp` to pick the TCP transport protocol
- `protocols=udp` to pick the UDP transport protocol
- `protocols=udp-mcast` to pick the UDP-multicast transport protocol

VLC allows to use the TCP transport protocol through the `--rtsp_tcp` flag:

```sh
vlc --network-caching=50 --rtsp-tcp rtsp://localhost:8554/mystream
```

VLC allows to use the UDP-multicast transport protocol by appending `?vlcmulticast` to the URL:

```sh
vlc --network-caching=50 rtsp://localhost:8554/mystream?vlcmulticast
```

## Encryption

Incoming and outgoing RTSP streams can be encrypted by replacing all the subprotocols that are normally used in RTSP with their secure variants (RTSPS, SRTP, SRTCP). A TLS certificate is needed and can be generated with OpenSSL:

```sh
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Edit `mediamtx.yml` and set the `encryption`, `serverKey` and serverCert parameters:

```yml
rtspEncryption: optional
rtspServerKey: server.key
rtspServerCert: server.crt
```

Streams can be published and read with the `rtsps` scheme and the `8322` port:

```
rtsps://localhost:8322/mystream
```

Some clients require additional flags for encryption to work properly.

When reading with GStreamer, set set `tls-validation-flags` to `0`:

```sh
gst-launch-1.0 rtspsrc tls-validation-flags=0 location=rtsps://ip:8322/...
```

When publishing with GStreamer, set `tls-validation-flags` to `0` and `profiles` to `GST_RTSP_PROFILE_SAVP`:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream tls-validation-flags=0 profiles=GST_RTSP_PROFILE_SAVP
```

## Tunneling

In environments where HTTP is the only protocol available for exposing services (for instance, when there are mandatory API gateways or strict firewalls), the RTSP protocol can be tunneled inside HTTP. There are two standardized HTTP tunneling variants:

- RTSP over WebSocket: more efficient, requires WebSocket support from the gateway / firewall
- RTSP over HTTP: older variant, should work even in extreme cases

_MediaMTX_ is automatically able to handle incoming HTTP tunneled connections, without any configuration required.

In order to read a RTSP from an external server using HTTP tunneling, you can use the `rtsp+http` scheme:

```yml
paths:
  source: rtsp+http://standard-rtsp-url
```

There are also the `rtsp+https`, `rtsp+ws`, `rtsp+wss` schemes to handle any variant.
