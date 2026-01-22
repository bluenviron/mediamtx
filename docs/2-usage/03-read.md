# Read a stream

## Compatibility matrix

Live streams can be read from the server with the following protocols and codecs:

| protocol          | variants                                   | codecs                                                                                                                                                                                                                                                 |
| ----------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [SRT](#srt)       |                                            | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [WebRTC](#webrtc) | WHEP                                       | **Video**: AV1, VP9, VP8, H265, H264<br/>**Audio**: Opus, G722, G711 (PCMA, PCMU)<br/>**Other**: KLV                                                                                                                                                   |
| [RTSP](#rtsp)     | UDP, UDP-Multicast, TCP, RTSPS             | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec |
| [RTMP](#rtmp)     | RTMP, RTMPS, Enhanced RTMP                 | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                                                                                                           |
| [HLS](#hls)       | Low-Latency HLS, MP4-based HLS, legacy HLS | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC)                                                                                                                                                                                |

We provide instructions for reading with the following software:

- [FFmpeg](#ffmpeg)
- [GStreamer](#gstreamer)
- [VLC](#vlc)
- [OBS Studio](#obs-studio)
- [Unity](#unity)
- [Web browsers](#web-browsers)

## Protocols

### SRT

SRT is a protocol that allows to publish and read live data stream, providing encryption, integrity and a retransmission mechanism. It is usually used to transfer media streams encoded with MPEG-TS. In order to read a stream from the server with the SRT protocol, use this URL:

```
srt://localhost:8890?streamid=read:mystream
```

Replace `mystream` with the path name.

If you need to use the standard stream ID syntax instead of the custom one in use by this server, see [Standard stream ID syntax](srt-specific-features#standard-stream-id-syntax).

Some clients that can read with SRT are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer) and [VLC](#vlc).

### WebRTC

WebRTC is an API that makes use of a set of protocols and methods to connect two clients together and allow them to exchange live media or data streams. You can read a stream with WebRTC and a web browser by visiting:

```
http://localhost:8889/mystream
```

WHEP is a WebRTC extensions that allows to read streams by using a URL, without passing through a web page. This allows to use WebRTC as a general purpose streaming protocol. If you are using a software that supports WHEP, you can read a stream from the server by using this URL:

```
http://localhost:8889/mystream/whep
```

Be aware that not all browsers can read any codec, check [Supported browsers](webrtc-specific-features#supported-browsers).

Depending on the network it may be difficult to establish a connection between server and clients, read [Solving WebRTC connectivity issues](webrtc-specific-features#solving-webrtc-connectivity-issues).

Some clients that can read with WebRTC and WHEP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [Unity](#unity) and [web browsers](#web-browsers).

### RTSP

RTSP is a protocol that allows to publish and read streams. It supports different underlying transport protocols and encryption (see [RTSP-specific features](rtsp-specific-features)). In order to read a stream with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

Some clients that can read with RTSP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer) and [VLC](#vlc).

#### Latency

The RTSP protocol doesn't introduce any latency by itself. Latency is usually introduced by clients, that put frames in a buffer to compensate network fluctuations. In order to decrease latency, the best way consists in tuning the client. For instance, in VLC, latency can be decreased by decreasing the _Network caching_ parameter, that is available in the _Open network stream_ dialog or alternatively can be set with the command line:

```
vlc --network-caching=50 rtsp://...
```

### RTMP

RTMP is a protocol that allows to read and publish streams. It supports encryption, see [RTMP-specific features](rtmp-specific-features). Streams can be read from the server by using the URL:

```
rtmp://localhost/mystream
```

Some clients that can read with RTMP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer) and [VLC](#vlc).

### HLS

HLS is a protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to generate a HLS stream, that is accessible through a web page:

```
http://localhost:8888/mystream
```

and can also be accessed without using the browsers, by software that supports the HLS protocol (for instance VLC or _MediaMTX_ itself) by using this URL:

```
http://localhost:8888/mystream/index.m3u8
```

Some clients that can read with HLS are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [VLC](#vlc) and [web browsers](#web-browsers).

#### LL-HLS

Low-Latency HLS is a recently standardized variant of the protocol that allows to greatly reduce playback latency. It works by splitting segments into parts, that are served before the segment is complete. LL-HLS is enabled by default. If the stream is not shown correctly, try tuning the hlsPartDuration parameter, for instance:

```yml
hlsPartDuration: 500ms
```

#### Codec support in browsers

The server can produce HLS streams with a variety of video and audio codecs (that are listed at the beginning of the README), but not all browsers can read all codecs due to internal limitations that cannot be overcome by this or any other server.

You can check what codecs your browser can read with HLS by [using this tool](https://jsfiddle.net/tjcyv5aw/).

If you want to support most browsers, you can to re-encode the stream by using H264 and AAC codecs, for instance by using FFmpeg:

```sh
ffmpeg -i rtsp://original-source \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-c:a aac -b:a 160k \
-f rtsp rtsp://localhost:8554/mystream
```

#### Encryption required by Apple devices

In order to correctly display Low-Latency HLS streams in Safari running on Apple devices (iOS or macOS), a TLS certificate is needed and can be generated with OpenSSL:

```sh
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Set the `hlsEncryption`, `hlsServerKey` and `hlsServerCert` parameters in the configuration file:

```yml
hlsEncryption: yes
hlsServerKey: server.key
hlsServerCert: server.crt
```

Keep also in mind that not all H264 video streams can be played on Apple Devices due to some intrinsic properties (distance between I-Frames, profile). If the video can't be played correctly, you can either:

- re-encode it by following instructions in this README
- disable the Low-latency variant of HLS and go back to the legacy variant:

  ```yml
  hlsVariant: mpegts
  ```

#### Latency

in HLS, latency is introduced since a client must wait for the server to generate segments before downloading them. This latency amounts to 500ms-3s when the low-latency HLS variant is enabled (and it is by default), otherwise amounts to 1-15secs.

To decrease the latency, you can:

- try decreasing the hlsPartDuration parameter
- try decreasing the hlsSegmentDuration parameter
- try decreasing the interval between random access frames of the video track, which are frames that can be decoded independently from others. The server adjusts the segment duration in order to include at least one random access frame into every segment. This interval can be changed in two ways:
  - if the stream is being hardware-generated (i.e. by a camera), there's usually a setting called "Key Frame Interval" in the camera configuration page
  - otherwise, the stream must be re-encoded. It is possible to tune the random access frame interval by using ffmpeg's -g option:

    ```sh
    ffmpeg -i rtsp://original-stream -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k -max_muxing_queue_size 1024 -g 30 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    ```

## Software

### FFmpeg

FFmpeg can read a stream from the server in several ways. The recommended one consists in reading with RTSP.

#### FFmpeg and RTSP

```sh
ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
```

#### FFmpeg and RTMP

```sh
ffmpeg -i rtmp://localhost/mystream -c copy output.mp4
```

In order to read AV1, VP9, H265, Opus, AC3 tracks and in order to read multiple video or audio tracks, the `-rtmp_enhanced_codecs` flag must be present:

```sh
ffmpeg -rtmp_enhanced_codecs ac-3,av01,avc1,ec-3,fLaC,hvc1,.mp3,mp4a,Opus,vp09 \
-i rtmp://localhost/mystream -c copy output.mp4
```

#### FFmpeg and SRT

```sh
ffmpeg -i 'srt://localhost:8890?streamid=read:test' -c copy output.mp4
```

### GStreamer

GStreamer can read a stream from the server in several way. The recommended one consists in reading with RTSP.

#### GStreamer and RTSP

```sh
gst-launch-1.0 rtspsrc location=rtsp://127.0.0.1:8554/mystream latency=0 ! decodebin ! autovideosink
```

For advanced options, see [RTSP-specific features](rtsp-specific-features).

#### GStreamer and WebRTC

GStreamer also supports reading streams with WebRTC/WHEP, although track codecs must be specified in advance through the `video-caps` and `audio-caps` parameters. Furthermore, if audio is not present, `audio-caps` must be set anyway and must point to a PCMU codec. For instance, the command for reading a video-only H264 stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint=http://127.0.0.1:8889/stream/whep use-link-headers=true \
video-caps="application/x-rtp,media=video,encoding-name=H264,payload=127,clock-rate=90000" \
audio-caps="application/x-rtp,media=audio,encoding-name=PCMU,payload=0,clock-rate=8000" \
! rtph264depay ! decodebin ! autovideosink
```

While the command for reading an audio-only Opus stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint="http://127.0.0.1:8889/stream/whep" use-link-headers=true \
audio-caps="application/x-rtp,media=audio,encoding-name=OPUS,payload=111,clock-rate=48000,encoding-params=(string)2" \
! rtpopusdepay ! decodebin ! autoaudiosink
```

While the command for reading a H264 and Opus stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint=http://127.0.0.1:8889/stream/whep use-link-headers=true \
video-caps="application/x-rtp,media=video,encoding-name=H264,payload=127,clock-rate=90000" \
audio-caps="application/x-rtp,media=audio,encoding-name=OPUS,payload=111,clock-rate=48000,encoding-params=(string)2" \
! decodebin ! autovideosink
```

### VLC

VLC can read a stream from the server in several way. The recommended one consists in reading with RTSP:

```sh
vlc --network-caching=50 rtsp://localhost:8554/mystream
```

#### RTSP and Ubuntu compatibility

The VLC shipped with Ubuntu 21.10 doesn't support playing RTSP due to a license issue (see [here](https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=982299) and [here](https://stackoverflow.com/questions/69766748/cvlc-cannot-play-rtsp-omxplayer-instead-can)). To fix the issue, remove the default VLC instance and install the snap version:

```sh
sudo apt purge -y vlc
snap install vlc
```

#### Encrypted RTSP compatibility

At the moment VLC doesn't support reading encrypted RTSP streams. However, you can use a proxy like [stunnel](https://www.stunnel.org) or [nginx](https://nginx.org/) or a local _MediaMTX_ instance to decrypt streams before reading them.

### OBS Studio

OBS Studio can read streams from the server by using the [RTSP protocol](#rtsp).

Open OBS, click on _Add Source_, _Media source_, _OK_, uncheck _Local file_, insert in _Input_:

```
rtsp://localhost:8554/stream
```

Then _Ok_.

### Unity

Software written with the Unity Engine can read a stream from the server by using the [WebRTC protocol](#webrtc).

Create a new Unity project or open an existing one.

Open _Window -> Package Manager_, click on the plus sign, _Add Package by name..._ and insert `com.unity.webrtc`. Wait for the package to be installed.

In the _Project_ window, under `Assets`, create a new C# Script called `WebRTCReader.cs` with this content:

```cs
using System.Collections;
using UnityEngine;
using Unity.WebRTC;

public class WebRTCReader : MonoBehaviour
{
    public string url = "http://localhost:8889/stream/whep";

    private RTCPeerConnection pc;
    private MediaStream receiveStream;

    void Start()
    {
        UnityEngine.UI.RawImage rawImage = gameObject.GetComponentInChildren<UnityEngine.UI.RawImage>();
        AudioSource audioSource = gameObject.GetComponentInChildren<AudioSource>();
        pc = new RTCPeerConnection();
        receiveStream = new MediaStream();

        pc.OnTrack = e =>
        {
            receiveStream.AddTrack(e.Track);
        };

        receiveStream.OnAddTrack = e =>
        {
            if (e.Track is VideoStreamTrack videoTrack)
            {
                videoTrack.OnVideoReceived += (tex) =>
                {
                    rawImage.texture = tex;
                };
            }
            else if (e.Track is AudioStreamTrack audioTrack)
            {
                audioSource.SetTrack(audioTrack);
                audioSource.loop = true;
                audioSource.Play();
            }
        };

        RTCRtpTransceiverInit init = new RTCRtpTransceiverInit();
        init.direction = RTCRtpTransceiverDirection.RecvOnly;
        pc.AddTransceiver(TrackKind.Audio, init);
        pc.AddTransceiver(TrackKind.Video, init);

        StartCoroutine(WebRTC.Update());
        StartCoroutine(createOffer());
    }

    private IEnumerator createOffer()
    {
        var op = pc.CreateOffer();
        yield return op;
        if (op.IsError) {
            Debug.LogError("CreateOffer() failed");
            yield break;
        }

        yield return setLocalDescription(op.Desc);
    }

    private IEnumerator setLocalDescription(RTCSessionDescription offer)
    {
        var op = pc.SetLocalDescription(ref offer);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetLocalDescription() failed");
            yield break;
        }

        yield return postOffer(offer);
    }

    private IEnumerator postOffer(RTCSessionDescription offer)
    {
        var content = new System.Net.Http.StringContent(offer.sdp);
        content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("application/sdp");
        var client = new System.Net.Http.HttpClient();

        var task = System.Threading.Tasks.Task.Run(async () => {
            var res = await client.PostAsync(new System.UriBuilder(url).Uri, content);
            res.EnsureSuccessStatusCode();
            return await res.Content.ReadAsStringAsync();
        });
        yield return new WaitUntil(() => task.IsCompleted);
        if (task.Exception != null) {
            Debug.LogError(task.Exception);
            yield break;
        }

        yield return setRemoteDescription(task.Result);
    }

    private IEnumerator setRemoteDescription(string answer)
    {
        RTCSessionDescription desc = new RTCSessionDescription();
        desc.type = RTCSdpType.Answer;
        desc.sdp = answer;
        var op = pc.SetRemoteDescription(ref desc);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetRemoteDescription() failed");
            yield break;
        }

        yield break;
    }

    void OnDestroy()
    {
        pc?.Close();
        pc?.Dispose();
        receiveStream?.Dispose();
    }
}
```

Edit the `url` variable according to your needs.

In the _Hierarchy_ window, find or create a scene. Inside the scene, add a _Canvas_. Inside the Canvas, add a _Raw Image_ and an _Audio Source_. Then add the `WebRTCReader.cs` script as component of the canvas, by dragging it inside the _Inspector_ window. then Press the _Play_ button at the top of the page.

### Web browsers

Web browsers can read a stream from the server in several ways.

#### Web browsers and WebRTC

You can read a stream by using the [WebRTC protocol](#webrtc) by visiting the web page:

```
http://localhost:8889/mystream
```

See [Embed streams in a website](embed-streams-in-a-website) for instructions on how to embed the stream into an external website.

#### Web browsers and HLS

Web browsers can also read a stream with the [HLS protocol](#hls). Latency is higher but there are less problems related to connectivity between server and clients, furthermore the server load can be balanced by using a common HTTP CDN (like Cloudflare or CloudFront), and this allows to handle an unlimited amount of readers. Visit the web page:

```
http://localhost:8888/mystream
```

See [Embed streams in a website](embed-streams-in-a-website) for instructions on how to embed the stream into an external website.
