# SRT Stream Persistence Feature

This feature allows MediaMTX to maintain continuous stream output to consumers even when SRT publishers disconnect temporarily. When enabled, the server generates silence (audio) to keep the stream alive until the publisher reconnects or the path is explicitly destroyed.

## Overview

The SRT Persistence feature addresses a common use case in live streaming where network interruptions can cause brief disconnections between publishers and the media server. Instead of terminating the stream and forcing all consumers to reconnect, MediaMTX can now maintain stream continuity by generating silence during disconnection periods.

## Configuration

### Static Configuration (YAML)

Add the following configuration to your path settings:

```yaml
paths:
  your_stream_name:
    source: publisher
    srtPersistOnDisconnect: yes  # Enable SRT persistence
    srtPublishPassphrase: "optional_passphrase"
```

Or in the global path defaults:

```yaml
pathDefaults:
  srtPersistOnDisconnect: yes  # Enable for all paths by default
```

### Dynamic Configuration (API)

The `srtPersistOnDisconnect` field is fully supported via the REST API for dynamic path management:

```bash
# Create path with SRT persistence via API
curl -X POST "http://localhost:9997/v3/config/paths/add/my_stream" \
     -H "Content-Type: application/json" \
     -d '{
       "source": "publisher",
       "srtPersistOnDisconnect": true,
       "srtPublishPassphrase": "optional_passphrase"
     }'

# Update existing path to enable/disable persistence
curl -X PATCH "http://localhost:9997/v3/config/paths/patch/my_stream" \
     -H "Content-Type: application/json" \
     -d '{
       "srtPersistOnDisconnect": false
     }'

# Get path configuration to verify settings
curl "http://localhost:9997/v3/config/paths/get/my_stream"
```

### Node.js/JavaScript Integration

```javascript
// Example for Waystream SRTManager integration
const props = {
    record: true,
    recordPath: `${process.env.RECORDINGS_DIR}%path/%Y-%m-%d_%H-%M-%S-%f`,
    recordFormat: 'fmp4',
    runOnReady: ffmpegCmd,
    srtPersistOnDisconnect: true,  // Enable persistence
    srtPublishPassphrase: process.env.SRT_PASSPHRASE || ""
};

axios.post(`${process.env.MUXER_API_URL}config/paths/add/${streamPath}`, 
           JSON.stringify(props));
```

## Behavior

### When SRT Publisher Connects
- Normal stream establishment
- Media data flows from publisher to consumers
- Stream is marked as ready and available

### When SRT Publisher Disconnects (with persistence enabled)
- Stream remains active and ready
- Silence generator starts automatically
- Audio tracks continue outputting silence frames
- Video tracks (if any) stop but don't terminate the stream
- Consumers remain connected and continue receiving data
- Log message: "SRT publisher disconnected, maintaining stream with silence generation"

### When SRT Publisher Reconnects
- Silence generator stops automatically
- Normal media data flow resumes
- No interruption to consumers
- Log message: "SRT publisher reconnecting, stopping silence generation"

### When SRT Publisher Disconnects (with persistence disabled)
- Standard MediaMTX behavior
- Stream terminates immediately
- All consumers are disconnected
- Path becomes unavailable until next publisher connects

## Supported Audio Formats

The silence generator supports the following audio formats:

- **Opus**: Generates DTX (Discontinuous Transmission) frames
- **AAC-LC (MPEG4Audio)**: Generates silent audio frames
- **G.711 (µ-law/A-law)**: Generates appropriate silence values
- **Generic formats**: Falls back to zero-filled frames

## Use Cases

1. **Live Broadcasting**: Maintain stream continuity during brief network issues
2. **Conference Streaming**: Keep audio channels alive during temporary disconnections  
3. **Remote Production**: Ensure uninterrupted output during encoder restarts
4. **Failover Scenarios**: Bridge the gap between primary and backup publishers

## Technical Implementation

- **Silence Generation**: Runs on 20ms intervals (typical audio frame duration)
- **Format-Aware**: Generates appropriate silence for each audio codec
- **Lightweight**: Minimal CPU impact during silence generation
- **Thread-Safe**: Safe concurrent operation with normal streaming

## Configuration Examples

### Basic Setup
```yaml
srt: yes
srtAddress: :8890

paths:
  live:
    source: publisher
    srtPersistOnDisconnect: yes
```

### Advanced Setup with Security
```yaml
paths:
  secure_stream:
    source: publisher
    srtPersistOnDisconnect: yes
    srtPublishPassphrase: "my_secret_key_123456"
    record: yes
    recordPath: "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f"
```

### Global Default
```yaml
pathDefaults:
  source: publisher
  srtPersistOnDisconnect: yes

paths:
  stream1: {}  # Inherits persistence
  stream2: 
    srtPersistOnDisconnect: no  # Override to disable
```

## Monitoring and Logging

Enable appropriate log levels to monitor the feature:

```yaml
logLevel: info  # Will show persistence state changes
```

Log messages to watch for:
- `SRT publisher disconnected, maintaining stream with silence generation`
- `SRT publisher reconnecting, stopping silence generation`
- `silence generator started`
- `silence generator stopped`

## Limitations

1. **SRT Only**: This feature is specific to SRT publishers
2. **Audio Focus**: Primarily designed for audio streams (video stops during disconnection)
3. **Path Lifecycle**: Stream persists until path is explicitly destroyed or server shutdown
4. **Memory Usage**: Keeps stream objects active during disconnection periods

## Migration

This feature is backward compatible:
- Default value is `false` (disabled)
- Existing configurations continue to work unchanged
- No performance impact when disabled

## Troubleshooting

### Stream Not Persisting
- Verify `srtPersistOnDisconnect: yes` in path configuration
- Check that the publisher is actually SRT (not RTSP/RTMP)
- Ensure path source is set to "publisher"

### High CPU Usage
- Monitor silence generation frequency
- Consider reducing number of concurrent persistent streams
- Check audio format complexity

### Memory Leaks
- Ensure paths are properly destroyed when no longer needed
- Monitor stream cleanup in server logs
- Restart server periodically in high-usage scenarios