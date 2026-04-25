# Web browsers

Web browsers can read a stream from the server with the WebRTC or the HLS protocol, by directly accessing web pages provided by the server. It is also possible to embed streams into an external website.

## Direct read

### WebRTC

Web browsers can read a stream with the [WebRTC protocol](02-webrtc.md) by visiting the web page:

```
http://localhost:8889/mystream
```

Replace `mystream` with the path name.

### HLS

Web browsers can also read a stream with the [HLS protocol](05-hls.md). Latency is higher but there are fewer problems related to connectivity between server and clients. Visit the web page:

```
http://localhost:8888/mystream
```

Replace `mystream` with the path name.

## Embed in a website

Live streams can be embedded into an external website by using the WebRTC or HLS protocol. Before embedding, check that the stream is ready and can be accessed with intended protocol by visiting web pages mentioned in the previous section.

### WebRTC in iframe

The simplest way to embed a live stream in a web page, using the WebRTC protocol, consists in adding an `<iframe>` tag to the body section of the HTML:

```html
<iframe src="http://mediamtx-ip:8889/mystream" scrolling="no"></iframe>
```

The iframe can be controlled by adding query parameters to the URL (example: `http://mediamtx-ip:8889/mystream?muted=false`). The following parameters are available:

- `controls` (boolean): whether to show controls. Default is true.
- `muted` (boolean): whether to start the stream muted. Default is true.
- `autoplay` (boolean): whether to autoplay the stream. Default is true.
- `playsInline` (boolean): whether to play the stream without using the entire window of mobile devices. Default is true.
- `disablepictureinpicture` (boolean): whether to disable the ability to open the stream in a dedicated window. Default is false.

The iframe method is fit for most use cases, but it has some limitations:

- it doesn't allow to pass credentials (username, password or token) from the website to _MediaMTX_; credentials are asked directly to users.
- it doesn't allow to directly access the video tag, to extract data from it, or to perform dynamic actions.

### WebRTC with JavaScript

In order to overcome the limitations of the iframe-based method, it is possible to load the stream directly inside a `<video>` tag in the web page, through a JavaScript library.

Download [reader.js](https://github.com/bluenviron/mediamtx/blob/{version_tag}/internal/servers/webrtc/reader.js) from the repository and serve it together with the other assets of the website.

If you are using a JavaScript bundler, you can import it by using:

```js
import "./reader.js";
```

Otherwise, you can add a `<script>` tag to the `<head>` section of the page:

```html
<script defer src="./reader.js"></script>
```

Add a `<video>` tag:

```html
<video id="myvideo" controls muted autoplay width="640" height="480"></video>
```

After the video tag, add a script that initializes the stream when the page is fully loaded:

```html
<script>
  let reader = null;

  window.addEventListener("load", () => {
    reader = new MediaMTXWebRTCReader({
      url: "http://mediamtx-ip:8889/mystream/whep",
      user: "", // fill if needed
      pass: "", // fill if needed
      token: "", // fill if needed
      onError: (err) => {
        console.error(err);
      },
      onTrack: (evt) => {
        document.getElementById("myvideo").srcObject = evt.streams[0];
      },
      onDataChannel: (evt) => {
        evt.channel.binaryType = "arraybuffer";
        evt.channel.onmessage = (evt) => {
          console.log("data channel message", evt.data);
        };
      },
    });
  });

  window.addEventListener("beforeunload", () => {
    if (reader !== null) {
      reader.close();
    }
  });
</script>
```

### HLS in iframe

Reading a stream with the HLS protocol introduces some latency, but is usually easier to setup since it doesn't involve managing additional ports that in WebRTC are used to transmit the stream.

The simplest way to embed a live stream in a web page, using the HLS protocol, consists in adding an `<iframe>` tag to the body section of the HTML:

```html
<iframe src="http://mediamtx-ip:8888/mystream" scrolling="no"></iframe>
```

The iframe can be controlled by adding query parameters to the URL (example: `http://mediamtx-ip:8888/mystream?muted=false`). The following parameters are available:

- `controls` (boolean): whether to show controls. Default is true.
- `muted` (boolean): whether to start the stream muted. Default is true.
- `autoplay` (boolean): whether to autoplay the stream. Default is true.
- `playsInline` (boolean): whether to play the stream without using the entire window of mobile devices. Default is true.
- `disablepictureinpicture` (boolean): whether to disable the ability to open the stream in a dedicated window. Default is false.

The iframe method is fit for most use cases, but it has some limitations:

- it doesn't allow to pass credentials (username, password or token) from the website to _MediaMTX_; credentials are asked directly to users.
- it doesn't allow to directly access the video tag, to extract data from it, or to perform dynamic actions.

### HLS with JavaScript

In order to overcome the limitations of the iframe-based method, it is possible to load the stream directly inside a `<video>` tag in the web page, through the _hls.js_ library.

If you are using a JavaScript bundler, you can import _hls.js_ by adding [its npm package](https://www.npmjs.com/package/hls.js) as dependency and then importing it:

```js
import Hls from "hls.js";
```

Otherwise, you can use a `<script>` tag inside the `<head>` section that points to a CDN:

```html
<script
  defer
  src="https://cdnjs.cloudflare.com/ajax/libs/hls.js/1.6.13/hls.min.js"
></script>
```

Add a `<video>` tag:

```html
<video id="myvideo" controls muted autoplay width="640" height="480"></video>
```

After the video tag, add a script that initializes the stream when the page is fully loaded:

```html
<script>
  window.addEventListener("load", () => {
    if (Hls.isSupported()) {
      const hls = new Hls({
        xhrSetup: function (xhr, url) {
          xhr.withCredentials = true;

          let user = ""; // fill if needed
          let pass = ""; // fill if needed
          let token = ""; // fill if needed

          if (user !== "") {
            const credentials = btoa(`${user}:${pass}`);
            xhr.setRequestHeader("Authorization", `Basic ${credentials}`);
          } else if (token !== "") {
            xhr.setRequestHeader("Authorization", `Bearer ${token}`);
          }
        },
      });

      hls.on(Hls.Events.MEDIA_ATTACHED, () => {
        hls.loadSource("http://mediamtx-ip:8888/mystream/index.m3u8");
      });

      hls.attachMedia(document.getElementById("myvideo"));
    }
  });
</script>
```
