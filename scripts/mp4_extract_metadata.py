#!/usr/bin/env python3
"""
Extract MediaMTX per-frame metadata from a recorded MP4 using ffprobe/ffmpeg.

Outputs JSONL where each line corresponds to one video packet/sample, including:
  - pkt_pts (stream timebase ticks)
  - pkt_pts_time (seconds)
  - raw metadata (decoded CBOR map)
  - reconstructed absolute timestamps for non-key frames

Optionally dumps decoded frames as PNG named by PTS ticks, so other teams can join
frames <-> metadata by filename.

Requirements:
  - Python 3.9+
  - ffprobe + ffmpeg in PATH (or pass --ffprobe/--ffmpeg)

This is intentionally stdlib-only (includes a tiny CBOR decoder for our schema).
"""

from __future__ import annotations

import argparse
import base64
import dataclasses
import json
import os
import subprocess
import sys
from typing import Any, Dict, List, Optional, Tuple


# Must match internal/framemetadata/frame_metadata.go
SCHEMA_VERSION = 1
UUID16 = bytes.fromhex("1d536d9a2b2d4418933e2a3cf80f605c")


@dataclasses.dataclass
class PacketInfo:
    pts: int
    pts_time: float
    dts: Optional[int]
    dts_time: Optional[float]
    flags: str
    duration: Optional[int]
    duration_time: Optional[float]


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
    out = run_cmd([ffprobe, "-v", "error", "-print_format", "json", "-show_error"] + args + [in_path])
    return json.loads(out.decode("utf-8"))


def detect_video_codec(ffprobe: str, in_path: str, stream_index: int) -> Tuple[str, str]:
    data = ffprobe_json(
        ffprobe,
        in_path,
        ["-select_streams", f"v:{stream_index}", "-show_streams"],
    )
    streams = data.get("streams") or []
    if not streams:
        raise RuntimeError("no video stream found")
    st = streams[0]
    codec = st.get("codec_name")
    time_base = st.get("time_base")
    if not codec or not time_base:
        raise RuntimeError("ffprobe missing codec_name/time_base")
    return codec, time_base


def list_packets(ffprobe: str, in_path: str, stream_index: int) -> List[PacketInfo]:
    data = ffprobe_json(
        ffprobe,
        in_path,
        [
            "-select_streams",
            f"v:{stream_index}",
            "-show_packets",
            "-show_entries",
            "packet=pts,pts_time,dts,dts_time,flags,duration,duration_time",
        ],
    )
    packets = []
    for p in data.get("packets") or []:
        if "pts" not in p or "pts_time" not in p:
            # skip packets without PTS (rare)
            continue
        packets.append(
            PacketInfo(
                pts=int(p["pts"]),
                pts_time=float(p["pts_time"]),
                dts=int(p["dts"]) if "dts" in p else None,
                dts_time=float(p["dts_time"]) if "dts_time" in p else None,
                flags=str(p.get("flags") or ""),
                duration=int(p["duration"]) if "duration" in p else None,
                duration_time=float(p["duration_time"]) if "duration_time" in p else None,
            )
        )
    return packets


def ffmpeg_bitstream(ffmpeg: str, in_path: str, stream_index: int, codec: str) -> bytes:
    """
    Returns an Annex-B bytestream for h264/hevc with AUD inserted.
    For AV1, returns a concatenated OBU stream (best-effort).
    """
    if codec == "h264":
        bsf = "h264_mp4toannexb,h264_metadata=aud=insert"
        fmt = "h264"
    elif codec in ("hevc", "h265"):
        bsf = "hevc_mp4toannexb,hevc_metadata=aud=insert"
        fmt = "hevc"
    elif codec == "av1":
        # best-effort: copy as raw AV1 (may still be length-prefixed depending on build).
        # We'll still try to split OBUs by their internal size field.
        bsf = "av1_mp4toannexb"
        fmt = "av1"
    else:
        raise RuntimeError(f"unsupported codec for extraction: {codec}")

    cmd = [
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
    return run_cmd(cmd)


def dump_frames(ffmpeg: str, in_path: str, stream_index: int, out_dir: str) -> None:
    os.makedirs(out_dir, exist_ok=True)
    cmd = [
        ffmpeg,
        "-v",
        "error",
        "-i",
        in_path,
        "-map",
        f"0:v:{stream_index}",
        "-vsync",
        "0",
        "-frame_pts",
        "1",
        os.path.join(out_dir, "%012d.png"),
    ]
    run_cmd(cmd)


def find_start_code(b: bytes, start: int) -> Tuple[int, int]:
    # returns (pos, length) or (-1, 0)
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
            # do not include AUD itself
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
            return payload[16:]  # after SEI UUID
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


def leb128_decode(b: bytes, off: int) -> Tuple[int, int]:
    v = 0
    shift = 0
    for i in range(10):
        if off + i >= len(b):
            raise ValueError("leb128 truncated")
        by = b[off + i]
        v |= (by & 0x7F) << shift
        if (by & 0x80) == 0:
            return v, off + i + 1
        shift += 7
    raise ValueError("leb128 too long")


def split_obu_stream(obus: bytes) -> List[bytes]:
    out: List[bytes] = []
    i = 0
    while i < len(obus):
        if i + 2 > len(obus):
            break
        h = obus[i]
        ext = (h & 0x04) != 0
        has_size = (h & 0x02) != 0
        j = i + 1
        if ext:
            j += 1
            if j > len(obus):
                break
        if not has_size:
            break
        sz, k = leb128_decode(obus, j)
        j = k
        if j + sz > len(obus):
            break
        out.append(obus[i : j + sz])
        i = j + sz
    return out


def parse_metadata_obu(obu: bytes) -> Optional[bytes]:
    if len(obu) < 2:
        return None
    obu_type = (obu[0] >> 3) & 0x0F
    if obu_type != 15:
        return None
    ext = (obu[0] & 0x04) != 0
    has_size = (obu[0] & 0x02) != 0
    i = 1 + (1 if ext else 0)
    if i >= len(obu):
        return None
    if not has_size:
        return obu[i:]
    sz, j = leb128_decode(obu, i)
    if j + sz > len(obu):
        return None
    return obu[j : j + sz]


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

    if major == 0:  # unsigned int
        u, off = cbor_read_u(b, off, ai)
        return u, off
    if major == 1:  # negative int
        u, off = cbor_read_u(b, off, ai)
        return -1 - u, off
    if major == 3:  # text string
        ln, off = cbor_read_u(b, off, ai)
        s = b[off : off + ln].decode("utf-8")
        return s, off + ln
    if major == 5:  # map
        ln, off = cbor_read_u(b, off, ai)
        m: Dict[str, Any] = {}
        for _ in range(ln):
            k, off = cbor_decode(b, off)
            v, off = cbor_decode(b, off)
            if not isinstance(k, str):
                raise ValueError("non-string key")
            m[k] = v
        return m, off
    if major == 7:  # floats / simple
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
    v, off = cbor_decode(b, 0)
    if off != len(b):
        # allow trailing bytes, but uncommon
        pass
    if not isinstance(v, dict):
        raise ValueError("cbor payload is not a map")
    return v


def reconstruct_absolute(m: Dict[str, Any], state: Dict[str, Optional[int]]) -> Dict[str, Any]:
    """
    Adds:
      cam_abs_ms
      ingest_abs_ms
    for both key and non-key frames, using last keyframe values + dt.
    """
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


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--in", dest="in_path", required=True, help="input MP4")
    ap.add_argument("--out-jsonl", required=True, help="output JSONL path")
    ap.add_argument("--stream", type=int, default=0, help="video stream index (default 0)")
    ap.add_argument("--ffprobe", default="ffprobe")
    ap.add_argument("--ffmpeg", default="ffmpeg")
    ap.add_argument("--dump-frames", action="store_true", help="dump decoded PNG frames")
    ap.add_argument("--frames-dir", default="frames", help="frames output directory (relative to CWD)")
    args = ap.parse_args()

    codec, time_base = detect_video_codec(args.ffprobe, args.in_path, args.stream)
    packets = list_packets(args.ffprobe, args.in_path, args.stream)

    if args.dump_frames:
        dump_frames(args.ffmpeg, args.in_path, args.stream, args.frames_dir)

    bs = ffmpeg_bitstream(args.ffmpeg, args.in_path, args.stream, codec)

    meta_by_index: List[Optional[Dict[str, Any]]] = []

    if codec == "h264":
        nalus = split_annexb_nalus(bs)
        aus = group_aus_by_aud_h264(nalus)
        for au in aus:
            raw = None
            for n in au:
                raw = extract_user_data_unregistered_from_sei_h264(n)
                if raw is not None:
                    break
            meta_by_index.append(parse_one_metadata(raw) if raw else None)

    elif codec in ("hevc", "h265"):
        nalus = split_annexb_nalus(bs)
        aus = group_aus_by_aud_h265(nalus)
        for au in aus:
            raw = None
            for n in au:
                raw = extract_user_data_unregistered_from_sei_h265(n)
                if raw is not None:
                    break
            meta_by_index.append(parse_one_metadata(raw) if raw else None)

    elif codec == "av1":
        # best-effort: split OBUs and treat each METADATA OBU as "current packet".
        # (AV1 in MP4 can be more complex; extend as needed.)
        obus = split_obu_stream(bs)
        # naïve: one TU per packet is unknown; we just scan sequentially and emit first metadata OBU seen.
        for obu in obus:
            pl = parse_metadata_obu(obu)
            if pl is None:
                continue
            md = parse_one_metadata(pl)
            if md is not None:
                meta_by_index.append(md)
        # will likely not match packet count; handled below.

    else:
        raise RuntimeError(f"unsupported codec: {codec}")

    # align counts (common case: h264/h265 with AUD insertion -> should match)
    n = min(len(packets), len(meta_by_index))
    if n == 0:
        raise RuntimeError("no packets or no metadata/bitstream parsed")

    state = {"cam_key": None, "ing_key": None}
    with open(args.out_jsonl, "w", encoding="utf-8") as f:
        for i in range(n):
            pkt = packets[i]
            md = meta_by_index[i]
            rec: Dict[str, Any] = {
                "codec": codec,
                "time_base": time_base,
                "pkt_index": i,
                "pkt_pts": pkt.pts,
                "pkt_pts_time": pkt.pts_time,
                "pkt_flags": pkt.flags,
            }
            if md is not None:
                rec["meta"] = md
                rec["meta_abs"] = reconstruct_absolute(md, state)
                # helpful for debugging / external systems
                rec["meta_b64"] = base64.b64encode(md.get("_raw_binary", b"")).decode("ascii")
            f.write(json.dumps(rec, sort_keys=False) + "\n")

    return 0


def parse_one_metadata(raw: Optional[bytes]) -> Optional[Dict[str, Any]]:
    if not raw:
        return None
    parsed = parse_binary_payload(raw)
    if parsed is None:
        return None
    ver, uid, cbor = parsed
    if ver != SCHEMA_VERSION or uid != UUID16:
        return None
    m = decode_cbor_map(cbor)
    # stash raw binary for optional b64 dump (useful for other teams)
    m["_raw_binary"] = raw
    return m


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as e:
        print(f"error: {e}", file=sys.stderr)
        raise

