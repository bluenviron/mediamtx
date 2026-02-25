# OBS Studio

OBS Studio can publish to the server in several ways. The recommended one consists in publishing with RTMP.

## OBS Studio and RTMP

In `Settings -> Stream` (or in the Auto-configuration Wizard), use the following parameters:

- Service: `Custom...`
- Server: `rtmp://localhost/mystream`
- Stream key: (empty)

Save the configuration and click `Start streaming`.

The resulting stream will be available on path `/mystream`.

If you want to generate a stream that can be read with WebRTC, open `Settings -> Output -> Recording` and use the following parameters:

- FFmpeg output type: `Output to URL`
- File path or URL: `rtsp://localhost:8554/mystream`
- Container format: `rtsp`
- Check `show all codecs (even if potentially incompatible)`
- Video encoder: `h264_nvenc (libx264)`
- Video encoder settings (if any): `bf=0`
- Audio track: `1`
- Audio encoder: `libopus`

Then use the button `Start Recording` (instead of `Start Streaming`) to start streaming.

## OBS Studio and RTMP, multitrack video

OBS Studio can publish multiple video tracks or renditions at once. Make sure that the OBS Studio version is &ge; 31.0.0. Open `Settings -> Stream` and use the following parameters:

- Service: `Custom...`
- Server: `rtmp://localhost/mystream`
- Stream key: (empty)
- Turn on `Enable Multitrack Video`
- Leave `Maximum Streaming Bandwidth` and `Maximum Video Tracks` to `Auto`
- Turn on `Enable Config Override`
- Fill `Config Override (JSON)` with the following text:

  ```json
  {
    "encoder_configurations": [
      {
        "type": "obs_x264",
        "width": 1920,
        "height": 1080,
        "framerate": {
          "numerator": 30,
          "denominator": 1
        },
        "settings": {
          "rate_control": "CBR",
          "bitrate": 6000,
          "keyint_sec": 2,
          "preset": "veryfast",
          "profile": "high",
          "tune": "zerolatency"
        },
        "canvas_index": 0
      },
      {
        "type": "obs_x264",
        "width": 640,
        "height": 480,
        "framerate": {
          "numerator": 30,
          "denominator": 1
        },
        "settings": {
          "rate_control": "CBR",
          "bitrate": 3000,
          "keyint_sec": 2,
          "preset": "veryfast",
          "profile": "main",
          "tune": "zerolatency"
        },
        "canvas_index": 0
      }
    ],
    "audio_configurations": {
      "live": [
        {
          "codec": "ffmpeg_aac",
          "track_id": 1,
          "channels": 2,
          "settings": {
            "bitrate": 160
          }
        }
      ]
    }
  }
  ```

  This can be adjusted according to specific needs. In particular, the `type` field is used to set the video encoder, and these are the available parameters:
  - `obs_nvenc_av1_tex`: NVIDIA NVENC AV1
  - `obs_nvenc_hevc_tex`: NVIDIA NVENC H265
  - `obs_nvenc_h264_tex`: NVIDIA NVENC H264
  - `av1_texture_amf`: AMD AV1
  - `h265_texture_amf`: AMD H265
  - `h264_texture_amf`: AMD H264
  - `obs_qsv11_av1`: QuickSync AV1
  - `obs_qsv11_v2`: QuickSync H264
  - `obs_x264`: software H264

## OBS Studio and WebRTC

Recent versions of OBS Studio can also publish to the server with the [WebRTC / WHIP protocol](21-webrtc-clients.md) Use the following parameters:

- Service: `WHIP`
- Server: `http://localhost:8889/mystream/whip`

Save the configuration and click `Start streaming`.

The resulting stream will be available on path `/mystream`.
