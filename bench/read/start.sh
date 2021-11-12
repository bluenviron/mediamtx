#!/bin/sh -e

READER_COUNT=20
READER_PROTOCOL=tcp

#####################################################
# source

CONF=""
CONF="${CONF}pprof: yes\n"
CONF="${CONF}paths:\n"
CONF="${CONF}  all:\n"
echo -e "$CONF" > /source.conf

/rtsp-simple-server /source.conf &

sleep 1

ffmpeg -re -stream_loop -1 -i /video.mkv -c copy -f rtsp rtsp://localhost:8554/source &

sleep 1

#####################################################
# readers

for i in $(seq 1 $READER_COUNT); do
    ffmpeg  -hide_banner -loglevel error \
    -rtsp_transport $READER_PROTOCOL \
    -i rtsp://localhost:8554/source -c copy -f mpegts -y /dev/null &
done

sleep 5

go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=15
