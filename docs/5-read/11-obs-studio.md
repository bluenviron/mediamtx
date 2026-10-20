# OBS Studio

OBS Studio can read streams from the server by using the [RTSP protocol](04-rtsp.md).

Open OBS, click on _Add Source_, _Media source_, _OK_, uncheck _Local file_, insert in _Input_:

```
rtsp://localhost:8554/stream
```

Then _Ok_.
