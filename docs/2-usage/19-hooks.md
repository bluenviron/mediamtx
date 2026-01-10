# Hooks

The server allows to specify commands that are executed when a certain event happens, allowing the propagation of events to external software.

## runOnConnect

`runOnConnect` allows to run a command when a client connects to the server:

```yml
# Command to run when a client connects to the server.
# This is terminated with SIGINT when a client disconnects from the server.
# The following environment variables are available:
# * MTX_CONN_TYPE: connection type
# * MTX_CONN_ID: connection ID
# * RTSP_PORT: RTSP server port
runOnConnect: curl http://my-custom-server/webhook?conn_type=$MTX_CONN_TYPE&conn_id=$MTX_CONN_ID
# Restart the command if it exits.
runOnConnectRestart: no
```

## runOnDisconnect

`runOnDisconnect` allows to run a command when a client disconnects from the server:

```yml
# Command to run when a client disconnects from the server.
# Environment variables are the same of runOnConnect.
runOnDisconnect: curl http://my-custom-server/webhook?conn_type=$MTX_CONN_TYPE&conn_id=$MTX_CONN_ID
```

## runOnInit

`runOnInit` allows to run a command when a path is initialized. This can be used to publish a stream when the server is launched:

```yml
paths:
  mypath:
    # Command to run when this path is initialized.
    # This can be used to publish a stream when the server is launched.
    # The following environment variables are available:
    # * MTX_PATH: path name
    # * RTSP_PORT: RTSP server port
    # * G1, G2, ...: regular expression groups, if path name is
    #   a regular expression.
    runOnInit: ffmpeg -i my_file.mp4 -c copy -f rtsp rtsp://localhost:8554/mypath
    # Restart the command if it exits.
    runOnInitRestart: no
```

## runOnDemand

`runOnDemand` allows to run a command when a path is requested by a reader. This can be used to publish a stream on demand:

```yml
pathDefaults:
  # Command to run when this path is requested by a reader
  # and no one is publishing to this path yet.
  # This is terminated with SIGINT when there are no readers anymore.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_QUERY: query parameters (passed by first reader)
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnDemand: ffmpeg -i my_file.mp4 -c copy -f rtsp rtsp://localhost:8554/mypath
  # Restart the command if it exits.
  runOnDemandRestart: no
```

## runOnUnDemand

`runOnUnDemand` allows to run a command when there are no readers anymore:

```yml
pathDefaults:
  # Command to run when there are no readers anymore.
  # Environment variables are the same of runOnDemand.
  runOnUnDemand:
```

## runOnReady

`runOnReady` allows to run a command when a stream is ready to be read:

```yml
pathDefaults:
  # Command to run when the stream is ready to be read, whenever it is
  # published by a client or pulled from a server / camera.
  # This is terminated with SIGINT when the stream is not ready anymore.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_QUERY: query parameters (passed by publisher)
  # * MTX_SOURCE_TYPE: source type
  # * MTX_SOURCE_ID: source ID
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnReady: curl http://my-custom-server/webhook?path=$MTX_PATH&source_type=$MTX_SOURCE_TYPE&source_id=$MTX_SOURCE_ID
  # Restart the command if it exits.
  runOnReadyRestart: no
```

## runOnNotReady

`runOnNotReady` allows to run a command when a stream is not available anymore:

```yml
pathDefaults:
  # Command to run when the stream is not available anymore.
  # Environment variables are the same of runOnReady.
  runOnNotReady: curl http://my-custom-server/webhook?path=$MTX_PATH&source_type=$MTX_SOURCE_TYPE&source_id=$MTX_SOURCE_ID
```

## runOnRead

`runOnRead` allows to run a command when a client starts reading:

```yml
pathDefaults:
  # Command to run when a client starts reading.
  # This is terminated with SIGINT when a client stops reading.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_QUERY: query parameters (passed by reader)
  # * MTX_READER_TYPE: reader type
  # * MTX_READER_ID: reader ID
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnRead: curl http://my-custom-server/webhook?path=$MTX_PATH&reader_type=$MTX_READER_TYPE&reader_id=$MTX_READER_ID
  # Restart the command if it exits.
  runOnReadRestart: no
```

## runOnUnread

`runOnUnread` allows to run a command when a client stops reading:

```yml
pathDefaults:
  # Command to run when a client stops reading.
  # Environment variables are the same of runOnRead.
  runOnUnread: curl http://my-custom-server/webhook?path=$MTX_PATH&reader_type=$MTX_READER_TYPE&reader_id=$MTX_READER_ID
```

## runOnRecordSegmentCreate

`runOnRecordSegmentCreate` allows to run a command when a recording segment is created:

```yml
pathDefaults:
  # Command to run when a recording segment is created.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_SEGMENT_PATH: segment file path
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnRecordSegmentCreate: curl http://my-custom-server/webhook?path=$MTX_PATH&segment_path=$MTX_SEGMENT_PATH
```

## runOnRecordSegmentComplete

`runOnRecordSegmentComplete` allows to run a command when a recording segment is complete:

```yml
pathDefaults:
  # Command to run when a recording segment is complete.
  # The following environment variables are available:
  # * MTX_PATH: path name
  # * MTX_SEGMENT_PATH: segment file path
  # * MTX_SEGMENT_DURATION: segment duration
  # * RTSP_PORT: RTSP server port
  # * G1, G2, ...: regular expression groups, if path name is
  #   a regular expression.
  runOnRecordSegmentComplete: curl http://my-custom-server/webhook?path=$MTX_PATH&segment_path=$MTX_SEGMENT_PATH
```
