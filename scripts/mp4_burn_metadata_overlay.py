#!/usr/bin/env python3
"""
Burn MediaMTX per-frame metadata into an MP4 as a text overlay using ffmpeg.

Pipeline:
  1) ffmpeg: produce an elementary stream suitable for parsing (AUD inserted for H264/H265)
  2) parse SEI metadata and build an ASS subtitle file
  3) ffmpeg: burn the ASS overlay into video (re-encodes video; copies audio)

Timing note:
  Overlay event timestamps are derived ONLY from metadata (camera clock):
    - key frames: utc_ms
    - non-key: dt_ms (reconstructed to cam_abs_ms)
  ASS start times are relative to the first frame's cam_abs_ms.

Stdlib-only (includes tiny CBOR decoder + SEI parsing similar to mp4_extract_metadata.py).
"""

from __future__ import annotations

import argparse
import datetime
import json
import os
import subprocess
import tempfile
from typing import Any, Dict, List, Optional, Tuple


SCHEMA_VERSION = 1
UUID16 = bytes.fromhex("1d536d9a2b2d4418933e2a3cf80f605c")


def run_cmd(cmd: List[str]) -> bytes:
    p = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if p.returncode != 0:
        raise RuntimeError(
            "command failed:\n"
            f"  cmd: {' '.join(cmd)}\n"
            f"  exit: {p.returncode}\n"
            f"  stderr:\n{p.stderr.decode('utf-8', errors='replace')}"
        )
    return p.stdout


def ffprobe_json(ffprobe: str, in_path: str, args: List[str]) -> Dict[str, Any]:
    out = run_cmd([ffprobe, "-v", "error", "-print_format", "json"] + args + [in_path])
    return json.loads(out.decode("utf-8"))


def detect_video_codec(ffprobe: str, in_path: str, stream_index: int) -> str:
    data = ffprobe_json(ffprobe, in_path, ["-select_streams", f"v:{stream_index}", "-show_streams"])
    streams = data.get("streams") or []
    if not streams:
        raise RuntimeError("no video stream found")
    codec = streams[0].get("codec_name")
    if not codec:
        raise RuntimeError("ffprobe missing codec_name")
    return str(codec)


def ffmpeg_bitstream(ffmpeg: str, in_path: str, stream_index: int, codec: str) -> bytes:
    if codec == "h264":
        bsf = "h264_mp4toannexb,h264_metadata=aud=insert"
        fmt = "h264"
    elif codec in ("hevc", "h265"):
        bsf = "hevc_mp4toannexb,hevc_metadata=aud=insert"
        fmt = "hevc"
    else:
        raise RuntimeError(f"unsupported codec for overlay: {codec} (supported: h264, hevc)")

    return run_cmd(
        [
            ffmpeg,
            "-v",
            "error",
            "-i",
            in_path,
            "-map",
            f"0:v:{stream_index}",
            "-c:v",
            "copy",
            "-bsf:v",
            bsf,
            "-f",
            fmt,
            "pipe:1",
        ]
    )


def find_start_code(b: bytes, start: int) -> Tuple[int, int]:
    for i in range(start, len(b) - 3):
        if i + 4 <= len(b) and b[i : i + 4] == b"\x00\x00\x00\x01":
            return i, 4
        if b[i : i + 3] == b"\x00\x00\x01":
            return i, 3
    return -1, 0


def split_annexb_nalus(b: bytes) -> List[bytes]:
    nalus: List[bytes] = []
    i = 0
    while True:
        pos, ln = find_start_code(b, i)
        if pos < 0:
            break
        j = pos + ln
        npos, _ = find_start_code(b, j)
        if npos < 0:
            npos = len(b)
        if j < npos:
            nalus.append(b[j:npos])
        i = npos
    return nalus


def group_aus_by_aud_h264(nalus: List[bytes]) -> List[List[bytes]]:
    aus: List[List[bytes]] = []
    cur: List[bytes] = []
    for n in nalus:
        if not n:
            continue
        ntype = n[0] & 0x1F
        if ntype == 9:  # AUD
            if cur:
                aus.append(cur)
                cur = []
            continue
        cur.append(n)
    if cur:
        aus.append(cur)
    return aus


def group_aus_by_aud_h265(nalus: List[bytes]) -> List[List[bytes]]:
    aus: List[List[bytes]] = []
    cur: List[bytes] = []
    for n in nalus:
        if len(n) < 2:
            continue
        ntype = (n[0] >> 1) & 0x3F
        if ntype == 35:  # AUD_NUT
            if cur:
                aus.append(cur)
                cur = []
            continue
        cur.append(n)
    if cur:
        aus.append(cur)
    return aus


def remove_epb(rbsp: bytes) -> bytes:
    out = bytearray()
    zeros = 0
    for by in rbsp:
        if zeros >= 2 and by == 0x03:
            zeros = 0
            continue
        out.append(by)
        if by == 0x00:
            zeros += 1
        else:
            zeros = 0
    return bytes(out)


def decode_sei_value(b: bytes, off: int) -> Tuple[int, int]:
    val = 0
    while True:
        if off >= len(b):
            raise ValueError("sei value truncated")
        by = b[off]
        off += 1
        val += by
        if by != 0xFF:
            return val, off


def extract_user_data_unregistered_from_sei_h264(nalu: bytes) -> Optional[bytes]:
    if len(nalu) < 2 or (nalu[0] & 0x1F) != 6:
        return None
    rbsp = remove_epb(nalu[1:])
    off = 0
    while off < len(rbsp):
        if len(rbsp) - off == 1 and rbsp[off] == 0x80:
            break
        pt, off = decode_sei_value(rbsp, off)
        ps, off = decode_sei_value(rbsp, off)
        if off + ps > len(rbsp):
            return None
        payload = rbsp[off : off + ps]
        off += ps
        if pt == 5 and len(payload) >= 16:
            return payload[16:]
    return None


def extract_user_data_unregistered_from_sei_h265(nalu: bytes) -> Optional[bytes]:
    if len(nalu) < 3:
        return None
    ntype = (nalu[0] >> 1) & 0x3F
    if ntype != 39:  # PREFIX_SEI_NUT
        return None
    rbsp = remove_epb(nalu[2:])
    off = 0
    while off < len(rbsp):
        if len(rbsp) - off == 1 and rbsp[off] == 0x80:
            break
        pt, off = decode_sei_value(rbsp, off)
        ps, off = decode_sei_value(rbsp, off)
        if off + ps > len(rbsp):
            return None
        payload = rbsp[off : off + ps]
        off += ps
        if pt == 5 and len(payload) >= 16:
            return payload[16:]
    return None


def parse_binary_payload(b: bytes) -> Optional[Tuple[int, bytes, bytes]]:
    if len(b) < 2 + 16:
        return None
    ver = int.from_bytes(b[0:2], "big")
    uid = b[2:18]
    cbor = b[18:]
    return ver, uid, cbor


# ---- Minimal CBOR decoding (only what our schema emits) ----


def cbor_read_u(b: bytes, off: int, ai: int) -> Tuple[int, int]:
    if ai < 24:
        return ai, off
    if ai == 24:
        return b[off], off + 1
    if ai == 25:
        return int.from_bytes(b[off : off + 2], "big"), off + 2
    if ai == 26:
        return int.from_bytes(b[off : off + 4], "big"), off + 4
    if ai == 27:
        return int.from_bytes(b[off : off + 8], "big"), off + 8
    raise ValueError("unsupported additional info")


def cbor_decode(b: bytes, off: int = 0) -> Tuple[Any, int]:
    if off >= len(b):
        raise ValueError("cbor truncated")
    ib = b[off]
    off += 1
    major = ib >> 5
    ai = ib & 0x1F

    if major == 0:
        u, off = cbor_read_u(b, off, ai)
        return u, off
    if major == 1:
        u, off = cbor_read_u(b, off, ai)
        return -1 - u, off
    if major == 3:
        ln, off = cbor_read_u(b, off, ai)
        s = b[off : off + ln].decode("utf-8")
        return s, off + ln
    if major == 5:
        ln, off = cbor_read_u(b, off, ai)
        m: Dict[str, Any] = {}
        for _ in range(ln):
            k, off = cbor_decode(b, off)
            v, off = cbor_decode(b, off)
            if not isinstance(k, str):
                raise ValueError("non-string key")
            m[k] = v
        return m, off
    if major == 7:
        if ai == 26:  # float32
            import struct

            (f,) = struct.unpack(">f", b[off : off + 4])
            return float(f), off + 4
        if ai == 27:  # float64
            import struct

            (f,) = struct.unpack(">d", b[off : off + 8])
            return float(f), off + 8
        if ai == 20:
            return False, off
        if ai == 21:
            return True, off
        if ai == 22:
            return None, off
        raise ValueError("unsupported simple/float")

    raise ValueError(f"unsupported major type {major}")


def decode_cbor_map(b: bytes) -> Dict[str, Any]:
    v, _ = cbor_decode(b, 0)
    if not isinstance(v, dict):
        raise ValueError("cbor payload is not a map")
    return v


def parse_one_metadata(raw: Optional[bytes]) -> Optional[Dict[str, Any]]:
    if not raw:
        return None
    parsed = parse_binary_payload(raw)
    if parsed is None:
        return None
    ver, uid, cbor = parsed
    if ver != SCHEMA_VERSION or uid != UUID16:
        return None
    return decode_cbor_map(cbor)


def reconstruct_absolute(m: Dict[str, Any], state: Dict[str, Optional[int]]) -> Dict[str, Any]:
    out = dict(m)
    ft = int(m.get("frame_type", 255))
    if ft == 0:
        cam = int(m.get("utc_ms", 0))
        ing = int(m.get("ingest_utc_ms", 0))
        state["cam_key"] = cam
        state["ing_key"] = ing
        out["cam_abs_ms"] = cam
        out["ingest_abs_ms"] = ing
        return out
    cam_key = state.get("cam_key") or 0
    ing_key = state.get("ing_key") or 0
    dt = int(m.get("dt_ms", 0))
    idt = int(m.get("ingest_dt_ms", 0))
    out["cam_abs_ms"] = cam_key + dt
    out["ingest_abs_ms"] = ing_key + idt
    return out


def overlay_text(m: Dict[str, Any], abs_m: Dict[str, Any]) -> str:
    cam_abs = int(abs_m.get("cam_abs_ms", 0))
    ing_abs = int(abs_m.get("ingest_abs_ms", 0))
    cam_hr = human_time_ms(cam_abs, base_ms=int(abs_m.get("_base_cam_ms", 0)))
    # Ingest display is shifted by +02:00 for this deployment.
    ing_hr = human_time_ms(ing_abs, base_ms=int(abs_m.get("_base_ing_ms", 0)), tz_offset_minutes=120)

    ft = int(m.get("frame_type", 255))
    # Use a single fixed template for all frames to avoid box-size jumps.
    ver = m.get("version", "")
    s = f"metadata v={ver} FT={ft} cam={cam_hr} ingest={ing_hr}"
    if "ptz_ver" in m:
        s += f" ptz_ver={m.get('ptz_ver')}"
        if "pan" in m:
            s += f" pan={m.get('pan'):.3f}"
        if "tilt" in m:
            s += f" tilt={m.get('tilt'):.3f}"
        if "zoom" in m:
            s += f" zoom={m.get('zoom'):.3f}"
    return s

def human_time_ms(ms: int, base_ms: int = 0, tz_offset_minutes: int = 0) -> str:
    # If it looks like epoch milliseconds, render UTC wall clock time, optionally with a fixed offset.
    # Otherwise render as offset since base_ms.
    if ms >= 1_000_000_000_000:  # ~2001-09-09 in ms
        dt = datetime.datetime.fromtimestamp(ms / 1000.0, tz=datetime.timezone.utc)
        if tz_offset_minutes:
            dt = dt + datetime.timedelta(minutes=tz_offset_minutes)
            # Show local wall-clock only (no "Z" / "+02:00").
            return dt.strftime("%Y-%m-%d %H:%M:%S.") + f"{ms % 1000:03d}"
        return dt.strftime("%Y-%m-%d %H:%M:%S.") + f"{ms % 1000:03d}Z"

    delta = ms - base_ms
    if delta < 0:
        delta = 0
    h = delta // 3_600_000
    delta -= h * 3_600_000
    m = delta // 60_000
    delta -= m * 60_000
    s = delta // 1000
    delta -= s * 1000
    return f"{h:02d}:{m:02d}:{s:02d}.{delta:03d}"


def ass_time(seconds: float) -> str:
    if seconds < 0:
        seconds = 0.0
    cs = int(round(seconds * 100.0))
    h = cs // (3600 * 100)
    cs -= h * 3600 * 100
    m = cs // (60 * 100)
    cs -= m * 60 * 100
    s = cs // 100
    cs -= s * 100
    return f"{h}:{m:02d}:{s:02d}.{cs:02d}"


def ass_escape(s: str) -> str:
    s = s.replace("\\", "\\\\")
    s = s.replace("{", "\\{").replace("}", "\\}")
    s = s.replace("\n", "\\N")
    return s


def ffmpeg_filter_escape_path(p: str) -> str:
    # escape ':' for filtergraph on mac/linux
    p = os.path.abspath(p)
    p = p.replace("\\", "\\\\")
    p = p.replace(":", "\\:")
    return p


def write_ass(path: str, packets: List[PacketInfo], metas: List[Optional[Dict[str, Any]]]) -> None:
    # Deprecated signature; kept for compatibility with older versions.
    raise RuntimeError("internal error: write_ass(packets, metas) should not be called")


def write_ass_from_metadata(path: str, metas: List[Optional[Dict[str, Any]]]) -> None:
    state = {"cam_key": None, "ing_key": None}
    events: List[Tuple[float, str]] = []

    base_cam: Optional[int] = None
    base_ing: Optional[int] = None

    for md in metas:
        if not md:
            continue
        abs_md = reconstruct_absolute(md, state)
        cam_abs = int(abs_md.get("cam_abs_ms", 0))
        ing_abs = int(abs_md.get("ingest_abs_ms", 0))
        if base_cam is None:
            base_cam = cam_abs
            base_ing = ing_abs
        abs_md["_base_cam_ms"] = base_cam
        abs_md["_base_ing_ms"] = base_ing if base_ing is not None else 0
        t = (cam_abs - base_cam) / 1000.0
        events.append((t, overlay_text(md, abs_md)))

    if base_cam is None:
        raise RuntimeError("no metadata found (nothing to overlay)")

    with open(path, "w", encoding="utf-8") as f:
        f.write("[Script Info]\n")
        f.write("ScriptType: v4.00+\n")
        f.write("PlayResX: 1920\n")
        f.write("PlayResY: 1080\n")
        f.write("\n[V4+ Styles]\n")
        f.write(
            "Format: Name, Fontname, Fontsize, PrimaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut,"
            " ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n"
        )
        # Bold=-1 (ASS convention). Increase outline for legibility.
        f.write("Style: Default,DejaVu Sans,44,&H00FFFFFF,&H00000000,&H8000FFFF,-1,0,0,0,100,100,0,0,1,3,1,7,40,40,140,1\n")
        f.write("\n[Events]\n")
        f.write("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

        for i, (t, txt) in enumerate(events):
            start = t
            if i + 1 < len(events):
                end = events[i + 1][0]
                if end <= start:
                    end = start + 0.04
            else:
                end = start + 0.04

            f.write(f"Dialogue: 0,{ass_time(start)},{ass_time(end)},Default,,0,0,0,,{ass_escape(txt)}\n")


def main() -> int:
    ap = argparse.ArgumentParser(description="Burn MediaMTX frame metadata into MP4 as overlay (Python + ffmpeg).")
    ap.add_argument("--in", dest="in_path", required=True, help="input MP4")
    ap.add_argument("--out", dest="out_path", required=True, help="output MP4")
    ap.add_argument("--stream", type=int, default=0, help="video stream index (default 0)")
    ap.add_argument("--codec", default="auto", help="h264, hevc, or auto (auto uses ffprobe)")
    ap.add_argument("--ffprobe", default="ffprobe", help="ffprobe binary path (used only when --codec=auto)")
    ap.add_argument("--ffmpeg", default="ffmpeg")
    ap.add_argument("--vcodec", default="libx264")
    ap.add_argument("--crf", type=int, default=18)
    ap.add_argument("--preset", default="veryfast")
    args = ap.parse_args()

    codec = args.codec
    if codec == "auto":
        codec = detect_video_codec(args.ffprobe, args.in_path, args.stream)
    codec = str(codec).lower()
    if codec == "h265":
        codec = "hevc"
    if codec not in ("h264", "hevc"):
        raise RuntimeError(f"unsupported video codec for overlay: {codec} (supported: h264, hevc)")

    bs = ffmpeg_bitstream(args.ffmpeg, args.in_path, args.stream, codec)

    nalus = split_annexb_nalus(bs)
    if codec == "h264":
        aus = group_aus_by_aud_h264(nalus)
        metas: List[Optional[Dict[str, Any]]] = []
        for au in aus:
            raw = None
            for n in au:
                raw = extract_user_data_unregistered_from_sei_h264(n)
                if raw is not None:
                    break
            metas.append(parse_one_metadata(raw) if raw else None)
    else:
        aus = group_aus_by_aud_h265(nalus)
        metas = []
        for au in aus:
            raw = None
            for n in au:
                raw = extract_user_data_unregistered_from_sei_h265(n)
                if raw is not None:
                    break
            metas.append(parse_one_metadata(raw) if raw else None)

    with tempfile.NamedTemporaryFile(prefix="mediamtx-meta-", suffix=".ass", delete=False) as tmp:
        ass_path = tmp.name
    try:
        try:
            write_ass_from_metadata(ass_path, metas)
        except RuntimeError as e:
            # Add quick diagnostics to distinguish "no metadata in file" from "parser bug".
            sei_nalus = 0
            for n in nalus:
                if codec == "h264":
                    if len(n) > 0 and (n[0] & 0x1F) == 6:
                        sei_nalus += 1
                else:
                    if len(n) > 1 and ((n[0] >> 1) & 0x3F) == 39:
                        sei_nalus += 1
            meta_frames = sum(1 for m in metas if m)
            raise RuntimeError(
                f"{e} | diagnostics: codec={codec} nalus={len(nalus)} "
                f"sei_nalus={sei_nalus} meta_frames={meta_frames}"
            ) from None

        run_cmd(
            [
                args.ffmpeg,
                "-y",
                "-i",
                args.in_path,
                "-vf",
                "ass=" + ffmpeg_filter_escape_path(ass_path),
                "-c:v",
                args.vcodec,
                "-crf",
                str(args.crf),
                "-preset",
                args.preset,
                "-c:a",
                "copy",
                "-movflags",
                "+faststart",
                args.out_path,
            ]
        )
    finally:
        try:
            os.unlink(ass_path)
        except OSError:
            # Best-effort cleanup: failure to remove the temporary ASS file is non-fatal.
            pass
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

