
# rtsp-simple-server

[![Build Status](https://travis-ci.org/aler9/rtsp-simple-server.svg?branch=master)](https://travis-ci.org/aler9/rtsp-simple-server)
[![Go Report Card](https://goreportcard.com/badge/github.com/aler9/rtsp-simple-server)](https://goreportcard.com/report/github.com/aler9/rtsp-simple-server)
[![Docker Hub](https://img.shields.io/badge/docker-aler9%2Frtsp--simple--server-blue)](https://hub.docker.com/r/aler9/rtsp-simple-server)

_rtsp-simple-server_ is a simple, ready-to-use and zero-dependency RTSP server and RTSP proxy, a software that allows multiple users to publish and read live video and audio streams over time. RTSP, RTP and RTCP are standardized protocol that describe how to perform these operations with the help of a server, that is contacted by both readers and publishers in order to negotiate a streaming protocol. The server is then responsible of relaying the publisher streams to the readers.

Features:
* Read and publish streams via UDP and TCP
* Pull and serve streams from other RTSP servers (RTSP proxy)
* Each stream can have multiple video and audio tracks, encoded in any format
* Publish multiple streams at once, each in a separate path, that can be read by multiple users
* Supports authentication
* Supports running a script when a client connects or disconnects
* Compatible with Linux, Windows and Mac, does not require any dependency or interpreter, it's a single executable

## Installation and basic usage

1. Download and extract a precompiled binary from the [release](https://github.com/aler9/rtsp-simple-server/releases) page.

2. Start the server:
   ```
   ./rtsp-simple-server
   ```

3. Publish a stream. For instance, you can publish a video file with _FFmpeg_:
   ```
   ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
   ```

4. Open the stream. For instance, you can open the stream with _VLC_:
   ```
   vlc rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:
   ```
   gst-launch-1.0 -v rtspsrc location=rtsp://localhost:8554/mystream ! rtph264depay ! decodebin ! autovideosink
   ```

   or _FFmpeg_:
   ```
   ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
   ```

## Advanced usage and FAQs

#### Usage with Docker

Download and launch the image:
```
docker run --rm -it --network=host aler9/rtsp-simple-server
```

The `--network=host` argument is mandatory since Docker can change the source port of UDP packets for routing reasons, and this makes RTSP routing impossible. An alternative consists in disabling UDP and exposing the RTSP port, by creating a configuration file named `conf.yml` with the following content:
```yaml
protocols: [tcp]
```

and passing it to the container:
```
docker run --rm -it -v $PWD/conf.yml:/conf.yml -p 8554:8554 aler9/rtsp-simple-server
```

#### Full configuration file

To change the configuration, it's enough to edit the file `conf.yml`, provided with the executable. The default configuration is [available here](conf.yml).

#### Usage as RTSP Proxy

An RTSP proxy is usually deployed in one of these scenarios:
* when there are multiple users that are receiving a stream and the bandwidth is limited, so the proxy is used to receive the stream once. Users can then connect to the proxy instead of the original source.
* when there's a NAT / firewall between a stream and the users, in this case the proxy is installed in the NAT and makes the stream available to the outside world.

Edit `conf.yml` and replace everything inside section `paths` with the following content:
```yaml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
```

Start the server:
```
./rtsp-simple-server
```

Users can then connect to `rtsp://localhost:8554/proxied`, instead of connecting to the original url. The server supports any number of source streams, it's enough to add additional entries to the `paths` section.

#### Publisher authentication

Edit `conf.yml` and replace everything inside section `paths` with the following content:
```yaml
paths:
  all:
    publishUser: admin
    publishPass: mypassword
```

Start the server:
```
./rtsp-simple-server
```

Only publishers that provide both username and password will be able to proceed:
```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://admin:mypassword@localhost:8554/mystream
```

WARNING: RTSP is a plain protocol, and the credentials can be intercepted and read by malicious users (even if hashed, since the only supported hash method is md5, which is broken). If you need a secure channel, use RTSP inside a VPN.

#### Remuxing, re-encoding, compression

_rtsp-simple-server_ is an RTSP server: it publishes existing streams and does not touch them. It is not a media server, that is a far more complex and heavy software that can receive existing streams, re-encode them and publish them.

To change the format, codec or compression of a stream, you can use _FFmpeg_ or _Gstreamer_ together with _rtsp-simple-server_, obtaining the same features of a media server. For instance, if we want to re-encode an existing stream, that is available in the `/original` path, and make the resulting stream available in the `/compressed` path, it is enough to launch _FFmpeg_ in parallel with _rtsp-simple-server_, with the following syntax:
```
ffmpeg -i rtsp://localhost:8554/original -c:v libx264 -preset ultrafast -tune zerolatency -b 600k -f rtsp rtsp://localhost:8554/compressed
```

#### Client count

The current number of clients, publishers and receivers is printed in each log line; for instance, the line:
```
2020/01/01 00:00:00 [2/1/1] [client 127.0.0.1:44428] OPTION
```

means that there are 2 clients, 1 publisher and 1 receiver.

#### Full command-line usage

```
usage: rtsp-simple-server [<flags>]

rtsp-simple-server v0.0.0

RTSP server.

Flags:
  --help     Show context-sensitive help (also try --help-long and --help-man).
  --version  print version

Args:
  [<confpath>]  path to a config file. The default is conf.yml. Use 'stdin' to
                read config from stdin
```

#### Compile and run from source

Install Go &ge; 1.12, download the repository, open a terminal in it and run:
```
go run .
```

You can perform the entire operation inside Docker with:
```
make run
```

## Links

Related projects
* https://github.com/aler9/gortsplib
* https://github.com/flaviostutz/rtsp-relay

IETF Standards
* RTSP 1.0 https://tools.ietf.org/html/rfc2326
* RTSP 2.0 https://tools.ietf.org/html/rfc7826
* HTTP 1.1 https://tools.ietf.org/html/rfc2616
