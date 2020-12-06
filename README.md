
# rtsp-simple-server

[![Test](https://github.com/aler9/rtsp-simple-server/workflows/test/badge.svg)](https://github.com/aler9/rtsp-simple-server/actions)
[![Lint](https://github.com/aler9/rtsp-simple-server/workflows/lint/badge.svg)](https://github.com/aler9/rtsp-simple-server/actions)
[![Docker Hub](https://img.shields.io/badge/docker-aler9%2Frtsp--simple--server-blue)](https://hub.docker.com/r/aler9/rtsp-simple-server)

_rtsp-simple-server_ is a simple, ready-to-use and zero-dependency RTSP server and RTSP proxy, a software that allows multiple users to publish, read and proxy live video and audio streams over time. RTSP is a standard protocol that describes how to perform these operations with the help of a server, that is contacted by both publishers and readers and relays the publisher's streams to the readers.

Features:

* Read and publish live streams with UDP and TCP
* Each stream can have multiple video and audio tracks, encoded with any codec (including H264, H265, VP8, VP9, MP3, AAC, Opus, PCM)
* Serve multiple streams at once in separate paths
* Pull and serve streams from other RTSP or RTMP servers, always or on-demand (RTSP proxy)
* Authenticate readers and publishers separately
* Redirect to other RTSP servers (load balancing)
* Run custom commands when clients connect, disconnect, read or publish streams
* Reload the configuration without disconnecting existing clients (hot reloading)
* Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

## Installation and basic usage

1. Download and extract a precompiled binary from the [release page](https://github.com/aler9/rtsp-simple-server/releases).

2. Start the server:

   ```
   ./rtsp-simple-server
   ```

3. Publish a stream. For instance, you can publish a video/audio file with _FFmpeg_:

   ```
   ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```
   gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream filesrc location=file.mp4 ! qtdemux name=d d.video_0 ! queue ! s.sink_0 d.audio_0 ! queue ! s.sink_1
   ```

4. Open the stream. For instance, you can open the stream with _VLC_:

   ```
   vlc rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```
   gst-launch-1.0 rtspsrc location=rtsp://localhost:8554/mystream name=s s. ! application/x-rtp,media=video ! decodebin ! autovideosink s. ! application/x-rtp,media=audio ! decodebin ! audioconvert ! audioresample ! autoaudiosink
   ```

   or _FFmpeg_:

   ```
   ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
   ```

## Advanced usage and FAQs

### Usage with Docker

Download and launch the image:

```
docker run --rm -it --network=host aler9/rtsp-simple-server
```

The `--network=host` argument is mandatory since Docker can change the source port of UDP packets for routing reasons, and this makes RTSP routing impossible. This issue can be avoided by disabling UDP and exposing the RTSP port:

```
docker run --rm -it -e RTSP_PROTOCOLS=tcp -p 8554:8554 aler9/rtsp-simple-server
```

### Configuration

To see or change the configuration, edit the `rtsp-simple-server.yml` file, that is:

* included the release bundle
* available in the root folder of the Docker image (`/rtsp-simple-server.yml`)
* also [available here](rtsp-simple-server.yml).

Every configuration parameter can be overridden by environment variables, in the format `RTSP_PARAMNAME`, where `PARAMNAME` is the uppercase name of a parameter. For instance, the `rtspPort` parameter can be overridden in the following way:

```
RTSP_RTSPPORT=8555 ./rtsp-simple-server
```

Parameters in maps can be overridden by using underscores, in the following way:

```
RTSP_PATHS_TEST_SOURCE=rtsp://myurl ./rtsp-simple-server
```

The configuration can be changed dinamically when the server is running (hot reloading) by writing to the configuration file. Changes are detected and applied without disconnecting existing clients, whenever it's possible.

### RTSP proxy mode

_rtsp-simple-server_ is also a RTSP proxy, that is usually deployed in one of these scenarios:

* when there are multiple users that are receiving a stream and the bandwidth is limited; the proxy is used to receive the stream once. Users can then connect to the proxy instead of the original source.
* when there's a NAT / firewall between a stream and the users; the proxy is installed on the NAT and makes the stream available to the outside world.

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
```

After starting the server, users can connect to `rtsp://localhost:8554/proxied`, instead of connecting to the original url. The server supports any number of source streams, it's enough to add additional entries to the `paths` section:

```yml
paths:
  proxied1:
    source: rtsp://url1

  proxied2:
    source: rtsp://url1
```

It's possible to save bandwidth by enabling the on-demand mode: the stream will be pulled only when at least a client is connected:

```yml
paths:
  proxied:
    source: rtsp://original-url
    sourceOnDemand: yes
```

### Publish a webcam

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f v4l2 -i /dev/video0 -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

If the platform is Windows:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f dshow -i video="USB2.0 HD UVC WebCam" -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

Where `USB2.0 HD UVC WebCam` is the name of your webcam, that can be obtained with:
```
ffmpeg -list_devices true -f dshow -i dummy
```

After starting the server, the webcam can be reached on `rtsp://localhost:8554/cam`.

### Publish a Raspberry Pi Camera

Install dependencies:

1. Gstreamer

   ```
   sudo apt install -y gstreamer1.0-tools gstreamer1.0-rtsp
   ```

2. gst-rpicamsrc, by following [instruction here](https://github.com/thaytan/gst-rpicamsrc)

Then edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: gst-launch-1.0 rpicamsrc preview=false bitrate=2000000 keyframe-interval=50 ! video/x-h264,width=1920,height=1080,framerate=25/1 ! rtspclientsink location=rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

After starting the server, the webcam is available on `rtsp://localhost:8554/cam`.

### Generate HLS

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  all:
    runOnPublish: ffmpeg -re -i rtsp://localhost:$RTSP_PORT/$RTSP_PATH -c copy -f hls -hls_time 1 -hls_list_size 3 -hls_flags delete_segments -hls_allow_cache 0 stream.m3u8
    runOnPublishRestart: yes
```

Every time someone publishes a stream, the server will produce HLS segments, that can be served by a web server.

The example above makes the assumption that the incoming stream is encoded with H264 and AAC, since they are the only codecs supported by HLS; if the incoming stream is encoded with different codecs, it must be converted:

```yml
paths:
  all:
    runOnPublish: ffmpeg -re -i rtsp://localhost:$RTSP_PORT/$RTSP_PATH -c:a aac -b:a 64k -c:v libx264 -preset ultrafast -b:v 500k -f hls -hls_time 1 -hls_list_size 3 -hls_flags delete_segments -hls_allow_cache 0 stream.m3u8
    runOnPublishRestart: yes
```

### Remuxing, re-encoding, compression

To change the format, codec or compression of a stream, use _FFmpeg_ or _Gstreamer_ together with _rtsp-simple-server_. For instance, to re-encode an existing stream, that is available in the `/original` path, and publish the resulting stream in the `/compressed` path, edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  all:
  original:
    runOnPublish: ffmpeg -i rtsp://localhost:$RTSP_PORT/$RTSP_PATH -c:v libx264 -preset ultrafast -b:v 500k -max_muxing_queue_size 1024 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    runOnPublishRestart: yes
```

### On-demand publishing

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  ondemand:
    runOnDemand: ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnDemandRestart: yes
```

The command inserted into `runOnDemand` will start only when a client requests the path `ondemand`, therefore the file will start streaming only when requested.

### Redirect to another server

To redirect to another server, use the `redirect` source:

```yml
paths:
  redirected:
    source: redirect
    sourceRedirect: rtsp://otherurl/otherpath
```

### Fallback stream

If no one is publishing to the server, readers can be redirected to a fallback URL that is serving a fallback stream:

```yml
paths:
  withfallback:
    fallback: rtsp://otherurl/otherpath
```

### Authentication

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  all:
    publishUser: admin
    publishPass: mypassword
```

Only publishers that provide both username and password will be able to proceed:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://admin:mypassword@localhost:8554/mystream
```

It's possible to setup authentication for readers too:

```yml
paths:
  all:
    publishUser: admin
    publishPass: mypassword

    readUser: user
    readPass: userpassword
```

WARNING: RTSP is a plain protocol, and the credentials can be intercepted and read by malicious users (even if hashed, since the only supported hash method is md5, which is broken). If you need a secure channel, use RTSP inside a VPN.

### Start on boot with systemd

Systemd is the service manager used by Ubuntu, Debian and many other Linux distributions, and allows to launch rtsp-simple-server on boot.

Download a release bundle from the [release page](https://github.com/aler9/rtsp-simple-server/releases), unzip it, and move the executable and configuration in the system:

```
sudo mv rtsp-simple-server /usr/local/bin/
sudo mv rtsp-simple-server.yml /usr/local/etc/
```

Create the service:

```
sudo tee /etc/systemd/system/rtsp-simple-server.service >/dev/null << EOF
[Unit]
After=network.target
[Service]
ExecStart=/usr/local/bin/rtsp-simple-server /usr/local/etc/rtsp-simple-server.yml
[Install]
WantedBy=multi-user.target
EOF
```

Enable and start the service:

```
sudo systemctl enable rtsp-simple-server
sudo systemctl start rtsp-simple-server
```

### Monitoring

There are multiple ways to monitor the server usage over time:

* The current number of clients, publishers and readers is printed in each log line; for instance, the line:

  ```
  2020/01/01 00:00:00 [2/1/1] [client 127.0.0.1:44428] OPTION
  ```

  means that there are 2 clients, 1 publisher and 1 reader.

* A metrics exporter, compatible with Prometheus, can be enabled with the parameter `metrics: yes`; then the server can be queried for metrics with Prometheus or with a simple HTTP request:

  ```
  wget -qO- localhost:9998
  ```

  Obtaining:

  ```
  rtsp_clients{state="idle"} 2 1596122687740
  rtsp_clients{state="publishing"} 15 1596122687740
  rtsp_clients{state="reading"} 8 1596122687740
  rtsp_sources{type="rtsp",state="idle"} 3 1596122687740
  rtsp_sources{type="rtsp",state="running"} 2 1596122687740
  rtsp_sources{type="rtmp",state="idle"} 1 1596122687740
  rtsp_sources{type="rtmp",state="running"} 0 1596122687740
  ```

  where:

  * `rtsp_clients{state="idle"}` is the count of clients that are neither publishing nor reading
  * `rtsp_clients{state="publishing"}` is the count of clients that are publishing
  * `rtsp_clients{state="reading"}` is the count of clients that are reading
  * `rtsp_sources{type="rtsp",state="idle"}` is the count of rtsp sources that are not running
  * `rtsp_sources{type="rtsp",state="running"}` is the count of rtsp sources that are running
  * `rtsp_sources{type="rtmp",state="idle"}` is the count of rtmp sources that are not running
  * `rtsp_sources{type="rtmp",state="running"}` is the count of rtmp sources that are running

* A performance monitor, compatible with pprof, can be enabled with the parameter `pprof: yes`; then the server can be queried for metrics with pprof-compatible tools, like:

  ```
  go tool pprof -text http://localhost:9999/debug/pprof/goroutine
  go tool pprof -text http://localhost:9999/debug/pprof/heap
  go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=30
  ```

### Full command-line usage

```
usage: rtsp-simple-server [<flags>]

rtsp-simple-server v0.0.0

RTSP server.

Flags:
  --help     Show context-sensitive help (also try --help-long and --help-man).
  --version  print version

Args:
  [<confpath>]  path to a config file. The default is rtsp-simple-server.yml.
```

### Compile and run from source

Install Go &ge; 1.15, download the repository, open a terminal in it and run:

```
go run .
```

You can perform the entire operation inside Docker:

```
make run
```

## Links

Related projects

* https://github.com/aler9/gortsplib (RTSP library used internally)
* https://github.com/pion/sdp (SDP library used internally)
* https://github.com/pion/rtcp (RTCP library used internally)
* https://github.com/pion/rtp (RTP library used internally)
* https://github.com/notedit/rtmp (RTMP library used internally)
* https://github.com/flaviostutz/rtsp-relay

IETF Standards

* RTSP 1.0 https://tools.ietf.org/html/rfc2326
* RTSP 2.0 https://tools.ietf.org/html/rfc7826
* HTTP 1.1 https://tools.ietf.org/html/rfc2616

Conventions

* https://github.com/golang-standards/project-layout
