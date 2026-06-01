using System.Collections.Immutable;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Channel spectrum after the AAC decoder's PNS-synthesis stage. The
/// layout matches <see cref="AacDequantizedSpectrum.Coefficients"/>:
/// for long windows, 1024 samples of the single window; for grouped
/// <see cref="AacWindowSequence.EightShort"/>, group-major /
/// SFB-window-interleaved per the AAC §4.6.2 ordering.
/// </summary>
/// <remarks>
/// <para>
/// This record sits one layer above
/// <see cref="AacDequantizedSpectrum"/> in the decoder chain. Once
/// intensity-stereo, M/S stereo, TNS and IMDCT composers ship, they
/// will accept <see cref="AacDecodedSpectrum"/> instances and produce
/// the next-stage record. The window-sequence field is duplicated
/// here so downstream stages do not have to plumb the
/// <see cref="AacChannelFrame"/> separately just to know the layout.
/// </para>
/// </remarks>
public sealed record AacDecodedSpectrum
{
    /// <summary>Length of the decoded spectrum (always 1024).</summary>
    public const int TransformLength = AacSpectralData.TransformLength;

    /// <summary>
    /// 1024 post-PNS MDCT coefficients in spec layout (see remarks).
    /// </summary>
    public required ImmutableArray<float> Coefficients { get; init; }

    /// <summary>
    /// Window sequence captured from the source channel's
    /// <c>ics_info()</c>, propagated so downstream stages can pick the
    /// right layout / SWB table without revisiting the frame.
    /// </summary>
    public required AacWindowSequence WindowSequence { get; init; }
}
