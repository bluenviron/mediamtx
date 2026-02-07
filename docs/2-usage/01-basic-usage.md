# Basic usage

1. [Publish](publish) a stream. For instance, you can publish a stream from a MP4 file with _FFmpeg_:

   ```sh
   ffmpeg -re -stream_loop -1 -i file.mp4 -c copy \
   -f rtsp rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```sh
   gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream filesrc location=file.mp4 \
   ! qtdemux name=d d.video_0 ! queue ! s.sink_0 d.audio_0 ! queue ! s.sink_1
   ```

2. [Read](read) the stream. For instance, you can read the stream with _VLC_:

   ```sh
   vlc --network-caching=50 rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```sh
   gst-play-1.0 rtsp://localhost:8554/mystream
   ```

   or _FFmpeg_:

   ```sh
   ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
   ```
