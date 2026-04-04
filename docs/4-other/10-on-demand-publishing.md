# On-demand publishing

Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  ondemand:
    runOnDemand: ffmpeg -re -stream_loop -1 -i file.mp4 -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$MTX_PATH
    runOnDemandRestart: yes
```

The command inserted into `runOnDemand` will start only when a client requests the path `ondemand`, therefore the file will start streaming only when requested.
