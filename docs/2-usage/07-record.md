# Record streams to disk

## Compatibility matrix

Live streams be recorded and played back with the following file containers and codecs:

| container | codecs                                                                                                                                                                          |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| fMP4      | **Video**: AV1, VP9, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM |
| MPEG-TS   | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3                                            |

## Usage

To record available streams to disk, set the `record` parameter in the configuration file:

```yml
pathDefaults:
  # Record streams to disk.
  record: yes
```

It's also possible to specify additional parameters:

```yml
pathDefaults:
  # Record streams to disk.
  record: yes
  # Path of recording segments.
  # Extension is added automatically.
  # Available variables are %path (path name), %Y %m %d (year, month, day),
  # %H %M %S (hours, minutes, seconds), %f (microseconds), %z (time zone), %s (unix epoch).
  recordPath: ./recordings/%path/%Y-%m-%d_%H-%M-%S-%f
  # Format of recorded segments.
  # Available formats are "fmp4" (fragmented MP4) and "mpegts" (MPEG-TS).
  recordFormat: fmp4
  # fMP4 segments are concatenation of small MP4 files (parts), each with this duration.
  # MPEG-TS segments are concatenation of 188-bytes packets, flushed to disk with this period.
  # When a system failure occurs, the last part gets lost.
  # Therefore, the part duration is equal to the RPO (recovery point objective).
  recordPartDuration: 1s
  # This prevents RAM exhaustion.
  recordMaxPartSize: 50M
  # Minimum duration of each segment.
  recordSegmentDuration: 1h
  # Delete segments after this timespan.
  # Set to 0s to disable automatic deletion.
  recordDeleteAfter: 1d
```

All available recording parameters are listed in the [configuration file](/docs/references/configuration-file).

## Remote upload

To upload recordings to a remote location, you can use _MediaMTX_ together with [rclone](https://github.com/rclone/rclone), a command line tool that provides file synchronization capabilities with a huge variety of services (including S3, FTP, SMB, Google Drive):

1. Download and install [rclone](https://github.com/rclone/rclone).

2. Configure _rclone_:

   ```sh
   rclone config
   ```

3. Place `rclone` into the `runOnInit` and `runOnRecordSegmentComplete` hooks:

   ```yml
   pathDefaults:
     # this is needed to sync segments after a crash.
     # replace myconfig with the name of the rclone config.
     runOnInit: rclone sync -v ./recordings myconfig:/my-path/recordings

     # this is called when a segment has been finalized.
     # replace myconfig with the name of the rclone config.
     runOnRecordSegmentComplete: rclone sync -v --min-age=1ms ./recordings myconfig:/my-path/recordings
   ```

   If you want to delete local segments after they are uploaded, replace `rclone sync` with `rclone move`.
