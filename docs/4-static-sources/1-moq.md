# Media-over-QUIC servers

|           | supported codecs          |
| --------- | ------------------------- |
| **video** | AV1, VP9, VP8, H265, H264 |
| **audio** | Opus, MPEG-4 Audio (AAC)  |

In order to ingest a Media-over-QUIC stream from a remote server, add the corresponding URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    source: moqt://user:pass@host:port/path
    # Transport protocol used to pull the stream. available values are "quic", "webtransport".
    moqTransport: quic
```

If username or password contain special characters (like ?, :, etc), they need to be [url-encoded](https://www.urlencoder.org/).

If host is a stateless IPv6 address, wrap the address in square brackets and write the zone separator `%` as `%25`, for example `moqt://[fe80::1234:5678:9abc:def0%25eth0]:8892/path`.

Only username and password can be encoded into the URL; they are sent to the remote server inside the `AUTHORIZATION_TOKEN` parameter of the `SUBSCRIBE` message. If the remote server expects a token or a JWT, pass it as password, with an arbitrary user:

```yml
paths:
  proxied:
    source: moqt://user:mytoken@host:port/path
```

If the connection to the remote server times out during the QUIC handshake, the network might have a low MTU (for instance, some VMs and VPNs). Decrease the size of the first QUIC packets with the global `moqInitialPacketSize` parameter:

```yml
moqInitialPacketSize: 1200
```
