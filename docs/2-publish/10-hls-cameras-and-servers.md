# HLS cameras and servers

HLS is a streaming protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to connect to one or several existing HLS servers and read their media streams:

```yml
paths:
  proxied:
    # url of the playlist of the stream, in the format http://user:pass@host:port/path
    source: http://original-url/stream/index.m3u8
```

The resulting stream will be available on path `/proxied`.
