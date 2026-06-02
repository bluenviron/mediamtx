// [B1] to be unified when B1 merges in.

namespace Mediar.Codecs.Opus.Encoder;

/// <summary>
/// Placeholder top-level Opus encoder for Phase B2. The CELT layer
/// implemented in this project drives the lower-level routines directly via
/// <see cref="OpusRangeEncoder"/>; this class exists so the public API
/// surface is in place and so the B1 sibling session can wire CELT-only
/// frames into <see cref="EncodeCeltOnlyFrame"/>.
/// </summary>
public sealed class OpusEncoder
{
    /// <summary>The configured parameters.</summary>
    public OpusEncoderParameters Parameters { get; }

    /// <summary>Number of input channels (1 or 2).</summary>
    public int Channels { get; }

    /// <summary>Initialise an encoder for <paramref name="channels"/> input PCM channels.</summary>
    public OpusEncoder(int channels, OpusEncoderParameters parameters)
    {
        if (channels is not (1 or 2))
            throw new ArgumentOutOfRangeException(nameof(channels), "Opus supports 1 or 2 channels per stream.");
        Channels = channels;
        Parameters = parameters;
    }

    /// <summary>
    /// Phase B2 placeholder — the full encoder pipeline is wired here once
    /// the per-frame CELT pipeline (windowing → MDCT → energy quant →
    /// allocator → PVQ search → range encoder writes) is unified with the
    /// B1 framing layer (TOC byte + frame packer). For now callers should
    /// drive the CELT layer directly via the per-frame helpers exposed by
    /// the <c>Mediar.Codecs.Opus.Encoder.Celt</c> namespace.
    /// </summary>
    public int EncodeCeltOnlyFrame(ReadOnlySpan<float> pcm, Span<byte> destination)
    {
        _ = pcm;
        _ = destination;
        throw new NotImplementedException(
            "OpusEncoder.EncodeCeltOnlyFrame is wired in Phase B1+B2 integration. " +
            "Drive the CELT layer directly via Mediar.Codecs.Opus.Encoder.Celt.* for now.");
    }
}
