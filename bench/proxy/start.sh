#!/bin/sh -e

PROXY_COUNT=20
PROXY_PROTOCOL=tcp

#####################################################
# source

CONF=""
CONF="${CONF}rtspPort: 8555\n"
CONF="${CONF}rtpPort: 8002\n"
CONF="${CONF}rtcpPort: 8003\n"
echo -e "$CONF" > /source.conf

/rtsp-simple-server /source.conf &

sleep 1

ffmpeg -hide_banner -loglevel error \
-re -stream_loop -1 -i /video.mkv -c copy -f rtsp rtsp://localhost:8555/source &

sleep 1

#####################################################
# proxy

CONF=""
CONF="${CONF}pprof: yes\n"
CONF="${CONF}paths:\n"
for i in $(seq 1 $PROXY_COUNT); do
    CONF="${CONF}  proxy$i:\n"
    CONF="${CONF}    source: rtsp://localhost:8555/source\n"
    CONF="${CONF}    sourceProtocol: $PROXY_PROTOCOL\n"
done
echo -e "$CONF" > /proxy.conf

/rtsp-simple-server /proxy.conf &

sleep 5

go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=15
