# RTMP-specific features

RTMP is a protocol that can be used for publishing and reading streams. Features in these page are shared among both tasks. Regarding specific tasks, see [Publish](publish) and [Read](read).

## Encryption

RTMP connections can be encrypted with TLS, obtaining the RTMPS protocol. A TLS certificate is needed and can be generated with OpenSSL:

```yml
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Edit mediamtx.yml and set the `rtmpEncryption`, `rtmpServerKey` and `rtmpServerCert` parameters:

```yml
rtmpEncryption: optional
rtmpServerKey: server.key
rtmpServerCert: server.crt
```

Streams can be published and read with the rtmps scheme and the 1937 port:

```
rtmps://localhost:1937/...
```

Be aware that RTMPS is currently unsupported by all major players. However, you can use a local _MediaMTX_ instance to decrypt streams before reading them, or alternatively a proxy like [stunnel](https://www.stunnel.org) or [nginx](https://nginx.org/). For instance, you can launch a local MediaMTX instance with this configuration:

```yml
paths:
  decrypted:
    source: rtmps://original-stream
```

And then read `rtmp://localhost/decrypted` instead of `rtmps://original-stream`.
