# Web browsers

Web browsers can publish a stream to the server by acting as a [Media-over-QUIC client](01-moq-clients.md) or a [WebRTC client](05-webrtc-clients.md).

## Media-over-QUIC

Start the server and open the web page:

```
https://localhost:8892/mystream/publish
```

The resulting stream will be available on path `/mystream`.

This web page can be embedded into another web page by using an iframe:

```html
<iframe src="https://mediamtx-ip:8892/mystream/publish" scrolling="no"></iframe>
```

## WebRTC

Start the server and open the web page:

```
http://localhost:8889/mystream/publish
```

The resulting stream will be available on path `/mystream`.

This web page can be embedded into another web page by using an iframe:

```html
<iframe src="http://mediamtx-ip:8889/mystream/publish" scrolling="no"></iframe>
```

For more advanced setups, you can create and serve a custom web page by starting from the [source code of the WebRTC publish page](https://github.com/bluenviron/mediamtx/blob/{version_tag}/internal/servers/webrtc/publish_index.html). In particular, there's a ready-to-use, standalone JavaScript class for publishing streams with WebRTC, available in [publisher.js](https://github.com/bluenviron/mediamtx/blob/{version_tag}/internal/servers/webrtc/publisher.js).
