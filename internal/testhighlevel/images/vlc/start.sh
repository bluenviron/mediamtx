#!/bin/sh -e

cvlc --play-and-exit --no-audio --no-video --sout file/ts:/out/stream.ts -vvv $@ 2>&1 &

COUNTER=0
while true; do
    sleep 1

    if [ $(stat -c "%s" /out/stream.ts) -gt 0 ]; then
        exit 0
    fi

    COUNTER=$(($COUNTER + 1))

    if [ $COUNTER -ge 15 ]; then
        exit 1
    fi
done
