# OBS Studio

OBS Studio can publish streams to the server in several ways. The recommended one consists in publishing with RTMP.

## OBS Studio and RTMP

### Standard

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

### Multitrack video

OBS Studio can publish multiple video tracks or renditions at once (simulcast). Make sure that the OBS Studio version is &ge; 31.0.0. Open `Settings -> Stream` and use the following parameters:

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

Save the configuration and click `Start streaming`.

The resulting stream will be available on path `/mystream`.

### Encryption (RTMPS)

When publishing streams to _MediaMTX_ with RTMP, you can encrypt streams in transit by using the encrypted variant of RTMP (RTMPS). This can be enabled by using the `rtmps` scheme and the 1936 port:

```
rtmps://localhost:1936/mystream
```

Make sure that RTMP encryption is allowed in _MediaMTX_ (`rtmpEncryption: "optional"`).

OBS Studio requires _MediaMTX_ to use a TLS certificate signed by a public certificate authority and silently rejects self-signed certificates. You can either buy a certificate from a public certificate authority or create a local certificate authority and use it to generate the server certificate and validate it on the OBS Studio machine, by following these instructions:

1. Create the key pair of the local certificate authority:

   ```sh
   openssl req \
   -x509 \
   -nodes \
   -days 3650 \
   -newkey rsa:4096 \
   -keyout myca.key \
   -out myca.crt \
   -subj "/O=myca/CN=myca"
   ```

2. Use the key pair to create the server certificate. Replace `localhost` with the domain name OBS Studio will use to connect to the server:

   ```sh
   openssl req \
   -newkey rsa:4096 \
   -nodes \
   -keyout server.key \
   -CA myca.crt \
   -CAkey myca.key \
   -subj "/CN=localhost" \
   -x509 \
   -days 3650 \
   -out server.crt
   ```

   You must use a domain name to connect to the server, not an IP address. If you do not have a domain name, edit the `/etc/hosts` file of the OBS Studio machine and associate a dummy domain name to the IP address of the server.

3. Put the newly-generated `server.key` and `server.cert` on the _MediaMTX_ machine, in the same folder of the _MediaMTX_ executable, and check that the configuration points to them.

4. Install the public key (`ca.crt`) of the local certificate authority on the OBS Studio machine.

   If you are using Linux, this can be accomplished with these commands:

   ```sh
   sudo mkdir -p /usr/local/share/ca-certificates
   sudo cp myca.crt /usr/local/share/ca-certificates/
   sudo update-ca-certificates
   ```

   WARNING: this will still not work when OBS Studio is installed with Flatpak, since Flatpak isolates OBS Studio from the host and prevents it from reading the `ca-certificates` folder. Install OBS Studio through another mean (snap / ppa / .deb).

   If you are using Windows, this can be accomplished with the command:

   ```sh
   certutil -addstore "Root" myca.crt
   ```

## OBS Studio and WebRTC

### Standard

Recent versions of OBS Studio can also publish streams to the server with the [WebRTC / WHIP protocol](03-webrtc-clients.md) Use the following parameters:

- Service: `WHIP`
- Server: `http://localhost:8889/mystream/whip`

Save the configuration and click `Start streaming`.

The resulting stream will be available on path `/mystream`.

### Multitrack video

OBS Studio can publish multiple video tracks or renditions at once (simulcast) with WebRTC / WHIP too. Make sure that the OBS Studio version is &ge; 32.1.0. Open `Settings -> Stream` and use the following parameters:

- Service: `WHIP`
- Server: `http://localhost:8889/mystream/whip`
- Simulcast, Total Layers: `2` (or greater)

Currently it's not possible to change resolution or bitrate (or canvas) of renditions, since quality of secondary renditions is hardcoded as a percentage of the main one. You can find details on the [OBS documentation](https://obsproject.com/kb/whip-streaming-guide).

Save the configuration and click `Start streaming`.

The resulting stream will be available on path `/mystream`.

### Encryption

When publishing streams to _MediaMTX_ with WebRTC, you can encrypt the WebRTC handshake by using the HTTPS-based variant of WHIP. This can be enabled by using the `https` scheme:

```
https://localhost:8889/mystream
```

Make sure that WebRTC encryption is enabled in _MediaMTX_ (`webrtcEncryption: true`).

OBS Studio requires _MediaMTX_ to use a TLS certificate signed by a public certificate authority and silently rejects self-signed certificates. You can either buy a certificate from a public certificate authority or create a local certificate authority and use it to generate the server certificate and validate it on the OBS Studio machine. Instructions are reported in [OBS Studio and RTMP, encryption (RTMPS)](#encryption-rtmps).
