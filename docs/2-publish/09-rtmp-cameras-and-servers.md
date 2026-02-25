# RTMP cameras and servers

You can use _MediaMTX_ to connect to one or several existing RTMP servers and read their media streams:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtmp://user:pass@host:port/path
    source: rtmp://original-url
```

The resulting stream will be available on path `/proxied`.
