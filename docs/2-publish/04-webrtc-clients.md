# WebRTC clients

WebRTC is an API that makes use of a set of protocols and methods to connect two clients together and allow them to exchange live media or data streams. You can publish a stream with WebRTC and a web browser by visiting:

```
http://localhost:8889/mystream/publish
```

The resulting stream will be available on path `/mystream`.

WHIP is a WebRTC extensions that allows to publish streams by using a URL, without passing through a web page. This allows to use WebRTC as a general purpose streaming protocol. If you are using a software that supports WHIP (for instance, latest versions of OBS Studio), you can publish a stream to the server by using this URL:

```
http://localhost:8889/mystream/whip
```

Be aware that not all browsers can read any codec, check [Supported browsers](../4-other/22-webrtc-specific-features.md#supported-browsers).

Depending on the network it might be difficult to establish a connection between server and clients, read [Solving WebRTC connectivity issues](../4-other/22-webrtc-specific-features.md#solving-webrtc-connectivity-issues).

Some clients that can publish with WebRTC and WHIP are [FFmpeg](15-ffmpeg.md), [GStreamer](16-gstreamer.md), [OBS Studio](17-obs-studio.md), [Unity](20-unity.md) and [Web browsers](21-web-browsers.md).
