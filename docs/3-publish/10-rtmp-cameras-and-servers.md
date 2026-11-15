# RTMP cameras and servers

|           | supported codecs                                                                    |
| --------- | ----------------------------------------------------------------------------------- |
| **video** | AV1, VP9, H265, H264                                                                |
| **audio** | Opus, FLAC, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM |

You can use _MediaMTX_ to connect to one or several existing RTMP servers and read their media streams:

```yml
paths:
  proxied:
    # Use rtmp:// for plain RTMP and rtmps:// for encrypted RTMP.
    source: rtmp://user:pass@host:port/path#streamKey
    # If the source is RTMPS and the source TLS certificate is self-signed
    # or invalid, you can provide the fingerprint of the certificate in order to
    # validate it anyway. It can be obtained by running:
    # openssl s_client -connect source_ip:source_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
    # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
    sourceFingerprint:
```

If username or password contain special characters (like ?, :, etc), they need to be [url-encoded](https://www.urlencoder.org/).

If host is a stateless IPv6 address, wrap the address in square brackets and write the zone separator `%` as `%25`, for example `rtmp://[fe80::1234:5678:9abc:def0%25eth0]:1935/path`.

The resulting stream will be available on path `/proxied`.
