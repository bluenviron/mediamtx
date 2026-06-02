// [B1] to be unified when B1 merges in.

namespace Mediar.Codecs.Opus.Encoder;

/// <summary>
/// Encoder configuration knobs the Phase B2 CELT layer cares about. The full
/// set will be expanded by B1; this is the minimum surface area used by the
/// CELT encoder and its tests.
/// </summary>
/// <param name="Complexity">
/// 0..10, mirrors libopus <c>OPUS_SET_COMPLEXITY</c>. Phase B2 uses it to
/// switch between the fast greedy PVQ search (≤ 5) and the higher-quality
/// local-search path (≥ 6).
/// </param>
/// <param name="BitrateBitsPerSecond">
/// Target bitrate. The CELT allocator uses this — through
/// <see cref="OpusEncoderParameters.FrameSizeMicroseconds"/> — to derive the
/// per-frame bit budget. Use <c>-1</c> for "auto" (libopus VBR).
/// </param>
/// <param name="FrameSizeMicroseconds">
/// One of 2500, 5000, 10000, 20000 (CELT supports up to 20 ms — larger
/// SILK / Hybrid frame sizes will be added in later phases).
/// </param>
/// <param name="UseVbr">If true, enable variable-bitrate (the default).</param>
public readonly record struct OpusEncoderParameters(
    int Complexity,
    int BitrateBitsPerSecond,
    int FrameSizeMicroseconds,
    bool UseVbr)
{
    /// <summary>Default — complexity 10, 64 kbps target, 20 ms frames, VBR on.</summary>
    public static OpusEncoderParameters Default { get; } =
        new(Complexity: 10, BitrateBitsPerSecond: 64_000, FrameSizeMicroseconds: 20_000, UseVbr: true);
}
