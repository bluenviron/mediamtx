#!/bin/bash

# Test script for SRT Persistence Feature
# This script demonstrates the SRT persistence functionality

set -e

echo "=== MediaMTX SRT Persistence Test ==="
echo

# Check if required tools are available
command -v ffmpeg >/dev/null 2>&1 || { echo "Error: ffmpeg is required but not installed."; exit 1; }

# Configuration
SRT_PORT=8890
MEDIAMTX_CONFIG="example_srt_persistence.yml"
STREAM_NAME="live_stream"
TEST_DURATION=60

echo "1. Starting MediaMTX with SRT persistence configuration..."
./mediamtx $MEDIAMTX_CONFIG &
MEDIAMTX_PID=$!

# Wait for server to start
sleep 3

echo "2. Publishing test audio stream via SRT..."
echo "   Stream: $STREAM_NAME"
echo "   Duration: ${TEST_DURATION}s"
echo

# Generate a test tone and publish via SRT
ffmpeg -f lavfi -i "sine=frequency=440:duration=$TEST_DURATION" \
       -c:a aac -ar 48000 -ac 2 -b:a 128k \
       -f mpegts "srt://localhost:$SRT_PORT?streamid=publish:$STREAM_NAME" &
FFMPEG_PID=$!

echo "3. You can now connect consumers to test persistence:"
echo "   SRT: srt://localhost:$SRT_PORT?streamid=read:$STREAM_NAME"
echo "   RTSP: rtsp://localhost:8554/$STREAM_NAME"
echo "   HLS: http://localhost:8888/$STREAM_NAME"
echo

echo "4. Test procedure:"
echo "   a) Connect a consumer (e.g., ffplay or VLC)"
echo "   b) Kill this script (Ctrl+C) to simulate publisher disconnection"
echo "   c) Notice that consumers continue receiving silence"
echo "   d) Restart publisher to see seamless reconnection"
echo

echo "Press Ctrl+C to stop the test..."

# Wait for processes
wait $FFMPEG_PID 2>/dev/null || echo "Publisher stopped"

echo
echo "Test completed. Stopping MediaMTX..."
kill $MEDIAMTX_PID 2>/dev/null || true
wait $MEDIAMTX_PID 2>/dev/null || true

echo "Done."