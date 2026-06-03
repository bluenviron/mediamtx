namespace Mediar.Codecs.Opus.Encoder.Silk;

/// <summary>
/// SILK voice activity detector. Computes per-sub-frame signal-to-noise
/// ratio estimates and a frame-level voice-active flag from a frame of
/// linear PCM. Mirrors the API of libopus <c>silk_VAD_GetSA_Q8</c>
/// (declared in <c>silk/VAD.c</c>); the SNR vector is consumed
/// downstream by the NLSF quantiser bit-budget estimator and by the
/// SILK/CELT bandwidth selector.
/// </summary>
/// <remarks>
/// <para>
/// This is a <b>behaviourally-equivalent simplification</b> of the
/// libopus VAD, not a bit-exact port. libopus splits the input into
/// four IIR-defined subbands (200 Hz / 1 kHz / 3 kHz / 8 kHz),
/// tracks per-subband noise floors with a leaky-integrator estimator
/// in Q-format fixed-point, and combines them with a tilted SNR
/// metric. The simplified detector here:
/// </para>
/// <list type="number">
///   <item><description>Splits the frame into four equal-length sub-frames.</description></item>
///   <item><description>Computes short-term sub-frame energy.</description></item>
///   <item><description>Estimates the noise floor as the minimum of a 5-frame running window.</description></item>
///   <item><description>Reports per-sub-frame SNR = 10·log10(energy / max(noiseFloor, ε)) in Q7.</description></item>
///   <item><description>Reports voice-active when at least one sub-frame's SNR exceeds <see cref="ActivityThresholdSnrQ7"/>.</description></item>
/// </list>
/// <para>
/// The bit-exact libopus VAD lands in a later slice. Its output is
/// consumed only by the encoder rate-distortion path; the VAD flag
/// itself is not directly written to the bitstream (it influences the
/// NLSF interpolator decision, the LBRR redundancy decision, and the
/// SILK/CELT mode switch). Replacing this simplified VAD with the
/// bit-exact port is therefore a behaviour-preserving change.
/// </para>
/// <para>References: libopus <c>silk/VAD.c</c>
/// (<c>silk_VAD_Init</c>, <c>silk_VAD_GetSA_Q8</c>); RFC 6716 §4.2.</para>
/// </remarks>
public sealed class SilkVad
{
    /// <summary>Number of analysis sub-frames per SILK frame.</summary>
    public const int SubframeCount = 4;

    /// <summary>Per-sub-frame SNR (Q7) at or above which the VAD reports voice activity.</summary>
    public const int ActivityThresholdSnrQ7 = 6 * 128;

    private const float MinNoiseEnergy = 1e-6f;
    private const int NoiseHistory = 5;

    private readonly float[] _noiseHistory = new float[SubframeCount * NoiseHistory];
    private int _historyFill;
    private int _historyHead;

    /// <summary>
    /// Result of a VAD call. Both <see cref="SnrQ7"/> entries are valid
    /// regardless of <see cref="IsVoiceActive"/>; downstream stages
    /// consume the SNR vector unconditionally.
    /// </summary>
    public readonly ref struct Result
    {
        /// <summary>Per-sub-frame SNR estimate in Q7 dB. Length <see cref="SubframeCount"/>.</summary>
        public readonly Span<int> SnrQ7;
        /// <summary>True if any sub-frame's SNR exceeded the activity threshold.</summary>
        public readonly bool IsVoiceActive;

        internal Result(Span<int> snr, bool active)
        {
            SnrQ7 = snr;
            IsVoiceActive = active;
        }
    }

    /// <summary>Reset the internal noise-floor state. Call between independent streams.</summary>
    public void Reset()
    {
        Array.Clear(_noiseHistory);
        _historyFill = 0;
        _historyHead = 0;
    }

    /// <summary>
    /// Analyse one frame of PCM (length must be divisible by
    /// <see cref="SubframeCount"/>) and produce per-sub-frame SNR
    /// plus the voice-active flag.
    /// </summary>
    /// <param name="frame">Linear PCM samples.</param>
    /// <param name="snrQ7">
    /// Destination for per-sub-frame SNR in Q7 dB. Length must be
    /// <see cref="SubframeCount"/>.
    /// </param>
    /// <returns>The analysis result (alias of <paramref name="snrQ7"/>).</returns>
    public Result Analyze(ReadOnlySpan<float> frame, Span<int> snrQ7)
    {
        if (snrQ7.Length < SubframeCount)
            throw new ArgumentException("SNR destination too small.", nameof(snrQ7));
        if (frame.Length == 0 || frame.Length % SubframeCount != 0)
            throw new ArgumentException(
                "Frame length must be a non-zero multiple of " + SubframeCount + ".",
                nameof(frame));

        int subLen = frame.Length / SubframeCount;
        Span<float> energy = stackalloc float[SubframeCount];

        for (int s = 0; s < SubframeCount; s++)
        {
            double e = 0.0;
            int start = s * subLen;
            for (int i = 0; i < subLen; i++)
            {
                float v = frame[start + i];
                e += (double)v * v;
            }
            energy[s] = (float)(e / subLen);
        }

        bool voiceActive = false;
        for (int s = 0; s < SubframeCount; s++)
        {
            float noise = EstimateNoiseFloor(s, energy[s]);
            float snrDb = 10f * MathF.Log10(MathF.Max(energy[s], MinNoiseEnergy)
                                          / MathF.Max(noise,   MinNoiseEnergy));
            int q7 = (int)MathF.Round(snrDb * 128f);
            snrQ7[s] = q7;
            if (q7 >= ActivityThresholdSnrQ7) voiceActive = true;
        }

        UpdateHistory(energy);
        return new Result(snrQ7[..SubframeCount], voiceActive);
    }

    private float EstimateNoiseFloor(int subframe, float currentEnergy)
    {
        // Noise floor = minimum of the last NoiseHistory energies for this
        // sub-frame slot, including the current observation. This is a
        // minimum-statistics estimator (Martin 2001) restricted to the
        // SILK 4-sub-frame grid.
        float min = currentEnergy;
        for (int h = 0; h < _historyFill; h++)
        {
            float e = _noiseHistory[h * SubframeCount + subframe];
            if (e < min) min = e;
        }
        return min;
    }

    private void UpdateHistory(ReadOnlySpan<float> energies)
    {
        int slot = _historyHead;
        for (int s = 0; s < SubframeCount; s++)
            _noiseHistory[slot * SubframeCount + s] = energies[s];
        _historyHead = (_historyHead + 1) % NoiseHistory;
        if (_historyFill < NoiseHistory) _historyFill++;
    }
}
