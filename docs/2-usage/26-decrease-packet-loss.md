# Decrease packet loss

MediaMTX is meant for routing live streams, and makes use of a series of protocol which try to preserve the real-time aspect of streams and minimize latency at cost of losing packets in transmit. In particular, most protocols are built on UDP, which is an "unrealiable transport", specifically picked because it allows to drop late packets in case of network congestions.

These packet losses are usually detected and printed in MediaMTX logs.

If you need to improve the stream reliability and decrease packet losses, the first thing to do is to check whether the network between the MediaMTX instance and the intended publishers and readers has sufficient bandwidth for transmitting the media stream. Most of the times, packet losses are caused by a network which is not fit for this scope. This limitation can be overcome by either recompressing the stream with a lower bitrate, or by upgrading the network physical infrastructure (routers, cables, Wi-Fi, firewalls, topology, etc).

There are however some parameters that can be tuned to improve the situation, at cost of increasing RAM consumption:

- When publishing a stream with a UDP-based protocol (currently RTSP, MPEG-TS, RTP, SRT, WebRTC), packets might get discarded by the server because the read buffer size of UDP sockets is too small. It can be increased with this parameter:

  ```yml
  udpReadBufferSize: 1000000
  ```

  The `udpReadBufferSize` parameter requires the `net.core.rmem_max` system parameter to be equal or greater than it. It can be set with this command:

  ```sh
  sudo sysctl net.core.rmem_max=100000000
  ```

- When reading a stream, packets might get discarded because the write queue is too small. This can be noticed in logs through the "reader is too slow" message. Try increasing the write queue:

  ```yml
  writeQueueSize: 1024
  ```

- When publishing or reading a stream with RTSP, it's possible to switch from the UDP transport protocol to the TCP transport protocol, which is less performant but has a packet retransmission mechanism:

  ```yml
  rtspTransports: [tcp]
  ```

  In case the source is a camera:

  ```yml
  paths:
    test:
      source: rtsp://..
      rtspTransport: tcp
  ```
