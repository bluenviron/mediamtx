#!/bin/sh -e

mkdir /out
chown user:user /out

CMD="cvlc --play-and-exit --no-audio --no-video --sout file/ts:/out/stream.ts -vvv $@"
su - user -c "$CMD" 2>&1 &

sleep 10

if [ $(stat -c "%s" /out/stream.ts) -gt 0 ]; then
    exit 0
else
    exit 1
fi
