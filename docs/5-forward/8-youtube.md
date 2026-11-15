# YouTube

Open YouTube, sign in with a valid account, search for the _Create_, _Go Live_ button, click it. In the next page, search for _Default stream key_, copy the key somewhere, then search for the _Stream URL_ section and copy the URL. Insert the stream URL and the stream key in _MediaMTX_ in this way:

```yml
paths:
  mypath:
    forward:
      # Use an hashtag to separate the URL from the stream key.
      # "a.rtmp.youtube.com/live2" was the URL YouTube was reporting last time this documentation was updated. Check that it is still correct.
      # Also replace rtmp:// with rtmps:// and rtmp with rtmps in the URI in order to enable encryption in-transit.
      - dest: rtmps://a.rtmps.youtube.com/live2#streamKey
```

**Warning**: YouTube requires streams to have both a video and an audio track. Video-only streams are silently rejected.
