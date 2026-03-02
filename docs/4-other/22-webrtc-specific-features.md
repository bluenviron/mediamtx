# WebRTC-specific features

WebRTC is a protocol that can be used for publishing and reading streams. Regarding specific tasks, see [Publish](../2-publish/04-webrtc-clients.md) and [Read](../3-read/03-webrtc.md). Features in this page are shared among both tasks.

## Codec support in browsers

The server can ingest and broadcast with WebRTC a wide variety of video and audio codecs (that are listed at the beginning of the README), but not all browsers can publish and read all codecs due to internal limitations that cannot be overcome by this or any other server.

In particular, reading and publishing H265 tracks with WebRTC was not possible until some time ago due to lack of browser support. The situation improved recently and can be described as following:

- Safari on iOS and macOS fully support publishing and reading H265 tracks.
- Chrome on Windows supports publishing and reading H265 tracks when a capable GPU is present.

You can check what codecs your browser can publish or read with WebRTC by [using this tool](https://jsfiddle.net/v24s8q1f/).

If you want to support most browsers, you can re-encode the stream by using H264 and Opus codecs, for instance by using FFmpeg:

```sh
ffmpeg -i rtsp://original-source \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-c:a libopus -b:a 64K -async 50 \
-f rtsp rtsp://localhost:8554/mystream
```

## Solving WebRTC connectivity issues

In WebRTC, the handshake between server and clients happens through standard HTTP requests and responses, while media streaming takes place inside a dedicated communication channel (peer connection) that is set up shortly after the handshake. The server supports establishing peer connections through the following methods (ordered by efficiency and simplicity):

1. using a static UDP server port (`webrtcLocalUDPAddress` must be filled, it is by default)
2. using a static TCP server port (`webrtcLocalTCPAddress` must be filled, it is not by default)
3. using a random UDP server port and UDP client port with the hole-punching technique (`webrtcICEServers2` must contain a STUN server, not present by default)
4. using a relay (TURN server) that exposes a TCP port that is accessed by both server and clients (`webrtcICEServers2` must contain a TURN server, not present by default)

Establishing the peer connection might get difficult when the server is hosted inside a container or there is a NAT / firewall between server and clients.

The first thing to do is making sure that `webrtcAdditionalHosts` includes your public IPs, that are IPs that can be used by clients to reach the server. If clients are on the same LAN as the server, add the LAN address of the server. If clients are coming from the internet, add the public IP address of the server, or alternatively a DNS name, if you have one. You can add several values to support all scenarios:

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

If you still have problems, the UDP protocol might be blocked by a firewall. Switch to the TCP protocol by enabling the TCP server port:

```yml
webrtcLocalTCPAddress: :8189
```

If there's a NAT / container between server and clients, it must be configured to route all incoming TCP packets on port 8189 to the server.

If you still have problems, add a STUN server, that is used by both server and clients to find out their public IP. Connections are then established with the "UDP hole punching" technique, that uses a random UDP port that does not need to be explicitly opened. For instance:

```yml
webrtcICEServers2:
  - url: stun:stun.l.google.com:19302
```

If you really still have problems, you can force all WebRTC/ICE connections to pass through a TURN server. The server address and credentials must be set in the configuration file:

```yml
webrtcICEServers2:
  - url: turn:host:port
    username: user
    password: password
```

Where user and pass are the username and password of the server. Note that port is not optional.

If the server uses a secret-based authentication (for instance, Coturn with the use-auth-secret option), it must be configured by using AUTH_SECRET as username, and the secret as password:

```yml
webrtcICEServers2:
  - url: turn:host:port
    username: AUTH_SECRET
    password: secret
```

Where secret is the secret of the TURN server. _MediaMTX_ will generate a set of credentials by using the secret, and credentials will be sent to clients before the WebRTC/ICE connection is established.

In some cases you may want the browser to connect using TURN servers but have _MediaMTX_ not using TURN (for example if the TURN server is on the same network as mediamtx). To allow this you can configure the TURN server to be client only:

```yml
webrtcICEServers2:
  - url: turn:host:port
    username: user
    password: password
    clientOnly: true
```

## Coturn setup

Here's how to setup the [Coturn](https://github.com/coturn/coturn) TURN server and use it with _MediaMTX_. This is needed only if all other WebRTC connectivity methods have failed. Start Coturn with Docker:

```sh
docker run --rm -it \
--network=host \
coturn/coturn \
--log-file=stdout -v \
--no-udp --no-dtls --no-tls \
--min-port=49152 --max-port=65535 \
--use-auth-secret --static-auth-secret=mysecret -r myrealm
```

We are suggesting and using the following settings:

- enable the TCP transport only. We are assuming you are setupping Coturn because other connectivity methods have failed, thus TCP is more reliable.
- toggle `--network=host` since Coturn allocates a TCP port for each peer connection.
- set `min-port` and `max-port` to specify the range of TCP ports.
- enable secret-based authentication, that prevents clients from storing permanently valid credentials.

Configure MediaMTX to use the TURN server:

```yml
webrtcICEServers2:
  - url: turn:REPLACE_WITH_COTURN_IP:3478?transport=tcp
    username: AUTH_SECRET
    password: mysecret
```

The `?transport=tcp` suffix is needed to force TCP usage. Use `AUTH_SECRET` as username and the shared secret as the password.
