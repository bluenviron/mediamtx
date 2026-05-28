namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Configuration for <see cref="FlacFrameEncoder"/> and
/// <see cref="FlacStreamInfoBuilder"/>. Mirrors the subset of FLAC STREAMINFO
/// (RFC 9639 §8.2) needed to encode a stream.
/// </summary>
/// <param name="SampleRate">Samples per second per channel. 1–655350 Hz.</param>
/// <param name="Channels">Channel count, 1–8.</param>
/// <param name="BitsPerSample">Bits per coded sample: 4–32 (FLAC supports
/// arbitrary widths, but only 8 / 12 / 16 / 20 / 24 use the compact frame-
/// header sample-size code; other widths fall back to streaminfo-resolved).</param>
/// <param name="BlockSize">Frame block size (samples per channel per FLAC
/// frame). All frames carry this many samples except possibly the last frame.</param>
public sealed record FlacEncoderParameters(
    int SampleRate,
    int Channels,
    int BitsPerSample,
    int BlockSize = 4096)
{
    /// <summary>Validate the parameter combination.</summary>
    /// <exception cref="ArgumentOutOfRangeException">A parameter is out of range.</exception>
    public void Validate()
    {
        if (SampleRate is <= 0 or > 655350)
        {
            throw new ArgumentOutOfRangeException(nameof(SampleRate), SampleRate, "FLAC sample rate must be in [1, 655350] Hz.");
        }
        if (Channels is < 1 or > 8)
        {
            throw new ArgumentOutOfRangeException(nameof(Channels), Channels, "FLAC supports 1–8 channels.");
        }
        if (BitsPerSample is < 4 or > 32)
        {
            throw new ArgumentOutOfRangeException(nameof(BitsPerSample), BitsPerSample, "FLAC bits-per-sample must be in [4, 32].");
        }
        if (BlockSize is < 16 or > 65535)
        {
            throw new ArgumentOutOfRangeException(nameof(BlockSize), BlockSize, "FLAC block size must be in [16, 65535].");
        }
    }
}
