# Mediar.Imaging.Jpeg

JPEG codec for Mediar implementing the full ITU-T Rec. T.81 (1992-09)
(ISO/IEC 10918-1) marker family for decode plus a baseline encoder.

## SOF coverage

| SOF marker | Variant | Decode | Encode | T.81 reference |
|------------|---------|:------:|:------:|----------------|
| `FF C0` SOF0  | Baseline DCT (Huffman, 8-bit)            | ✅ | ✅ | §B.2.2, Annex F.1 |
| `FF C1` SOF1  | Extended sequential DCT (Huffman, 12-bit) | ✅ | — | §B.2.2 |
| `FF C2` SOF2  | Progressive DCT (Huffman)                | ✅ | — | §G.1 |
| `FF C3` SOF3  | Lossless (Huffman, predictive)           | ✅ | — | §H.1 |
| `FF C9` SOF9  | Extended sequential DCT (arithmetic)     | ⚠️ primitives only | — | §F.1.4, Annex D |
| `FF CA` SOF10 | Progressive DCT (arithmetic)             | ⚠️ marker recognised; throws `InvalidDataException` | — | §G.1 + Annex D |
| `FF CB` SOF11 | Lossless (arithmetic)                    | ⚠️ marker recognised; throws `InvalidDataException` | — | §H.1 + Annex D |

⚠️ = recognised but not fully implemented: callers receive an explicit
`InvalidDataException` rather than silently incorrect pixels.
`JpegArithmeticDecoder` exposes the QM-coder primitives (Qe table per
T.81 Table D.3, `INITDEC` / `Decode` / `Renormalize_dec`) which are
covered by unit tests; integrating them into a full multi-component
SOF9 scan is deferred until a royalty-free arithmetic-coded corpus is
available for regression testing.

## Colour models and subsampling

* Grayscale (`Gray8`) — 1 component, sampling 1×1.
* `Rgb24` ↔ Rec. ITU-R BT.601 YCbCr per JFIF 1.02 (Annex B of T.81).
* Chroma subsampling on encode: `4:4:4` (1×1), `4:2:2` (2×1), `4:2:0`
  (2×2). All three are also accepted on decode.
* 12-bit decode emits `Gray16` or `Rgb48` with samples scaled to fill
  the 16-bit range (T.81 §B.2.2 — encoded 12-bit values are left-shifted
  by 4 bits when materialised into the output buffer).

## Encoder features

| Feature | Spec reference | Notes |
|---------|----------------|-------|
| Quality 1–100 → Q-table scaling | Annex K + libjpeg formula | `JpegStandardTables.ScaleForQuality` |
| Annex K standard Q-tables and Huffman tables | T.81 Annex K.1 / K.3 | `JpegStandardTables` |
| Two-pass optimised Huffman | T.81 Annex K.2 | `JpegOptimisedHuffman` |
| Restart intervals (`FF DD`, `FF D0..D7`) | T.81 §F.1.2.3 | `JpegEncodeOptions.RestartInterval` |
| EXIF (`APP1 / Exif\0\0`) | T.81 Annex B + TIFF 6.0 | `JpegMetadataWriter` |
| ICC profile (`APP2 / ICC_PROFILE\0`) | ICC.1:2010 §B.4 | Multi-segment chunking |
| XMP (`APP1 / http://ns.adobe.com/xap/1.0/\0`) | XMP Specification Part 3 | Single segment |
| Multi-Picture Format (MPO) | CIPA DC-007:2009 | `JpegMpo` |

## Acceleration

* `JpegIdctSimd` registers the 8×8 inverse DCT with
  `Mediar.Acceleration.Kernels.Idct8x8`; a scalar Loeffler-Lightenberg-
  Moschytz integer reference (`ScalarIdct8x8`) is always present.
* AVX2 / SSE2 / ARM AdvSimd backends share the same fixed-point
  constants (`ConstBits = 13`) so any future SIMD implementation is
  required to be bit-exact against the scalar.

## Error handling

All decoders raise `System.IO.InvalidDataException` for malformed
streams (truncated segments, unknown markers, invalid Huffman, etc.);
no path is permitted to surface `IndexOutOfRangeException` on bad
input.

## References

* **ITU-T Rec. T.81 (1992-09)** = ISO/IEC 10918-1: *Information
  technology — Digital compression and coding of continuous-tone still
  images: Requirements and guidelines.*
* **JFIF 1.02** (Annex B of T.81): file-format wrapping.
* **CIPA DC-007:2009**: *Multi-Picture Format (MPO).*
* **ICC.1:2010-12**: *Image technology colour management — Architecture,
  profile format, and data structure*, §B.4 (JPEG embedding).
* **W. Pennebaker & J. Mitchell, 1992**, *JPEG: Still Image Data
  Compression Standard*, Van Nostrand Reinhold. Ch. 7 (sequential),
  ch. 13–14 (arithmetic coding), App. E (Qe table).
* **C. Loeffler, A. Lightenberg, G. Moschytz, 1989**, *Practical fast
  1-D DCT algorithms with 11 multiplications*, IEEE ICASSP-89, vol. 2,
  pp. 988–991.
* **Adobe XMP Specification Part 3** (storage in files), 2016.
