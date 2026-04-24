# Extract metrics

_MediaMTX_ provides several metrics through a dedicated HTTP server, in a format compatible with [Prometheus](https://prometheus.io/).

This server can be enabled by setting `metrics: yes` in the configuration.

Metrics can be extracted with Prometheus or with a simple HTTP request:

```
curl http://localhost:9998/metrics
```

Obtaining:

```ini
# Paths
paths{name="[path_name]",state="[state]"} 1
paths_readers{name="[path_name]",state="[state]",readerType="[readerType]"} 5
paths_inbound_bytes{name="[path_name]",state="[state]"} 1234
paths_outbound_bytes{name="[path_name]",state="[state]"} 1234
paths_inbound_frames_in_error{name="[path_name]",state="[state]"} 1234

# HLS sessions
hls_sessions{id="[id]",path="[path]",remoteAddr="[remoteAddr]"} 1
hls_sessions_outbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]"} 187

# HLS muxers
hls_muxers{name="[name]"} 1
hls_muxers_outbound_bytes{name="[name]"} 187
hls_muxers_outbound_frames_discarded{name="[name]"} 12

# RTSP connections
rtsp_conns{id="[id]"} 1
rtsp_conns_inbound_bytes{id="[id]"} 1234
rtsp_conns_outbound_bytes{id="[id]"} 187

# RTSP sessions
rtsp_sessions{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtsp_sessions_inbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtsp_sessions_inbound_rtp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_inbound_rtp_packets_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_inbound_rtp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_inbound_rtp_packets_jitter{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_inbound_rtcp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_inbound_rtcp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_outbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
rtsp_sessions_outbound_rtp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_outbound_rtp_packets_reported_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_outbound_rtp_packets_discarded{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_outbound_rtcp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123

# RTSPS connections
rtsps_conns{id="[id]"} 1
rtsps_conns_inbound_bytes{id="[id]"} 1234
rtsps_conns_outbound_bytes{id="[id]"} 187

# RTSPS sessions
rtsps_sessions{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtsps_sessions_inbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtsps_sessions_inbound_rtp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_inbound_rtp_packets_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_inbound_rtp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_inbound_rtp_packets_jitter{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_inbound_rtcp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_inbound_rtcp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_outbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
rtsps_sessions_outbound_rtp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_outbound_rtp_packets_reported_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_outbound_rtp_packets_discarded{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_outbound_rtcp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123

# RTMP connections
rtmp_conns{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtmp_conns_inbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtmp_conns_outbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
rtmp_conns_outbound_frames_discarded{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 12

# RTMPS connections
rtmps_conns{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtmps_conns_inbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtmps_conns_outbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
rtmps_conns_outbound_frames_discarded{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 12

# SRT connections
srt_conns{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
srt_conns_packets_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_sent_unique{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_unique{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_send_loss{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_loss{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_retrans{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_retrans{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_sent_ack{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_ack{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_sent_nak{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_nak{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_sent_km{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_km{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_us_snd_duration{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_belated{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_send_drop{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_drop{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_undecrypt{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
srt_conns_bytes_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
srt_conns_bytes_sent_unique{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_unique{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_loss{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_retrans{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_retrans{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_belated{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_send_drop{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_drop{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_undecrypt{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_us_packets_send_period{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_packets_flow_window{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_flight_size{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_ms_rtt{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_mbps_send_rate{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_mbps_receive_rate{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_mbps_link_capacity{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_bytes_avail_send_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_avail_receive_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_mbps_max_bw{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} -123
srt_conns_bytes_mss{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_send_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_send_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_ms_send_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_ms_send_tsb_pd_delay{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_receive_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_receive_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_ms_receive_buf{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_ms_receive_tsb_pd_delay{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_reorder_tolerance{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_avg_belated_time{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_send_loss_rate{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_packets_received_loss_rate{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_outbound_frames_discarded{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 12

# WebRTC sessions
webrtc_sessions{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
webrtc_sessions_inbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
webrtc_sessions_inbound_rtp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_inbound_rtp_packets_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_inbound_rtp_packets_jitter{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_inbound_rtcp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_outbound_bytes{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
webrtc_sessions_outbound_rtp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_outbound_rtcp_packets{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_outbound_frames_discarded{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 12
```

Bitrates are not provided directly as metrics because they can be computed from received and sent bytes by any metrics analyzer (i.e. Grafana).

Metrics can be filtered by using HTTP query parameters:

- `type=[TYPE]`: show metrics of a certain type only. TYPE can be `paths`, `hls_sessions`, `hls_muxers`, `rtsp_conns`, `rtsp_sessions`, `rtsps_conns`, `rtsps_sessions`, `rtmp_conns`, `rtmps_conns`, `srt_conns`, `webrtc_sessions`.
- `path=[PATH]`: show metrics belonging to a specific path only
- `hls_muxer=[PATH]`: show metrics belonging to a specific HLS muxer only
- `hls_session=[ID]`: show metrics belonging to a specific HLS session only
- `rtsp_conn=[ID]` show metrics belonging to a specific RTSP connection only
- `rtsp_session=[SESSION]`: show metrics belonging to a specific RTSP session only
- `rtsps_conn=[ID]` show metrics belonging to a specific RTSPS connection only
- `rtsps_session=[SESSION]`: show metrics belonging to a specific RTSPS session only
- `rtmp_conn=[ID]` show metrics belonging to a specific RTMP connection only
- `rtmps_conn=[ID]` show metrics belonging to a specific RTMPS connection only
- `srt_conn=[ID]` show metrics belonging to a specific SRT connection only
- `webrtc_session=[ID]` show metrics belonging to a specific WebRTC session only
