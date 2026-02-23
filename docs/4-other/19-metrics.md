# Extract metrics

_MediaMTX_ provides several metrics through a dedicated HTTP server, in a format compatible with [Prometheus](https://prometheus.io/).

This server can be enabled by setting `metrics: yes` in the configuration.

Metrics can be extracted with Prometheus or with a simple HTTP request:

```
curl localhost:9998/metrics
```

Obtaining:

```ini
# metrics of every path
paths{name="[path_name]",state="[state]"} 1
paths_bytes_received{name="[path_name]",state="[state]"} 1234
paths_bytes_sent{name="[path_name]",state="[state]"} 1234
paths_readers{name="[path_name]",state="[state]"} 1234

# metrics of every HLS muxer
hls_muxers{name="[name]"} 1
hls_muxers_bytes_sent{name="[name]"} 187

# metrics of every RTSP connection
rtsp_conns{id="[id]"} 1
rtsp_conns_bytes_received{id="[id]"} 1234
rtsp_conns_bytes_sent{id="[id]"} 187

# metrics of every RTSP session
rtsp_sessions{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtsp_sessions_bytes_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtsp_sessions_bytes_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
rtsp_sessions_rtp_packets_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_rtp_packets_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_rtp_packets_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_rtp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_rtp_packets_jitter{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_rtcp_packets_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_rtcp_packets_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsp_sessions_rtcp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123

# metrics of every RTSPS connection
rtsps_conns{id="[id]"} 1
rtsps_conns_bytes_received{id="[id]"} 1234
rtsps_conns_bytes_sent{id="[id]"} 187

# metrics of every RTSPS session
rtsps_sessions{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtsps_sessions_bytes_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtsps_sessions_bytes_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
rtsps_sessions_rtp_packets_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_rtp_packets_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_rtp_packets_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_rtp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_rtp_packets_jitter{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_rtcp_packets_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_rtcp_packets_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
rtsps_sessions_rtcp_packets_in_error{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123

# metrics of every RTMP connection
rtmp_conns{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtmp_conns_bytes_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtmp_conns_bytes_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187

# metrics of every RTMPS connection
rtmps_conns{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
rtmps_conns_bytes_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
rtmps_conns_bytes_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187

# metrics of every SRT connection
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
srt_conns_bytes_send_drop{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_drop{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_bytes_received_undecrypt{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_us_packets_send_period{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_packets_flow_window{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_flight_size{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_ms_rtt{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123.123
srt_conns_mbps_send_rate{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
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
srt_conns_ms_receive_tsb_pd_delay{iid="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_reorder_tolerance{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_avg_belated_time{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_send_loss_rate{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
srt_conns_packets_received_loss_rate{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123

# metrics of every WebRTC session
webrtc_sessions{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1
webrtc_sessions_bytes_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 1234
webrtc_sessions_bytes_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 187
webrtc_sessions_rtp_packets_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_rtp_packets_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_rtp_packets_lost{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_rtp_packets_jitter{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_rtcp_packets_received{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
webrtc_sessions_rtcp_packets_sent{id="[id]",path="[path]",remoteAddr="[remoteAddr]",state="[state]"} 123
```

Bitrates are not provided directly as metrics because they can be computed from received and sent bytes by any metrics analyzer (i.e. Grafana).

Metrics can be filtered by using HTTP query parameters:

- `type=[TYPE]`: show metrics of a certain type only (where TYPE can be `paths`, `hls_muxers`, `rtsp_conns`, `rtsp_sessions`, `rtsps_conns`, `rtsps_sessions`, `rtmp_conns`, `rtmps_conns`, `srt_conns`, `webrtc_sessions`)
- `path=[PATH]`: show metrics belonging to a specific path only
- `hls_muxer=[PATH]`: show metrics belonging to a specific HLS muxer only
- `rtsp_conn=[ID]` show metrics belonging to a specific RTSP connection only
- `rtsp_session=[SESSION]`: show metrics belonging to a specific RTSP session only
- `rtsps_conn=[ID]` show metrics belonging to a specific RTSPS connection only
- `rtsps_session=[SESSION]`: show metrics belonging to a specific RTSPS session only
- `rtmp_conn=[ID]` show metrics belonging to a specific RTMP connection only
- `rtmps_conn=[ID]` show metrics belonging to a specific RTMPS connection only
- `srt_conn=[ID]` show metrics belonging to a specific SRT connection only
- `webrtc_session=[ID]` show metrics belonging to a specific WebRTC session only
