# WebRTC

WebRTC is an API that makes use of a set of protocols and methods to connect two clients together and allow them to exchange live media or data streams. You can read a stream with WebRTC and a web browser by visiting:

```
http://localhost:8889/mystream
```

WHEP is a WebRTC extensions that allows to read streams by using a URL, without passing through a web page. This allows to use WebRTC as a general purpose streaming protocol. If you are using a software that supports WHEP, you can read a stream from the server by using this URL:

```
http://localhost:8889/mystream/whep
```

Be aware that not all browsers can read any codec, check [Supported browsers](../4-other/22-webrtc-specific-features.md#supported-browsers).

Depending on the network it may be difficult to establish a connection between server and clients, read [Solving WebRTC connectivity issues](../4-other/22-webrtc-specific-features.md#solving-webrtc-connectivity-issues).

Some clients that can read with WebRTC and WHEP are [FFmpeg](07-ffmpeg.md), [GStreamer](08-gstreamer.md), [Unity](11-unity.md) and [web browsers](12-web-browsers.md).
