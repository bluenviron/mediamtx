# Web browsers

Web browsers can read a stream from the server in several ways.

## Web browsers and WebRTC

You can read a stream by using the [WebRTC protocol](03-webrtc.md) by visiting the web page:

```
http://localhost:8889/mystream
```

See [Embed streams in a website](../4-other/14-embed-streams-in-a-website.md) for instructions on how to embed the stream into an external website.

## Web browsers and HLS

Web browsers can also read a stream with the [HLS protocol](06-hls.md). Latency is higher but there are less problems related to connectivity between server and clients, furthermore the server load can be balanced by using a common HTTP CDN (like Cloudflare or CloudFront), and this allows to handle an unlimited amount of readers. Visit the web page:

```
http://localhost:8888/mystream
```

See [Embed streams in a website](../4-other/14-embed-streams-in-a-website.md) for instructions on how to embed the stream into an external website.
