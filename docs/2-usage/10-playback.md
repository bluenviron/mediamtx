# Playback recorded streams

Existing recordings can be served to users through a dedicated HTTP server, that can be enabled inside the configuration:

```yml
playback: yes
playbackAddress: :9996
```

The server provides an endpoint to list recorded timespans:

```
http://localhost:9996/list?path=[mypath]&start=[start]&end=[end]
```

Where:

- [mypath] is the name of a path
- [start] (optional) is the start date in [RFC3339 format](https://www.utctime.net/)
- [end] (optional) is the end date in [RFC3339 format](https://www.utctime.net/)

The server will return a list of timespans in JSON format:

```json
[
  {
    "start": "2006-01-02T15:04:05Z07:00",
    "duration": 60.0,
    "url": "http://localhost:9996/get?path=[mypath]&start=2006-01-02T15%3A04%3A05Z07%3A00&duration=60.0"
  },
  {
    "start": "2006-01-02T15:07:05Z07:00",
    "duration": 32.33,
    "url": "http://localhost:9996/get?path=[mypath]&start=2006-01-02T15%3A07%3A05Z07%3A00&duration=32.33"
  }
]
```

The server provides an endpoint to download recordings:

```
http://localhost:9996/get?path=[mypath]&start=[start]&duration=[duration]&format=[format]
```

Where:

- [mypath] is the path name
- [start] is the start date in [RFC3339 format](https://www.utctime.net/)
- [duration] is the maximum duration of the recording in seconds
- [format] (optional) is the output format of the stream. Available values are "fmp4" (default) and "mp4"

All parameters must be [url-encoded](https://www.urlencoder.org/). For instance:

```
http://localhost:9996/get?path=mypath&start=2024-01-14T16%3A33%3A17%2B00%3A00&duration=200.5
```

The resulting stream uses the fMP4 format, that is natively compatible with any browser, therefore its URL can be directly inserted into a \<video> tag:

```html
<video controls>
  <source
    src="http://localhost:9996/get?path=[mypath]&start=[start_date]&duration=[duration]"
    type="video/mp4"
  />
</video>
```

The fMP4 format may offer limited compatibility with some players. To fix the issue, it's possible to use the standard MP4 format, by adding `format=mp4` to a `/get` request:

```
http://localhost:9996/get?path=[mypath]&start=[start_date]&duration=[duration]&format=mp4
```
