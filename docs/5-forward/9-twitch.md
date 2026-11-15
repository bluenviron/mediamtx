# Twitch

Open Twitch, sign in with a valid account, go to _Settings_, _Stream_. Search for the _Primary Stream Key_, copy it somewhere. Then search for _Recommended Ingest Endpoints_, that should be in [this page](https://help.twitch.tv/s/twitch-ingest-recommendation?language=en_US). Copy one of the URLs. Insert the URL and the stream key in _MediaMTX_ in this way:

```yml
paths:
  mypath:
    forward:
      # Use an hashtag to separate the URL from the stream key.
      # "rtmp://ingest.global-contribute.live-video.net/app" was the URL Twitch was reporting last time this documentation was updated. Check that it is still correct.
      # Also replace rtmp:// with rtmps:// in order to enable encryption in-transit.
      - dest: rtmps://ingest.global-contribute.live-video.net/app#streamKey
```
