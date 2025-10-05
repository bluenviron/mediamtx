# RTSP-specific features

RTSP is a protocol that can be used for publishing and reading streams. Regarding specific tasks, see [Publish](publish#rtsp-clients) and [Read](read#rtsp). Features in these page are shared among both tasks.

## Transport protocols

The RTSP protocol supports several underlying transport protocols, that are chosen by clients during the handshake with the server:

- UDP: the most performant, but doesn't work when there's a NAT/firewall between server and clients.
- UDP-multicast: allows to save bandwidth when clients are all in the same LAN, by sending packets once to a fixed multicast IP.
- TCP: the most versatile.

To change the transport protocol, you have to tune the configuration of the client you are using to publish or read streams. In most clients, the default transport protocol is UDP.

For instance, FFmpeg allows to change the transport protocol with the `-rtsp_transport` flag:

```sh
ffmpeg -rtsp_transport tcp -i rtsp://localhost:8554/mystream -c copy output.mp4
```

GStreamer allows to change the transport protocol with the `protocols` property of `rtspsrc` and `rtspclientsink`:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream protocols=tcp
```

VLC allows to use the TCP transport protocol through the `--rtsp_tcp` flag:

```sh
vlc --network-caching=50 --rtsp-tcp rtsp://localhost:8554/mystream
```

VLC allows to use the UDP-multicast transport protocol by appending `?vlcmulticast` to the URL:

```sh
vlc --network-caching=50 rtsp://localhost:8554/mystream?vlcmulticast
```

## Encryption

Incoming and outgoing RTSP streams can be encrypted by using a secure protocol variant, called RTSPS, that replaces all the subprotocols that are normally used in RTSP with their secure variant (TLS, SRTP, SRTCP). A TLS certificate is needed and can be generated with OpenSSL:

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

## Decreasing corrupted frames

In some scenarios, when publishing or reading from the server with RTSP, frames can get corrupted. This can be caused by several reasons:

- When the transport protocol is UDP (which is default one), packets sent to the server might get discarded because the UDP read buffer size is too small. This can be noticed in logs through the "RTP packets lost" message. Try increasing the UDP read buffer size:

  ```yml
  rtspUDPReadBufferSize: 1000000
  ```

  If the source of the stream is a camera:

  ```yml
  paths:
    test:
      source: rtsp://..
      rtspUDPReadBufferSize: 1000000
  ```

  Both these options require the `net.core.rmem_max` system parameter to be equal or greater than `rtspUDPReadBufferSize`:

  ```sh
  sudo sysctl net.core.rmem_max=100000000
  ```

- When the transport protocol is UDP (which is the default one), packets sent from the server to readers might get discarded because the write queue is too small. This can be noticed in logs through the "reader is too slow" message. Try increasing the write queue:

  ```yml
  writeQueueSize: 1024
  ```

- The stream is too big and it can't be transmitted correctly with the UDP transport protocol. UDP is more performant, faster and more efficient than TCP, but doesn't have a retransmission mechanism, that is needed in case of streams that need a large bandwidth. A solution consists in switching to TCP:

  ```yml
  rtspTransports: [tcp]
  ```

  In case the source is a camera:

  ```yml
  paths:
    test:
      source: rtsp://..
      rtspTransport: tcp
  ```

- The stream throughput is too big to be handled by the network between server and readers. Upgrade the network or decrease the stream bitrate by re-encoding it.
