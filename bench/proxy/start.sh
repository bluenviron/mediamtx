#!/bin/sh -e

PROXY_COUNT=20
PROXY_PROTOCOL=tcp

#####################################################
# source

CONF=""
CONF="${CONF}hlsDisable: yes\n"
CONF="${CONF}rtspAddress: :8555\n"
CONF="${CONF}rtpAddress: :8002\n"
CONF="${CONF}rtcpAddress: :8003\n"
CONF="${CONF}paths:\n"
CONF="${CONF}  all:\n"
echo -e "$CONF" > /source.conf

RTSP_RTMPDISABLE=yes /rtsp-simple-server /source.conf &

sleep 1

ffmpeg -hide_banner -loglevel error \
-re -stream_loop -1 -i /video.mkv -c copy -f rtsp rtsp://localhost:8555/source &

sleep 1

#####################################################
# proxy

CONF=""
CONF="${CONF}hlsDisable: yes\n"
CONF="${CONF}pprof: yes\n"
CONF="${CONF}paths:\n"
for i in $(seq 1 $PROXY_COUNT); do
    CONF="${CONF}  proxy$i:\n"
    CONF="${CONF}    source: rtsp://localhost:8555/source\n"
    CONF="${CONF}    sourceProtocol: $PROXY_PROTOCOL\n"
done
echo -e "$CONF" > /proxy.conf

RTSP_RTMPDISABLE=yes /rtsp-simple-server /proxy.conf &

sleep 5

go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=15
