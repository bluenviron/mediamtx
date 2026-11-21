# RTMP-specific features

RTMP is a protocol that can be used for publishing and reading streams. Regarding specific tasks, check out [Publish with RTMP clients](../3-publish/09-rtmp-clients.md) and [Read with RTMP clients](../4-read/05-rtmp.md). Features in this page are shared among both tasks.

## Encryption

RTMP connections can be encrypted by using the secure protocol variant (RTMPS). A TLS certificate is needed and can be generated with OpenSSL:

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

Be aware that RTMPS is currently unsupported by most major players. However, you can use a local _MediaMTX_ instance to decrypt streams before reading them, or alternatively a proxy like [stunnel](https://www.stunnel.org) or [nginx](https://nginx.org/). For instance, you can launch a local _MediaMTX_ instance with this configuration:

```yml
paths:
  decrypted:
    source: rtmps://original-stream
```

And then read `rtmp://localhost/decrypted` instead of `rtmps://original-stream`.

## Multiple certificates

When the server is reachable under several hostnames that need different certificates, additional certificates can be listed in `rtmpServerCerts`. During the TLS handshake, the server picks the first certificate that is valid for the hostname requested by the client (TLS SNI), starting with the one in `rtmpServerKey` / `rtmpServerCert`. Clients that don't send a hostname, or request a hostname that no certificate covers, receive the default one:

```yml
rtmpEncryption: optional
# default certificate
rtmpServerKey: internal.key
rtmpServerCert: internal.crt
# additional certificates
rtmpServerCerts:
  - key: public.key
    cert: public.crt
```

If `rtmpServerKey` and `rtmpServerCert` are set to empty strings, the first entry of `rtmpServerCerts` becomes the default certificate:

```yml
rtmpServerKey: ''
rtmpServerCert: ''
rtmpServerCerts:
  - key: a.key
    cert: a.crt
  - key: b.key
    cert: b.crt
```

Additional certificates are reloaded when their files change, like the default one. The same setting is available for every server that supports encryption: `rtspServerCerts`, `hlsServerCerts`, `webrtcServerCerts`, `moqServerCerts`, `apiServerCerts`, `metricsServerCerts`, `pprofServerCerts`, `playbackServerCerts`.
