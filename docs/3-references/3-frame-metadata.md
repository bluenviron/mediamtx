# Frame metadata (SEI / AV1 METADATA OBU)

This document describes the per-frame metadata that MediaMTX can optionally embed into video frames.

## Overview

- **H.264 / H.265**: embedded as **SEI `user_data_unregistered`**, inserted **before the first VCL NAL** in the access unit.
- **AV1**: embedded as **METADATA OBU**, prepended to the temporal unit.
- **Disabled by default**. Enabled per-path via `enableFrameMetadata: true`.

## Binary layout (common to all codecs)

The embedded metadata payload is a single binary blob:

```
[u16 schemaVersion BE][16-byte UUID][CBOR payload]
```

- **`schemaVersion`**: big-endian unsigned 16-bit, currently `1`.
- **`UUID`**: a fixed 16-byte identifier for this metadata schema.
- **`CBOR payload`**: canonical CBOR map with the fields described below.

### Note about the UUID

For H.264/H.265, the SEI `user_data_unregistered` message format *also* contains a 16-byte UUID field.
The implementation therefore includes:

- the UUID **once** as the SEI UUID (per spec), and
- the UUID **again** inside the common binary layout above.

Consumers should validate the inner UUID (the one after `schemaVersion`) to identify the schema reliably.

## CBOR payload (semantics)

CBOR payload is a **map** with string keys. All fields are optional unless stated otherwise.

### `frame_type` (required)

Unsigned integer:

- `0`: key frame (H.264 IDR, H.265 IDR/CRA/BLA, AV1 key/intra-only/switch)
- `1`: P
- `2`: B
- `3`: AV1 inter (non-key, non-show-existing)

### Camera clock timestamps (from RTP/PTS)

These come from the stream clock (RTP/PTS) converted to milliseconds using the codec clock rate.

- **Key frames (`frame_type == 0`)**
  - `utc_ms`: signed/unsigned integer, **camera clock “absolute” in ms** (derived from RTP/PTS)
- **Non-key frames**
  - `dt_ms`: signed integer, **delta ms from the last key frame `utc_ms`**

> NOTE: despite the name `utc_ms`, this is derived from the stream RTP/PTS clock and is not necessarily a wall-clock UTC epoch timestamp.

### Ingest timestamps (server wall clock)

These come from server wall clock (ingest time).

- **Key frames (`frame_type == 0`)**
  - `ingest_utc_ms`: signed/unsigned integer, **UTC milliseconds since Unix epoch**
- **Non-key frames**
  - `ingest_dt_ms`: signed integer, **delta ms from the last key frame `ingest_utc_ms`**

### PTZ metadata (version-gated)

PTZ fields are included **only when `ptz_ver` changes** (to avoid repeating unchanged PTZ on every frame).

- `ptz_ver`: unsigned integer
- `pan`: float (typically float32)
- `tilt`: float
- `zoom`: float

## Codec-specific embedding details

### H.264 / H.265

- Message type: `user_data_unregistered` (payloadType `5`)
- Inserted as a SEI NAL:
  - H.264 NAL unit type `6`
  - H.265 NAL unit type `39` (PREFIX_SEI_NUT)
- SEI RBSP uses **emulation prevention bytes** as required.
- Inserted **before the first VCL NAL** in the access unit.

### AV1

- Inserted as a **METADATA OBU** (`obu_type = 15`) with an OBU size field (LEB128).
- No emulation prevention is applied.

## Extraction utilities

This repo includes two utilities built around this metadata:

### Mode A: MP4 → MP4 with burned overlay (Go + ffmpeg)

Tool: `cmd/mp4metaoverlay`

What it does:

- Parses the MP4 and extracts the per-frame metadata
- Writes a temporary ASS subtitle file
- Runs `ffmpeg` to **burn** the subtitles into the video (video is re-encoded; audio is copied)

Usage:

```bash
go run ./cmd/mp4metaoverlay -in recordings/in.mp4 -out out.mp4
```

Useful flags:

- `-ffmpeg`: path to the `ffmpeg` binary (default: `ffmpeg`)
- `-track`: which video track to use (0-based, default picks first video track)
- `-vcodec/-crf/-preset`: encoder settings for the re-encoded video

Notes:

- Burning an overlay requires re-encoding the video stream.
- This is intended for quick inspection/debug; for “metadata export”, use Mode B.

Python alternative (recommended for sharing with other teams):

```bash
python3 scripts/mp4_burn_metadata_overlay.py --in recordings/in.mp4 --out out.mp4
```

### Mode B: MP4 → JSONL (+ optional PNG frames) extraction (Python + ffmpeg/ffprobe)

Tool: `scripts/mp4_extract_metadata.py`

What it does:

- Uses `ffprobe` to list MP4 video packets with PTS/DTS
- Uses `ffmpeg` bitstream filters to get an elementary stream suitable for parsing
- Extracts the raw metadata and emits a **JSONL** file that maps each packet to:
  - packet PTS/PTS time
  - decoded CBOR map (`meta`)
  - reconstructed absolute values (`meta_abs.cam_abs_ms`, `meta_abs.ingest_abs_ms`)
- Optionally dumps decoded frames to PNG named by PTS (to join frame↔metadata by filename)

Usage (metadata only):

```bash
python3 scripts/mp4_extract_metadata.py --in recordings/in.mp4 --out-jsonl out.jsonl
```

Usage (metadata + raw frames):

```bash
python3 scripts/mp4_extract_metadata.py \
  --in recordings/in.mp4 \
  --out-jsonl out.jsonl \
  --dump-frames \
  --frames-dir frames
```

Notes:

- The JSONL includes both the “camera clock” timeline and the ingest wall-clock timeline.
- H.264/H.265 extraction is the primary supported path; AV1 is currently best-effort.