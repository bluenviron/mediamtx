# Publish a stream

## Compatibility matrix

Live streams can be published to the server with the following protocols and codecs:

| protocol                                              | variants                                    | video codecs                                                                                              | audio codecs                                                                                                           |
| ----------------------------------------------------- | ------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| [SRT clients](#srt-clients)                           |                                             | H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video                                                     | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3                                                                   |
| [SRT cameras and servers](#srt-cameras-and-servers)   |                                             | H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video                                                     | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3                                                                   |
| [WebRTC clients](#webrtc-clients)                     | WHIP                                        | AV1, VP9, VP8, H265, H264                                                                                 | Opus, G722, G711 (PCMA, PCMU)                                                                                          |
| [WebRTC servers](#webrtc-servers)                     | WHEP                                        | AV1, VP9, VP8, H265, H264                                                                                 | Opus, G722, G711 (PCMA, PCMU)                                                                                          |
| [RTSP clients](#rtsp-clients)                         | UDP, TCP, RTSPS                             | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG and any RTP-compatible codec | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM and any RTP-compatible codec |
| [RTSP cameras and servers](#rtsp-cameras-and-servers) | UDP, UDP-Multicast, TCP, RTSPS              | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG and any RTP-compatible codec | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM and any RTP-compatible codec |
| [RTMP clients](#rtmp-clients)                         | RTMP, RTMPS, Enhanced RTMP                  | AV1, VP9, H265, H264                                                                                      | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                          |
| [RTMP cameras and servers](#rtmp-cameras-and-servers) | RTMP, RTMPS, Enhanced RTMP                  | AV1, VP9, H265, H264                                                                                      | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                          |
| [HLS cameras and servers](#hls-cameras-and-servers)   | Low-Latency HLS, MP4-based HLS, legacy HLS  | AV1, VP9, H265, H264                                                                                      | Opus, MPEG-4 Audio (AAC)                                                                                               |
| [MPEG-TS](#mpeg-ts)                                   | MPEG-TS over UDP, MPEG-TS over Unix sockets | H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video                                                     | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3                                                                   |
| [RTP](#rtp)                                           | RTP over UDP, RTP over Unix sockets         | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG and any RTP-compatible codec | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM and any RTP-compatible codec |

We provide instructions for publishing with the following devices:

- [Raspberry Pi Cameras](#raspberry-pi-cameras)
- [Generic webcams](#generic-webcams)

We provide instructions for publishing with the following software:

- [FFmpeg](#ffmpeg)
- [GStreamer](#gstreamer)
- [OBS Studio](#obs-studio)
- [OpenCV](#opencv)
- [Unity](#unity)
- [Web browsers](#web-browsers)

## Protocols

### SRT clients

SRT is a protocol that allows to publish and read live data stream, providing encryption, integrity and a retransmission mechanism. It is usually used to transfer media streams encoded with MPEG-TS. In order to publish a stream to the server with the SRT protocol, use this URL:

```
srt://localhost:8890?streamid=publish:mystream&pkt_size=1316
```

Replace `mystream` with any name you want. The resulting stream is available in path `/mystream`.

If you need to use the standard stream ID syntax instead of the custom one in use by this server, see [Standard stream ID syntax](srt-specific-features#standard-stream-id-syntax).

If you want to publish a stream by using a client in listening mode (i.e. with `mode=listener` appended to the URL), read the next section.

Known clients that can publish with SRT are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio).

### SRT cameras and servers

In order to ingest into the server a SRT stream from an existing server, camera or client in listening mode (i.e. with `mode=listener` appended to the URL), add the corresponding URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    # url of the source stream, in the format srt://host:port?streamid=streamid&other_parameters
    source: srt://original-url
```

### WebRTC clients

WebRTC is an API that makes use of a set of protocols and methods to connect two clients together and allow them to exchange live media or data streams. You can publish a stream with WebRTC and a web browser by visiting:

```
http://localhost:8889/mystream/publish
```

The resulting stream is available in path `/mystream`.

WHIP is a WebRTC extensions that allows to publish streams by using a URL, without passing through a web page. This allows to use WebRTC as a general purpose streaming protocol. If you are using a software that supports WHIP (for instance, latest versions of OBS Studio), you can publish a stream to the server by using this URL:

```
http://localhost:8889/mystream/whip
```

Be aware that not all browsers can read any codec, check [Supported browsers](webrtc-specific-features#supported-browsers).

Depending on the network it might be difficult to establish a connection between server and clients, read [Solving WebRTC connectivity issues](webrtc-specific-features#solving-webrtc-connectivity-issues).

Known clients that can publish with WebRTC and WHIP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio), [Unity](#unity) and [Web browsers](#web-browsers).

### WebRTC servers

In order to ingest into the server a WebRTC stream from an existing server, add the corresponding WHEP URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    # url of the source stream, in the format whep://host:port/path (HTTP) or wheps:// (HTTPS)
    source: wheps://host:port/path
```

### RTSP clients

RTSP is a protocol that allows to publish and read streams. It supports several underlying transport protocols and encryption (see [RTSP-specific features](rtsp-specific-features)). In order to publish a stream to the server with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

The resulting stream is available in path `/mystream`.

Known clients that can publish with RTSP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio).

### RTSP cameras and servers

Most IP cameras expose their video stream by using a RTSP server that is embedded into the camera itself. In particular, cameras that are compliant with ONVIF profile S or T meet this requirement. You can use _MediaMTX_ to connect to one or several existing RTSP servers and read their video streams:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
```

The resulting stream is available in path `/proxied`.

The server supports any number of source streams (count is just limited by available hardware resources) it's enough to add additional entries to the paths section:

```yml
paths:
  proxied1:
    source: rtsp://url1

  proxied2:
    source: rtsp://url1
```

### RTMP clients

RTMP is a protocol that allows to read and publish streams, but is less versatile and less efficient than RTSP and WebRTC (doesn't support UDP, doesn't support most RTSP codecs, doesn't support feedback mechanism). Streams can be published to the server by using the URL:

```
rtmp://localhost/mystream
```

The resulting stream is available in path `/mystream`.

Known clients that can publish with RTMP are [FFmpeg](#ffmpeg), [GStreamer](#gstreamer), [OBS Studio](#obs-studio).

### RTMP cameras and servers

You can use _MediaMTX_ to connect to one or several existing RTMP servers and read their video streams:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtmp://user:pass@host:port/path
    source: rtmp://original-url
```

The resulting stream is available in path `/proxied`.

### HLS cameras and servers

HLS is a streaming protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to connect to one or several existing HLS servers and read their video streams:

```yml
paths:
  proxied:
    # url of the playlist of the stream, in the format http://user:pass@host:port/path
    source: http://original-url/stream/index.m3u8
```

The resulting stream is available in path `/proxied`.

### MPEG-TS

The server supports ingesting MPEG-TS streams, shipped in two different ways (UDP packets or Unix sockets).

In order to read a UDP MPEG-TS stream, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  mypath:
    source: udp+mpegts://238.0.0.1:1234
```

Where `238.0.0.1` is the IP for listening packets, in this case a multicast IP.

You can generate a UDP multicast MPEG-TS stream with GStreamer:

```sh
gst-launch-1.0 -v mpegtsmux name=mux alignment=1 ! udpsink host=238.0.0.1 port=1234 \
videotestsrc ! video/x-raw,width=1280,height=720,format=I420 ! x264enc speed-preset=ultrafast bitrate=3000 key-int-max=60 ! video/x-h264,profile=high ! mux. \
audiotestsrc ! audioconvert ! avenc_aac ! mux.
```

or FFmpeg:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f mpegts udp://238.0.0.1:1234?pkt_size=1316
```

The resulting stream is available in path `/mypath`.

If the listening IP is a multicast IP, _MediaMTX_ will listen for incoming packets on the default multicast interface, picked by the operating system. It is possible to specify the interface manually by using the `interface` parameter:

```yml
paths:
  mypath:
    source: udp+mpegts://238.0.0.1:1234?interface=eth0
```

It is possible to restrict who can send packets by using the `source` parameter:

```yml
paths:
  mypath:
    source: udp+mpegts://0.0.0.0:1234?source=192.168.3.5
```

Known clients that can publish with UDP and MPEG-TS are [FFmpeg](#ffmpeg) and [GStreamer](#gstreamer).

Unix sockets are more efficient than UDP packets and can be used as transport by specifying the `unix+mpegts` scheme:

```yml
paths:
  mypath:
    source: unix+mpegts:///tmp/socket.sock
```

FFmpeg can generate such streams:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f mpegts unix:/tmp/socket.sock
```

### RTP

The server supports ingesting RTP streams, shipped in two different ways (UDP packets or Unix sockets).

In order to read a UDP RTP stream, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  mypath:
    source: udp+rtp://238.0.0.1:1234
    rtpSDP: |
      v=0
      o=- 123456789 123456789 IN IP4 192.168.1.100
      s=H264 Video Stream
      c=IN IP4 192.168.1.100
      t=0 0
      m=video 5004 RTP/AVP 96
      a=rtpmap:96 H264/90000
      a=fmtp:96 profile-level-id=42e01e;packetization-mode=1;sprop-parameter-sets=Z0LAHtkDxWhAAAADAEAAAAwDxYuS,aMuMsg==
```

`rtpSDP` must contain a valid SDP, that is a description of the RTP session.

FFmpeg can generate a RTP over UDP stream:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f rtp udp://238.0.0.1:1234?pkt_size=1316
```

The stream is available on path `/mypath`.

Known clients that can publish with UDP and MPEG-TS are [FFmpeg](#ffmpeg) and [GStreamer](#gstreamer).

Unix sockets are more efficient than UDP packets and can be used as transport by specifying the `unix+rtp` scheme:

```yml
paths:
  mypath:
    source: unix+rtp:///tmp/socket.sock
    rtpSDP: |
      v=0
      o=- 123456789 123456789 IN IP4 192.168.1.100
      s=H264 Video Stream
      c=IN IP4 192.168.1.100
      t=0 0
      m=video 5004 RTP/AVP 96
      a=rtpmap:96 H264/90000
      a=fmtp:96 profile-level-id=42e01e;packetization-mode=1;sprop-parameter-sets=Z0LAHtkDxWhAAAADAEAAAAwDxYuS,aMuMsg==
```

FFmpeg can generate such streams:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f rtp unix:/tmp/socket.sock
```

## Devices

### Raspberry Pi Cameras

_MediaMTX_ natively supports most of the Raspberry Pi Camera models, enabling high-quality and low-latency video streaming from the camera to any user, for any purpose. There are some additional requirements:

1. The server must run on a Raspberry Pi, with one of the following operating systems:
   - Raspberry Pi OS Bookworm
   - Raspberry Pi OS Bullseye

   Both 32 bit and 64 bit architectures are supported.

2. If you are using Raspberry Pi OS Bullseye, make sure that the legacy camera stack is disabled. Type `sudo raspi-config`, then go to `Interfacing options`, `enable/disable legacy camera support`, choose `no`. Reboot the system.

If you want to run the standard (non-Docker) version of the server:

1. Download the server executable. If you're using 64-bit version of the operative system, make sure to pick the `arm64` variant.

2. Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

   ```yml
   paths:
     cam:
       source: rpiCamera
   ```

The resulting stream is available in path `/cam`.

If you want to run the server inside Docker, you need to use the `latest-rpi` image and launch the container with some additional flags:

```sh
docker run --rm -it \
--network=host \
--privileged \
--tmpfs /dev/shm:exec \
-v /run/udev:/run/udev:ro \
-e MTX_PATHS_CAM_SOURCE=rpiCamera \
bluenviron/mediamtx:latest-rpi
```

Be aware that precompiled binaries and Docker images are not compatible with cameras that require a custom `libcamera` (like some ArduCam products), since they come with a bundled `libcamera`. If you want to use a custom one, you can [compile from source](/docs/other/compile#custom-libcamera).

Camera settings can be changed by using the `rpiCamera*` parameters:

```yml
paths:
  cam:
    source: rpiCamera
    rpiCameraWidth: 1920
    rpiCameraHeight: 1080
```

All available parameters are listed in the [configuration file](/docs/references/configuration-file).

#### Adding audio

In order to add audio from a USB microfone, install GStreamer and alsa-utils:

```sh
sudo apt install -y gstreamer1.0-tools gstreamer1.0-rtsp gstreamer1.0-alsa alsa-utils
```

list available audio cards with:

```sh
arecord -L
```

Sample output:

```
surround51:CARD=ICH5,DEV=0
    Intel ICH5, Intel ICH5
    5.1 Surround output to Front, Center, Rear and Subwoofer speakers
default:CARD=U0x46d0x809
    USB Device 0x46d:0x809, USB Audio
    Default Audio Device
```

Find the audio card of the microfone and take note of its name, for instance `default:CARD=U0x46d0x809`. Then create a new path that takes the video stream from the camera and audio from the microphone:

```yml
paths:
  cam:
    source: rpiCamera

  cam_with_audio:
    runOnInit: >
      gst-launch-1.0
      rtspclientsink name=s location=rtsp://localhost:$RTSP_PORT/cam_with_audio
      rtspsrc location=rtsp://127.0.0.1:$RTSP_PORT/cam latency=0 ! rtph264depay ! s.
      alsasrc device=default:CARD=U0x46d0x809 ! opusenc bitrate=16000 ! s.
    runOnInitRestart: yes
```

The resulting stream is available in path `/cam_with_audio`.

#### Secondary stream

It is possible to enable a secondary stream from the same camera, with a different resolution, FPS and codec. Configuration is the same of a primary stream, with `rpiCameraSecondary` set to `true` and parameters adjusted accordingly:

```yml
paths:
  # primary stream
  rpi:
    source: rpiCamera
    # Width of frames.
    rpiCameraWidth: 1920
    # Height of frames.
    rpiCameraHeight: 1080
    # FPS.
    rpiCameraFPS: 30

  # secondary stream
  secondary:
    source: rpiCamera
    # This is a secondary stream.
    rpiCameraSecondary: true
    # Width of frames.
    rpiCameraWidth: 640
    # Height of frames.
    rpiCameraHeight: 480
    # FPS.
    rpiCameraFPS: 10
    # Codec. in case of secondary streams, it defaults to M-JPEG.
    rpiCameraCodec: auto
    # JPEG quality.
    rpiCameraMJPEGQuality: 60
```

The secondary stream is available in path `/secondary`.

### Generic webcams

If the operating system is Linux, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f v4l2 -i /dev/video0 -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k -f rtsp rtsp://localhost:$RTSP_PORT/$MTX_PATH
    runOnInitRestart: yes
```

If the operating system is Windows:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f dshow -i video="USB2.0 HD UVC WebCam" -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k -f rtsp rtsp://localhost:$RTSP_PORT/$MTX_PATH
    runOnInitRestart: yes
```

Where `USB2.0 HD UVC WebCam` is the name of a webcam, that can be obtained with:

```sh
ffmpeg -list_devices true -f dshow -i dummy
```

The resulting stream is available in path `/cam`.

## Software

### FFmpeg

FFmpeg can publish a stream to the server in several ways (SRT client, SRT server, RTSP client, RTMP client, MPEG-TS over UDP, MPEG-TS over Unix sockets, WebRTC with WHIP, RTP over UDP, rtp over Unix sockets). The recommended one consists in publishing as a [RTSP client](#rtsp-clients):

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
```

The RTSP protocol supports several underlying transport protocols, each with its own characteristics (see [RTSP-specific features](rtsp-specific-features)). You can set the transport protocol by using the `rtsp_transport` flag, for instance, in order to use TCP:

```sh
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp -rtsp_transport tcp rtsp://localhost:8554/mystream
```

The resulting stream is available in path `/mystream`.

### GStreamer

GStreamer can publish a stream to the server in several ways (SRT client, SRT server, RTSP client, RTMP client, MPEG-TS over UDP, WebRTC with WHIP, RTP over UDP). The recommended one consists in publishing as a [RTSP client](#rtsp-clients):

```sh
gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream \
filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! queue ! s.sink_0 \
d.audio_0 ! queue ! s.sink_1
```

If the stream is video only:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream
```

The RTSP protocol supports several underlying transport protocols, each with its own characteristics (see [RTSP-specific features](rtsp-specific-features)). You can set the transport protocol by using the `protocols` flag:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream protocols=tcp
```

If encryption is enabled, the `tls-validation-flags` and `profiles` options must be specified too:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream tls-validation-flags=0 profiles=GST_RTSP_PROFILE_SAVP
```

The resulting stream is available in path `/mystream`.

GStreamer can also publish a stream by using the [WebRTC / WHIP protocol](#webrtc-clients). Make sure that GStreamer version is at least 1.22, and that if the codec is H264, the profile is baseline. Use the `whipclientsink` element:

```
gst-launch-1.0 videotestsrc \
! video/x-raw,width=1920,height=1080,format=I420 \
! x264enc speed-preset=ultrafast bitrate=2000 \
! video/x-h264,profile=baseline \
! whipclientsink signaller::whip-endpoint=http://localhost:8889/mystream/whip
```

### OBS Studio

OBS Studio can publish to the server in several ways (SRT client, RTMP client, WebRTC client). The recommended one consists in publishing as a [RTMP client](#rtmp-clients). In `Settings -> Stream` (or in the Auto-configuration Wizard), use the following parameters:

- Service: `Custom...`
- Server: `rtmp://localhost/mystream`
- Stream key: (empty)

Save the configuration and click `Start streaming`.

If you want to generate a stream that can be read with WebRTC, open `Settings -> Output -> Recording` and use the following parameters:

- FFmpeg output type: `Output to URL`
- File path or URL: `rtsp://localhost:8554/mystream`
- Container format: `rtsp`
- Check `show all codecs (even if potentically incompatible)`
- Video encoder: `h264_nvenc (libx264)`
- Video encoder settings (if any): `bf=0`
- Audio track: `1`
- Audio encoder: `libopus`

Then use the button `Start Recording` (instead of `Start Streaming`) to start streaming.

Recent versions of OBS Studio can also publish to the server with the [WebRTC / WHIP protocol](#webrtc-clients) Use the following parameters:

- Service: `WHIP`
- Server: `http://localhost:8889/mystream/whip`

Save the configuration and click `Start streaming`.

The resulting stream is available in path `/mystream`.

### OpenCV

Software which uses the OpenCV library can publish to the server through its GStreamer plugin, as a [RTSP client](#rtsp-clients). It must be compiled with GStreamer support, by following this procedure:

```sh
sudo apt install -y libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev gstreamer1.0-plugins-ugly gstreamer1.0-rtsp python3-dev python3-numpy
git clone --depth=1 -b 4.5.4 https://github.com/opencv/opencv
cd opencv
mkdir build && cd build
cmake -D CMAKE_INSTALL_PREFIX=/usr -D WITH_GSTREAMER=ON ..
make -j$(nproc)
sudo make install
```

You can check that OpenCV has been installed correctly by running:

```sh
python3 -c 'import cv2; print(cv2.getBuildInformation())'
```

Check that the output contains `GStreamer: YES`.

Videos can be published with `cv2.VideoWriter`:

```python
from datetime import datetime
from time import sleep, time

import cv2
import numpy as np

fps = 15
width = 800
height = 600
colors = [
    (0, 0, 255),
    (255, 0, 0),
    (0, 255, 0),
]

out = cv2.VideoWriter('appsrc ! videoconvert' + \
    ' ! video/x-raw,format=I420' + \
    ' ! x264enc speed-preset=ultrafast bitrate=600 key-int-max=' + str(fps * 2) + \
    ' ! video/x-h264,profile=baseline' + \
    ' ! rtspclientsink location=rtsp://localhost:8554/mystream',
    cv2.CAP_GSTREAMER, 0, fps, (width, height), True)
if not out.isOpened():
    raise Exception("can't open video writer")

curcolor = 0
start = time()

while True:
    frame = np.zeros((height, width, 3), np.uint8)

    # create a rectangle
    color = colors[curcolor]
    curcolor += 1
    curcolor %= len(colors)
    for y in range(0, int(frame.shape[0] / 2)):
        for x in range(0, int(frame.shape[1] / 2)):
            frame[y][x] = color

    out.write(frame)
    print("%s frame written to the server" % datetime.now())

    now = time()
    diff = (1 / fps) - now - start
    if diff > 0:
        sleep(diff)
    start = now
```

The resulting stream is available in path `/mystream`.

### Unity

Software written with the Unity Engine can publish a stream to the server by using the [WebRTC protocol](#webrtc-clients).

Create a new Unity project or open an existing one.

Open _Window -> Package Manager_, click on the plus sign, _Add Package by name..._ and insert `com.unity.webrtc`. Wait for the package to be installed.

In the _Project_ window, under `Assets`, create a new C# Script called `WebRTCPublisher.cs` with this content:

```cs
using System.Collections;
using UnityEngine;
using Unity.WebRTC;
using UnityEngine.Networking;

public class WebRTCPublisher : MonoBehaviour
{
    public string url = "http://localhost:8889/unity/whip";
    public int videoWidth = 1280;
    public int videoHeight = 720;

    private RTCPeerConnection pc;
    private MediaStream videoStream;

    void Start()
    {
        pc = new RTCPeerConnection();
        Camera sourceCamera = gameObject.GetComponent<Camera>();
        videoStream = sourceCamera.CaptureStream(videoWidth, videoHeight);
        foreach (var track in videoStream.GetTracks())
        {
            pc.AddTrack(track);
        }

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
        videoStream?.Dispose();
    }
}
```

In the _Hierarchy_ window, find or create a scene and a camera, then add the `WebRTCPublisher.cs` script as component of the camera, by dragging it inside the _Inspector_ window. then Press the _Play_ button at the top of the page.

The resulting stream is available in path `/unity`.

### Web browsers

Web browsers can publish a stream to the server by using the [WebRTC protocol](#webrtc-clients). Start the server and open the web page:

```
http://localhost:8889/mystream/publish
```

The resulting stream is available in path `/mystream`.

This web page can be embedded into another web page by using an iframe:

```html
<iframe src="http://mediamtx-ip:8889/mystream/publish" scrolling="no"></iframe>
```

For more advanced setups, you can create and serve a custom web page by starting from the [source code of the WebRTC publish page](https://github.com/bluenviron/mediamtx/blob/{version_tag}/internal/servers/webrtc/publish_index.html). In particular, there's a ready-to-use, standalone JavaScript class for publishing streams with WebRTC, available in [publisher.js](https://github.com/bluenviron/mediamtx/blob/{version_tag}/internal/servers/webrtc/publisher.js).
