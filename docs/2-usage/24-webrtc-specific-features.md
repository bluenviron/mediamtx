# WebRTC-specific features

WebRTC is a protocol that can be used for publishing and reading streams. Regarding specific tasks, see [Publish](publish#webrtc-clients) and [Read](read#webrtc). Features in these page are shared among both tasks.

## Codec support in browsers

The server can ingest and broadcast with WebRTC a wide variety of video and audio codecs (that are listed at the beginning of the README), but not all browsers can publish and read all codecs due to internal limitations that cannot be overcome by this or any other server.

In particular, reading and publishing H265 tracks with WebRTC was not possible until some time ago due to lack of browser support. The situation improved recently and can be described as following:

- Safari on iOS and macOS fully support publishing and reading H265 tracks
- Chrome on Windows supports publishing and reading H265 tracks when a GPU is present and when the browser is launched with the following flags:

  ```
  chrome.exe --enable-features=PlatformHEVCEncoderSupport,WebRtcAllowH265Receive,WebRtcAllowH265Send --force-fieldtrials=WebRTC-Video-H26xPacketBuffer/Enabled
  ```

  We are expecting these flags to become redundant in the future and the feature to be turned on by default.

You can check what codecs your browser can publish or read with WebRTC by [using this tool](https://jsfiddle.net/v24s8q1f/).

If you want to support most browsers, you can to re-encode the stream by using H264 and Opus codecs, for instance by using FFmpeg:

```sh
ffmpeg -i rtsp://original-source \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-c:a libopus -b:a 64K -async 50 \
-f rtsp rtsp://localhost:8554/mystream
```

## Solving WebRTC connectivity issues

If the server is hosted inside a container or is behind a NAT, additional configuration is required in order to allow the two WebRTC parts (server and client) to establish a connection.

Make sure that `webrtcAdditionalHosts` includes your public IPs, that are IPs that can be used by clients to reach the server. If clients are on the same LAN as the server, add the LAN address of the server. If clients are coming from the internet, add the public IP address of the server, or alternatively a DNS name, if you have one. You can add several values to support all scenarios:

```yml
webrtcAdditionalHosts: [192.168.x.x, 1.2.3.4, my-dns.example.org, ...]
```

If there's a NAT / container between server and clients, it must be configured to route all incoming UDP packets on port 8189 to the server. If you're using Docker, this can be achieved with the flag:

```sh
docker run --rm -it \
-p 8189:8189/udp
....
bluenviron/mediamtx:1
```

If you still have problems, the UDP protocol might be blocked by a firewall. Enable the TCP protocol by enabling the local TCP listener:

```yml
webrtcLocalTCPAddress: :8189
```

If there's a NAT / container between server and clients, it must be configured to route all incoming TCP packets on port 8189 to the server.

If you still have problems, add a STUN server. When a STUN server is in use, server IP is obtained automatically and connections are established with the "UDP hole punching" technique, that uses a random UDP port that does not need to be open. For instance:

```yml
webrtcICEServers2:
  - url: stun:stun.l.google.com:19302
```

If you really still have problems, you can force all WebRTC/ICE connections to pass through a TURN server, like [coturn](https://github.com/coturn/coturn), that must be configured externally. The server address and credentials must be set in the configuration file:

```yml
webrtcICEServers2:
  - url: turn:host:port
    username: user
    password: password
```

Where user and pass are the username and password of the server. Note that port is not optional.

If the server uses a secret-based authentication (for instance, coturn with the use-auth-secret option), it must be configured by using AUTH_SECRET as username, and the secret as password:

```yml
webrtcICEServers2:
  - url: turn:host:port
    username: AUTH_SECRET
    password: secret
```

where secret is the secret of the TURN server. _MediaMTX_ will generate a set of credentials by using the secret, and credentials will be sent to clients before the WebRTC/ICE connection is established.

In some cases you may want the browser to connect using TURN servers but have mediamtx not using TURN (for example if the TURN server is on the same network as mediamtx). To allow this you can configure the TURN server to be client only:

```yml
webrtcICEServers2:
  - url: turn:host:port
    username: user
    password: password
    clientOnly: true
```
