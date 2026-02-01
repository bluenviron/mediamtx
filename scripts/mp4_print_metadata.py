#!/usr/bin/env python3
"""
Print MediaMTX per-frame metadata from an MP4 as logs (no burn-in).

This is a lightweight "viewer" for teams that just want to inspect metadata
per frame/sample.

Supported:
  - H.264: SEI user_data_unregistered
  - H.265/HEVC: SEI user_data_unregistered

It uses ffmpeg bitstream filters to extract an Annex-B stream with AUD inserted,
then scans each access unit for our metadata and prints one line per AU that
contains metadata.

Stdlib-only.
"""

from __future__ import annotations

import argparse
import datetime
import json
import subprocess
import sys
import signal
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

def ffprobe_codec(ffprobe: str, in_path: str, stream_index: int) -> str:
    out = run_cmd(
        [
            ffprobe,
            "-v",
            "error",
            "-select_streams",
            f"v:{stream_index}",
            "-show_entries",
            "stream=codec_name",
            "-of",
            "default=nw=1:nk=1",
            in_path,
        ]
    )
    codec = out.decode("utf-8", errors="replace").strip().splitlines()[0].strip()
    return codec

def ffmpeg_bitstream(ffmpeg: str, in_path: str, stream_index: int, codec: str) -> bytes:
    if codec == "h264":
        bsf = "h264_mp4toannexb,h264_metadata=aud=insert"
        fmt = "h264"
    elif codec in ("hevc", "h265"):
        bsf = "hevc_mp4toannexb,hevc_metadata=aud=insert"
        fmt = "hevc"
    else:
        raise RuntimeError(f"unsupported codec: {codec} (supported: h264, hevc)")

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
        zeros = zeros + 1 if by == 0 else 0
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
    if ntype != 39:
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
        return b[off : off + ln].decode("utf-8"), off + ln
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
        if ai == 26:
            import struct

            (f,) = struct.unpack(">f", b[off : off + 4])
            return float(f), off + 4
        if ai == 27:
            import struct

            (f,) = struct.unpack(">d", b[off : off + 8])
            return float(f), off + 8
        if ai == 22:
            return None, off
        raise ValueError("unsupported simple/float")
    raise ValueError("unsupported cbor major")


def decode_cbor_map(b: bytes) -> Dict[str, Any]:
    v, _ = cbor_decode(b, 0)
    if not isinstance(v, dict):
        raise ValueError("cbor payload is not map")
    return v


def parse_one_metadata(raw: Optional[bytes]) -> Optional[Dict[str, Any]]:
    if not raw:
        return None
    parsed = parse_binary_payload(raw)
    if not parsed:
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
    out["cam_abs_ms"] = cam_key + int(m.get("dt_ms", 0))
    out["ingest_abs_ms"] = ing_key + int(m.get("ingest_dt_ms", 0))
    return out

def human_time_ms(ms: int, base_ms: int = 0) -> str:
    # If it looks like epoch milliseconds, render local wall clock time.
    # Otherwise render as offset since base_ms.
    if ms >= 1_000_000_000_000:
        dt = datetime.datetime.fromtimestamp(ms / 1000.0, tz=datetime.timezone.utc).astimezone()
        return dt.strftime("%Y-%m-%d %H:%M:%S.") + f"{ms % 1000:03d}" + dt.strftime(" %Z")

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


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("input", nargs="?", help="input MP4 (positional alternative to --in)")
    ap.add_argument("--in", dest="in_path", default=None, help="input MP4")
    ap.add_argument("--codec", default="auto", help="h264, hevc, or auto (default auto)")
    ap.add_argument("--stream", type=int, default=0)
    ap.add_argument("--ffmpeg", default="ffmpeg")
    ap.add_argument("--ffprobe", default="ffprobe")
    ap.add_argument("--only-meta", action="store_true", help="print only frames that contain metadata")
    ap.add_argument("--max-frames", type=int, default=0, help="stop after N frames (0 = no limit)")
    args = ap.parse_args()

    in_path = args.in_path or args.input
    if not in_path:
        ap.error("missing input MP4 (use --in or positional input)")

    codec = args.codec.lower()
    if codec == "h265":
        codec = "hevc"
    if codec == "auto":
        codec = ffprobe_codec(args.ffprobe, in_path, args.stream).lower()
        if codec == "h265":
            codec = "hevc"
    if codec not in ("h264", "hevc"):
        raise SystemExit("codec must be h264 or hevc (or use --codec auto)")

    bs = ffmpeg_bitstream(args.ffmpeg, in_path, args.stream, codec)
    nalus = split_annexb_nalus(bs)
    aus = group_aus_by_aud_h264(nalus) if codec == "h264" else group_aus_by_aud_h265(nalus)

    state = {"cam_key": None, "ing_key": None}
    printed = 0
    base_cam: Optional[int] = None
    base_ing: Optional[int] = None
    for i, au in enumerate(aus):
        if args.max_frames and i >= args.max_frames:
            break
        raw = None
        if codec == "h264":
            for n in au:
                raw = extract_user_data_unregistered_from_sei_h264(n)
                if raw:
                    break
        else:
            for n in au:
                raw = extract_user_data_unregistered_from_sei_h265(n)
                if raw:
                    break
        md = parse_one_metadata(raw)
        if not md:
            if not args.only_meta:
                try:
                    print(f"frame={i} metadata no_meta=1", flush=True)
                except BrokenPipeError:
                    return 0
            continue

        abs_md = reconstruct_absolute(md, state)
        cam_abs = int(abs_md.get("cam_abs_ms", 0))
        ing_abs = int(abs_md.get("ingest_abs_ms", 0))
        if base_cam is None:
            base_cam = cam_abs
            base_ing = ing_abs
        cam_hr = human_time_ms(cam_abs, base_ms=base_cam or 0)
        ing_hr = human_time_ms(ing_abs, base_ms=base_ing or 0)

        try:
            ver = md.get("version", "")
            print(
                f"frame={i} metadata v={ver} cam={cam_hr} ingest={ing_hr} "
                f"meta={json.dumps(md, separators=(',', ':'))} "
                f"abs={json.dumps(abs_md, separators=(',', ':'))}",
                flush=True,
            )
        except BrokenPipeError:
            return 0
        printed += 1

    if printed == 0:
        raise SystemExit("no metadata found")
    return 0


if __name__ == "__main__":
    try:
        if hasattr(signal, "SIGPIPE"):
            signal.signal(signal.SIGPIPE, signal.SIG_DFL)
        raise SystemExit(main())
    except Exception as e:
        print(f"error: {e}", file=sys.stderr)
        raise

