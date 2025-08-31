# Extract screenshots

You can periodically extract screenshots from available streams by using FFmpeg inside the `runOnReady` hook:

```yml
pathDefaults:
  runOnReady: |
    bash -c "
    while true; do
      mkdir -p $(dirname screenshots/$MTX_PATH)
      ffmpeg -i rtsp://localhost:8554/$MTX_PATH -frames:v 1 -update true -y screenshots/$MTX_PATH.jpg
      sleep 10
    done"
```

Where `10` is the interval between screenshots.
