# RTSP cameras and servers

|           | supported codecs                                                                          |
| --------- | ----------------------------------------------------------------------------------------- |
| **video** | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, MJPEG               |
| **audio** | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM |
| **other** | KLV, MPEG-TS, any RTP-compatible codec                                                    |

Most IP cameras expose their video stream by using a RTSP server that is embedded into the camera itself. In particular, cameras that are compliant with ONVIF profile S or T meet this requirement. You can use _MediaMTX_ to connect to one or several existing RTSP servers and read their media streams:

```yml
paths:
  proxied:
    source: rtsp://user:pass@host:port/path
```

If username or password contain special characters (like ?, :, etc), they need to be [url-encoded](https://www.urlencoder.org/).

The resulting stream will be available on path `/proxied`.

It is possible to tune the connection by using several additional parameters, that are listed in the [configuration file](../5-references/1-configuration-file.md).

Advanced RTSP features and settings are described in [RTSP-specific features](../2-features/27-rtsp-specific-features.md).
