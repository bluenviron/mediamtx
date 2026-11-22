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
hls_sessions{path="[path]"} 1
hls_sessions_outbound_bytes{path="[path]"} 187

# HLS muxers
hls_muxers{name="[name]"} 1
hls_muxers_outbound_bytes{name="[name]"} 187
hls_muxers_outbound_frames_discarded{name="[name]"} 12

# RTSP connections
rtsp_conns 1
rtsp_conns_inbound_bytes 1234
rtsp_conns_outbound_bytes 187

# RTSP sessions
rtsp_sessions{path="[path]",state="[state]"} 1
rtsp_sessions_inbound_bytes{path="[path]",state="[state]"} 1234
rtsp_sessions_inbound_rtp_packets{path="[path]",state="[state]"} 123
rtsp_sessions_inbound_rtp_packets_lost{path="[path]",state="[state]"} 123
rtsp_sessions_inbound_rtp_packets_in_error{path="[path]",state="[state]"} 123
rtsp_sessions_inbound_rtp_packets_jitter{path="[path]",state="[state]"} 123
rtsp_sessions_inbound_rtcp_packets{path="[path]",state="[state]"} 123
rtsp_sessions_inbound_rtcp_packets_in_error{path="[path]",state="[state]"} 123
rtsp_sessions_outbound_bytes{path="[path]",state="[state]"} 187
rtsp_sessions_outbound_rtp_packets{path="[path]",state="[state]"} 123
rtsp_sessions_outbound_rtp_packets_reported_lost{path="[path]",state="[state]"} 123
rtsp_sessions_outbound_rtp_packets_discarded{path="[path]",state="[state]"} 123
rtsp_sessions_outbound_rtcp_packets{path="[path]",state="[state]"} 123

# RTSPS connections
rtsps_conns 1
rtsps_conns_inbound_bytes 1234
rtsps_conns_outbound_bytes 187

# RTSPS sessions
rtsps_sessions{path="[path]",state="[state]"} 1
rtsps_sessions_inbound_bytes{path="[path]",state="[state]"} 1234
rtsps_sessions_inbound_rtp_packets{path="[path]",state="[state]"} 123
rtsps_sessions_inbound_rtp_packets_lost{path="[path]",state="[state]"} 123
rtsps_sessions_inbound_rtp_packets_in_error{path="[path]",state="[state]"} 123
rtsps_sessions_inbound_rtp_packets_jitter{path="[path]",state="[state]"} 123
rtsps_sessions_inbound_rtcp_packets{path="[path]",state="[state]"} 123
rtsps_sessions_inbound_rtcp_packets_in_error{path="[path]",state="[state]"} 123
rtsps_sessions_outbound_bytes{path="[path]",state="[state]"} 187
rtsps_sessions_outbound_rtp_packets{path="[path]",state="[state]"} 123
rtsps_sessions_outbound_rtp_packets_reported_lost{path="[path]",state="[state]"} 123
rtsps_sessions_outbound_rtp_packets_discarded{path="[path]",state="[state]"} 123
rtsps_sessions_outbound_rtcp_packets{path="[path]",state="[state]"} 123

# RTMP connections
rtmp_conns{path="[path]",state="[state]"} 1
rtmp_conns_inbound_bytes{path="[path]",state="[state]"} 1234
rtmp_conns_outbound_bytes{path="[path]",state="[state]"} 187
rtmp_conns_outbound_frames_discarded{path="[path]",state="[state]"} 12

# RTMPS connections
rtmps_conns{path="[path]",state="[state]"} 1
rtmps_conns_inbound_bytes{path="[path]",state="[state]"} 1234
rtmps_conns_outbound_bytes{path="[path]",state="[state]"} 187
rtmps_conns_outbound_frames_discarded{path="[path]",state="[state]"} 12

# SRT connections
srt_conns{path="[path]",state="[state]"} 1
srt_conns_packets_sent{path="[path]",state="[state]"} 123
srt_conns_packets_received{path="[path]",state="[state]"} 123
srt_conns_packets_sent_unique{path="[path]",state="[state]"} 123
srt_conns_packets_received_unique{path="[path]",state="[state]"} 123
srt_conns_packets_send_loss{path="[path]",state="[state]"} 123
srt_conns_packets_received_loss{path="[path]",state="[state]"} 123
srt_conns_packets_retrans{path="[path]",state="[state]"} 123
srt_conns_packets_received_retrans{path="[path]",state="[state]"} 123
srt_conns_packets_sent_ack{path="[path]",state="[state]"} 123
srt_conns_packets_received_ack{path="[path]",state="[state]"} 123
srt_conns_packets_sent_nak{path="[path]",state="[state]"} 123
srt_conns_packets_received_nak{path="[path]",state="[state]"} 123
srt_conns_packets_sent_km{path="[path]",state="[state]"} 123
srt_conns_packets_received_km{path="[path]",state="[state]"} 123
srt_conns_us_snd_duration{path="[path]",state="[state]"} 123
srt_conns_packets_received_belated{path="[path]",state="[state]"} 123
srt_conns_packets_send_drop{path="[path]",state="[state]"} 123
srt_conns_packets_received_drop{path="[path]",state="[state]"} 123
srt_conns_packets_received_undecrypt{path="[path]",state="[state]"} 123
srt_conns_bytes_sent{path="[path]",state="[state]"} 187
srt_conns_bytes_received{path="[path]",state="[state]"} 1234
srt_conns_bytes_sent_unique{path="[path]",state="[state]"} 123
srt_conns_bytes_received_unique{path="[path]",state="[state]"} 123
srt_conns_bytes_received_loss{path="[path]",state="[state]"} 123
srt_conns_bytes_retrans{path="[path]",state="[state]"} 123
srt_conns_bytes_received_retrans{path="[path]",state="[state]"} 123
srt_conns_bytes_received_belated{path="[path]",state="[state]"} 123
srt_conns_bytes_send_drop{path="[path]",state="[state]"} 123
srt_conns_bytes_received_drop{path="[path]",state="[state]"} 123
srt_conns_bytes_received_undecrypt{path="[path]",state="[state]"} 123
srt_conns_us_packets_send_period{path="[path]",state="[state]"} 123.123
srt_conns_packets_flow_window{path="[path]",state="[state]"} 123
srt_conns_packets_flight_size{path="[path]",state="[state]"} 123
srt_conns_ms_rtt{path="[path]",state="[state]"} 123.123
srt_conns_mbps_send_rate{path="[path]",state="[state]"} 123.123
srt_conns_mbps_receive_rate{path="[path]",state="[state]"} 123.123
srt_conns_mbps_link_capacity{path="[path]",state="[state]"} 123.123
srt_conns_bytes_avail_send_buf{path="[path]",state="[state]"} 123
srt_conns_bytes_avail_receive_buf{path="[path]",state="[state]"} 123
srt_conns_mbps_max_bw{path="[path]",state="[state]"} -123
srt_conns_bytes_mss{path="[path]",state="[state]"} 123
srt_conns_packets_send_buf{path="[path]",state="[state]"} 123
srt_conns_bytes_send_buf{path="[path]",state="[state]"} 123
srt_conns_ms_send_buf{path="[path]",state="[state]"} 123
srt_conns_ms_send_tsb_pd_delay{path="[path]",state="[state]"} 123
srt_conns_packets_receive_buf{path="[path]",state="[state]"} 123
srt_conns_bytes_receive_buf{path="[path]",state="[state]"} 123
srt_conns_ms_receive_buf{path="[path]",state="[state]"} 123
srt_conns_ms_receive_tsb_pd_delay{path="[path]",state="[state]"} 123
srt_conns_packets_reorder_tolerance{path="[path]",state="[state]"} 123
srt_conns_packets_received_avg_belated_time{path="[path]",state="[state]"} 123
srt_conns_packets_send_loss_rate{path="[path]",state="[state]"} 123.123
srt_conns_packets_received_loss_rate{path="[path]",state="[state]"} 123.123
srt_conns_outbound_frames_discarded{path="[path]",state="[state]"} 12

# WebRTC sessions
webrtc_sessions{path="[path]",state="[state]"} 1
webrtc_sessions_inbound_bytes{path="[path]",state="[state]"} 1234
webrtc_sessions_inbound_rtp_packets{path="[path]",state="[state]"} 123
webrtc_sessions_inbound_rtp_packets_lost{path="[path]",state="[state]"} 123
webrtc_sessions_inbound_rtp_packets_jitter{path="[path]",state="[state]"} 123
webrtc_sessions_inbound_rtcp_packets{path="[path]",state="[state]"} 123
webrtc_sessions_outbound_bytes{path="[path]",state="[state]"} 187
webrtc_sessions_outbound_rtp_packets{path="[path]",state="[state]"} 123
webrtc_sessions_outbound_rtcp_packets{path="[path]",state="[state]"} 123
webrtc_sessions_outbound_frames_discarded{path="[path]",state="[state]"} 12

# MoQ sessions
moq_sessions{path="[path]",state="[state]"} 1
moq_sessions_inbound_bytes{path="[path]",state="[state]"} 1234
moq_sessions_outbound_bytes{path="[path]",state="[state]"} 187

# Forward destinations
forward_dests{pos="[pos]",path="[path]",type="[type]",state="[state]"} 1
forward_dests_outbound_bytes{pos="[pos]",path="[path]",type="[type]",state="[state]"} 1234
```

Bitrates are not provided directly as metrics because they can be computed from received and sent bytes by any metrics analyzer (for instance Grafana).

Metrics can be filtered by using HTTP query parameters:

- `type=[TYPE]`: show metrics of a certain type only. TYPE can be `paths`, `forward_dests`, `hls_sessions`, `hls_muxers`, `rtsp_conns`, `rtsp_sessions`, `rtsps_conns`, `rtsps_sessions`, `rtmp_conns`, `rtmps_conns`, `srt_conns`, `webrtc_sessions`, `moq_sessions`.
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
- `forward_dest=[ID]` show metrics belonging to a specific forward destination only
- `moq_session=[ID]` show metrics belonging to a specific MoQ session only
