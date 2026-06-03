using Mediar.Codecs.Opus.Decoder.Celt;

namespace Mediar.Codecs.Opus.Encoder.Celt;

/// <summary>
/// CELT encoder state — mirror of the decoder's
/// <c>Mediar.Codecs.Opus.Decoder.Celt.CeltDecoder</c>. Owns the
/// window-overlap tail, the previous-frame log-energies for the coarse
/// energy predictor, and the running anti-collapse seed.
/// </summary>
/// <remarks>
/// <para>
/// Per-frame pipeline (Phase B2 v1 — partial, see README for the gap
/// list):
/// </para>
/// <list type="number">
///   <item><description>Window the PCM input (overlap with the prior
///     frame's tail via <see cref="CeltMdct.BuildSineWindow"/>).</description></item>
///   <item><description>Forward MDCT into <c>N/2</c> coefficients
///     (<see cref="CeltMdct.Forward"/>).</description></item>
///   <item><description>Compute per-band energy magnitudes, log-encode
///     them with <see cref="CeltEnergyQuant.QuantCoarseEnergy"/>
///     then <see cref="CeltEnergyQuant.QuantFineEnergy"/>.</description></item>
///   <item><description>Apply allocator-determined K per band
///     (<see cref="CeltAllocator.FlatAllocation"/> in v1) and run
///     <see cref="CeltBandQuant.QuantBandSimple"/> for each band.</description></item>
///   <item><description>Finalise leftover bits with
///     <see cref="CeltEnergyQuant.QuantEnergyFinalise"/>.</description></item>
/// </list>
/// <para>
/// References: RFC 6716 §4.3; libopus <c>celt/celt_encoder.c</c>;
/// Valin et al. <i>Definition of the Opus Audio Codec</i> (IETF 2012).
/// </para>
/// </remarks>
public sealed class CeltEncoder
{
    private readonly int _channels;
    private readonly int _sampleRate;
    private readonly float[] _overlap;
    private readonly float[] _oldLogE;
    private readonly float[] _oldLogE2;
    private uint _antiCollapseSeed;

    /// <summary>
    /// Construct an encoder for the given channel count and sample rate.
    /// </summary>
    /// <param name="channels">1 (mono) or 2 (stereo).</param>
    /// <param name="sampleRate">Sample rate in Hz (8000/12000/16000/24000/48000).</param>
    public CeltEncoder(int channels, int sampleRate)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(channels, 1);
        ArgumentOutOfRangeException.ThrowIfGreaterThan(channels, 2);
        if (sampleRate is not (8000 or 12000 or 16000 or 24000 or 48000))
            throw new ArgumentOutOfRangeException(nameof(sampleRate), sampleRate, "Sample rate must be 8/12/16/24/48 kHz.");

        _channels = channels;
        _sampleRate = sampleRate;
        // The overlap region for a 20 ms WB frame is N/2 = 480 samples,
        // but we size for the maximum FB frame (960) to keep the buffer
        // re-usable across LM modes.
        _overlap = new float[960 * channels];
        _oldLogE = new float[channels * CeltConstants.MaxBands];
        _oldLogE2 = new float[channels * CeltConstants.MaxBands];
        Array.Fill(_oldLogE, -28f);
        Array.Fill(_oldLogE2, -28f);
        _antiCollapseSeed = 0;
    }

    /// <summary>Channel count.</summary>
    public int Channels => _channels;

    /// <summary>Sample rate in Hz.</summary>
    public int SampleRate => _sampleRate;

    /// <summary>Overlap tail from the previous frame (length = window/2).</summary>
    internal Span<float> Overlap => _overlap;

    /// <summary>Previous-frame coarse log energies (decoder predictor state).</summary>
    internal Span<float> OldLogE => _oldLogE;

    /// <summary>Frame before <see cref="OldLogE"/> — second-order predictor state.</summary>
    internal Span<float> OldLogE2 => _oldLogE2;

    /// <summary>Anti-collapse PRNG seed (advanced once per band per frame).</summary>
    internal uint AntiCollapseSeed
    {
        get => _antiCollapseSeed;
        set => _antiCollapseSeed = value;
    }
}
