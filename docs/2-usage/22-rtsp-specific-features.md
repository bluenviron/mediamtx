# RTSP-specific features

## Transport protocols

The RTSP protocol supports several underlying transport protocols, that are chosen by clients during the handshake with the server:

- UDP: the most performant, but doesn't work when there's a NAT/firewall between server and clients.
- UDP-multicast: allows to save bandwidth when clients are all in the same LAN, by sending packets once to a fixed multicast IP.
- TCP: the most versatile.

The default transport protocol is UDP. To change the transport protocol, you have to tune the configuration of your client of choice.

## Encryption

Incoming and outgoing RTSP streams can be encrypted with TLS, obtaining the RTSPS protocol. A TLS certificate is needed and can be generated with OpenSSL:

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

## Corrupted frames

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
