namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// Decoded post-filter parameters for one CELT frame (RFC 6716 §4.3.2).
/// Phase 2b parses these out of the bitstream; the actual comb-filter
/// application lands in Phase 2d together with the IMDCT path.
/// </summary>
internal readonly record struct CeltPostFilterParams(
    bool Enabled,
    int Octave,
    int PitchPeriod,
    int GainQ8,
    int Tapset)
{
    public static CeltPostFilterParams Disabled => default;
}

/// <summary>
/// CELT layer decoder. Owned per-stream by <see cref="OpusDecoder"/>.
/// </summary>
/// <remarks>
/// <para>
/// <b>Phased delivery</b>:
/// </para>
/// <list type="bullet">
///   <item><description>Phase 2a — foundation: state, mode resolution, decode-frame skeleton.</description></item>
///   <item><description>Phase 2b (this commit) — silence / post-filter / transient / intra flags + coarse energy decode.</description></item>
///   <item><description>Phase 2c — PVQ shape decode + bit allocation + anti-collapse + mid-side stereo.</description></item>
///   <item><description>Phase 2d — IMDCT, post-filter, window overlap-add → first real PCM samples.</description></item>
/// </list>
/// <para>
/// Phase 2b decodes the front-of-packet flag set and the Laplace-coded
/// coarse-energy log-spectrum, but still emits silence for the audio
/// output. The decoded state — old log-energies, post-filter parameters,
/// transient flag — is exposed via internal properties so Phase 2c/2d
/// can plug in cleanly.
/// </para>
/// </remarks>
internal sealed class CeltDecoder
{
    private readonly int _channels;

    // Log-energy storage in DB_SHIFT (Q10) units: stored = log2(energy) × 1024.
    // Layout: _oldLogE[channel * MaxBands + band].
    private readonly float[] _oldLogE;

    // Per-band TF resolution adjustment from Phase 2c.1. One entry per
    // band (only [StartBand, EndBand) are meaningful). Values are
    // signed and feed the MDCT layer offset during synthesis.
    private readonly sbyte[] _tfRes;

    // Per-band bit-allocation caps from Phase 2c.2a (init_caps).
    // Units: fractional bits (1 / (1<<BitRes) of a whole bit).
    private readonly int[] _caps;

    // Per-band dyn_alloc boost from Phase 2c.2a (dyn_alloc loop).
    // Units: fractional bits, same as _caps.
    private readonly int[] _boost;

    /// <summary>The active band layout for this decoder.</summary>
    public CeltMode Mode { get; }

    /// <summary>Number of output channels (1 = mono, 2 = stereo).</summary>
    public int Channels => _channels;

    /// <summary>
    /// Whether the next frame should be treated as the start of a new
    /// stream (no overlap-add history). Set by <see cref="Reset"/> and
    /// cleared after the first <see cref="DecodeFrame"/> call.
    /// </summary>
    public bool IsFirstFrame { get; private set; } = true;

    /// <summary>
    /// Total decoded sample count (per channel) the decoder has produced
    /// since construction or the most recent <see cref="Reset"/>.
    /// </summary>
    public long SamplesProduced { get; private set; }

    /// <summary>
    /// Whether the most recently decoded frame was flagged silent (the
    /// silence shortcut in RFC 6716 §4.3.2). When true the entropy
    /// payload is skipped and the output is zeroed.
    /// </summary>
    public bool LastFrameWasSilent { get; private set; }

    /// <summary>
    /// Whether the most recently decoded frame was flagged transient
    /// (split into <see cref="CeltMode.ShortBlocksPerFrame"/> short MDCTs).
    /// Always false on 2.5 ms frames (LM == 0).
    /// </summary>
    public bool LastFrameWasTransient { get; private set; }

    /// <summary>
    /// Whether the most recently decoded frame used intra coding for the
    /// coarse-energy predictor.
    /// </summary>
    public bool LastFrameUsedIntra { get; private set; }

    /// <summary>Decoded post-filter parameters for the most recent frame.</summary>
    public CeltPostFilterParams LastPostFilter { get; private set; } = CeltPostFilterParams.Disabled;

    /// <summary>
    /// Decoded spread mode for the most recent frame. One of
    /// <see cref="CeltConstants.SpreadNone"/>..<see cref="CeltConstants.SpreadAggressive"/>.
    /// Defaults to <c>SpreadNormal</c> when the budget did not admit the symbol.
    /// </summary>
    public int LastSpreadDecision { get; private set; } = CeltConstants.SpreadNormal;

    /// <summary>
    /// Per-band TF (time-frequency) resolution offset from the most
    /// recent frame. Only entries in <c>[Mode.StartBand, Mode.EndBand)</c>
    /// are meaningful; values outside that range read as 0.
    /// </summary>
    public ReadOnlySpan<sbyte> LastTfResolution => _tfRes;

    /// <summary>
    /// Per-band allocation caps from the most recent frame (Phase 2c.2a
    /// init_caps). Units: fractional bits (1 / (1&lt;&lt;BitRes) of a
    /// whole bit). Always populated, regardless of bit budget — caps are
    /// table-driven and do not consume entropy.
    /// </summary>
    public ReadOnlySpan<int> LastBandCaps => _caps;

    /// <summary>
    /// Per-band dyn_alloc boost from the most recent frame (Phase 2c.2a).
    /// Units: fractional bits. Zero outside <c>[Mode.StartBand, Mode.EndBand)</c>
    /// and for bands that received no boost.
    /// </summary>
    public ReadOnlySpan<int> LastBandBoost => _boost;

    /// <summary>
    /// Decoded <c>alloc_trim</c> value from the most recent frame
    /// (Phase 2c.2a). One of 0..10; defaults to
    /// <see cref="CeltConstants.AllocTrimDefault"/> when the bit budget
    /// did not admit the symbol.
    /// </summary>
    public int LastAllocTrim { get; private set; } = CeltConstants.AllocTrimDefault;

    /// <summary>
    /// Read-only view over the per-band log-energy state. Stored in
    /// DB_SHIFT units (multiply by <c>1/1024</c> to recover log2 energy).
    /// Layout matches libopus: <c>channel * MaxBands + band</c>.
    /// </summary>
    public ReadOnlySpan<float> OldLogE => _oldLogE;

    /// <summary>
    /// Create a decoder for the given band layout and channel count.
    /// </summary>
    public CeltDecoder(CeltMode mode, int channels)
    {
        if (channels is < 1 or > 2)
            throw new ArgumentOutOfRangeException(nameof(channels), "CELT supports 1 or 2 channels per stream.");
        if (mode.SamplesPerFrame <= 0)
            throw new ArgumentException("CeltMode is uninitialised (SamplesPerFrame == 0).", nameof(mode));

        Mode = mode;
        _channels = channels;
        _oldLogE = new float[channels * CeltConstants.MaxBands];
        _tfRes = new sbyte[CeltConstants.MaxBands];
        _caps = new int[CeltConstants.MaxBands];
        _boost = new int[CeltConstants.MaxBands];
    }

    /// <summary>
    /// Decode one CELT frame from the range decoder into
    /// <paramref name="output"/>. Buffer must be at least
    /// <c>Mode.SamplesPerFrame × Channels</c> floats long.
    /// </summary>
    /// <remarks>
    /// Phase 2b reads the silence / post-filter / transient / intra flags
    /// and the Laplace-coded coarse-energy spectrum; the actual PCM
    /// reconstruction (PVQ + IMDCT + post-filter) still emits silence
    /// until Phase 2c/2d ship.
    /// </remarks>
    public int DecodeFrame(ref OpusRangeDecoder rangeDecoder, Span<float> output)
    {
        int needed = Mode.SamplesPerFrame * _channels;
        if (output.Length < needed)
            throw new ArgumentException(
                $"Output buffer is too small: need {needed} floats, got {output.Length}.",
                nameof(output));

        var outSpan = output.Slice(0, needed);
        outSpan.Clear();

        int totalBits = rangeDecoder.BufferLength * 8;
        int lm = CeltConstants.LmFor(Mode.SamplesPerFrame);

        // 1. Silence flag — present whenever the budget admits the 15-bit
        //    symbol. Matches libopus celt_decode_with_ec (tell+15 ≤ total).
        bool silence = false;
        if (rangeDecoder.Tell() + 15 <= totalBits)
        {
            silence = rangeDecoder.DecodeBitLogP(15) != 0;
        }

        if (silence)
        {
            // RFC: silent frames clamp the energy state to a very low
            // value and stop consuming bits from this packet.
            for (int i = 0; i < _oldLogE.Length; i++)
                _oldLogE[i] = CeltConstants.DbSilentReplacement;

            LastFrameWasSilent = true;
            LastFrameWasTransient = false;
            LastFrameUsedIntra = false;
            LastPostFilter = CeltPostFilterParams.Disabled;
            IsFirstFrame = false;
            SamplesProduced += Mode.SamplesPerFrame;
            return Mode.SamplesPerFrame;
        }

        LastFrameWasSilent = false;

        // 2. Post-filter parameters (CELT-only path).
        LastPostFilter = CeltPostFilterParams.Disabled;
        if (Mode.StartBand == 0 && rangeDecoder.Tell() + 16 <= totalBits)
        {
            if (rangeDecoder.DecodeBitLogP(1) != 0)
            {
                int octave = (int)rangeDecoder.DecodeUint(6);
                int rawPeriod = (int)rangeDecoder.DecodeBits(4 + octave);
                int period = (16 << octave) + rawPeriod - 1;
                int qg = (int)rangeDecoder.DecodeBits(3);
                int tapset = 0;
                if (rangeDecoder.Tell() + 2 <= totalBits)
                {
                    tapset = rangeDecoder.DecodeIcdf(CeltConstants.TapsetIcdf, 2);
                }
                int gainQ8 = 24 * (qg + 1); // QCONST16(0.09375f, 15) ≈ 24 in Q8.
                LastPostFilter = new CeltPostFilterParams(true, octave, period, gainQ8, tapset);
            }
        }

        // 3. Transient flag — meaningful only when LM > 0 (multiple short blocks possible).
        bool isTransient = false;
        if (lm > 0 && rangeDecoder.Tell() + 3 <= totalBits)
        {
            isTransient = rangeDecoder.DecodeBitLogP(3) != 0;
        }
        LastFrameWasTransient = isTransient;

        // 4. Intra-energy flag.
        bool intraEnergy = false;
        if (rangeDecoder.Tell() + 3 <= totalBits)
        {
            intraEnergy = rangeDecoder.DecodeBitLogP(3) != 0;
        }
        LastFrameUsedIntra = intraEnergy;

        // 5. Coarse band energies (Laplace-coded with linear prediction).
        DecodeCoarseEnergy(ref rangeDecoder, totalBits, intraEnergy, lm);

        // 6. Per-band time-frequency resolution offsets (RFC 6716 §4.3.4.5).
        Array.Clear(_tfRes);
        DecodeTfChanges(ref rangeDecoder, totalBits, isTransient, lm);

        // 7. Spread decision (RFC 6716 §4.3.4.3).
        if (rangeDecoder.Tell() + 4 <= totalBits)
        {
            LastSpreadDecision = rangeDecoder.DecodeIcdf(CeltConstants.SpreadIcdf, 5);
        }
        else
        {
            LastSpreadDecision = CeltConstants.SpreadNormal;
        }

        // 8. Per-band allocation caps (no entropy — pure table lookup).
        Array.Clear(_caps);
        InitCaps(lm);

        // 9. dyn_alloc — per-band boost loop. Operates in *fractional*
        //    bits (libopus BITRES = 3, so 1 whole bit = 8 frac units).
        //    We track totalBitsFrac as a local since dyn_alloc shrinks
        //    the remaining budget as boost is allocated.
        Array.Clear(_boost);
        long totalBitsFrac = (long)totalBits << CeltConstants.BitRes;
        totalBitsFrac = DecodeDynAlloc(ref rangeDecoder, totalBitsFrac, lm);

        // 10. alloc_trim (RFC 6716 §4.3.3) — global trim biasing
        //     allocation towards low or high bands.
        if (rangeDecoder.TellFrac() + (6 << CeltConstants.BitRes) <= totalBitsFrac)
        {
            LastAllocTrim = rangeDecoder.DecodeIcdf(CeltConstants.AllocTrimIcdf, 7);
        }
        else
        {
            LastAllocTrim = CeltConstants.AllocTrimDefault;
        }

        // Phase 2c.2b+ ships compute_allocation + intensity/dual stereo +
        // skip, then 2c.3 ships fine energy + PVQ shape decode, 2c.4
        // ships anti-collapse + final energy, and Phase 2d ships the
        // IMDCT pipeline. For now the output remains silent — but the
        // decoded state above is observable for tests.

        IsFirstFrame = false;
        SamplesProduced += Mode.SamplesPerFrame;
        return Mode.SamplesPerFrame;
    }

    private void DecodeTfChanges(ref OpusRangeDecoder rd, int totalBits, bool isTransient, int lm)
    {
        // Port of libopus tf_decode (celt/celt_decoder.c).
        int logp = isTransient ? 2 : 4;
        bool tfSelectRsv = lm > 0 && rd.Tell() + logp + 1 <= totalBits;
        int budget = totalBits - (tfSelectRsv ? 1 : 0);

        int curr = 0;
        bool tfChanged = false;
        for (int i = Mode.StartBand; i < Mode.EndBand; i++)
        {
            if (rd.Tell() + logp <= budget)
            {
                curr ^= rd.DecodeBitLogP(logp);
                if (curr != 0) tfChanged = true;
            }
            _tfRes[i] = (sbyte)curr;
            logp = isTransient ? 4 : 5;
        }

        int tfSelect = 0;
        int isT = isTransient ? 1 : 0;
        int changed = tfChanged ? 1 : 0;
        if (tfSelectRsv &&
            CeltConstants.TfSelectTable[lm * 8 + 4 * isT + 0 + changed] !=
            CeltConstants.TfSelectTable[lm * 8 + 4 * isT + 2 + changed])
        {
            tfSelect = rd.DecodeBitLogP(1);
        }

        for (int i = Mode.StartBand; i < Mode.EndBand; i++)
        {
            int tableIdx = lm * 8 + 4 * isT + 2 * tfSelect + _tfRes[i];
            _tfRes[i] = CeltConstants.TfSelectTable[tableIdx];
        }
    }

    private void InitCaps(int lm)
    {
        // Port of libopus init_caps (celt/celt.c).
        //   cap[i] = (cache.caps[nbEBands*(2*LM + C-1) + i] + 64) * C * N >> 2
        // where N = (eBands[i+1] - eBands[i]) << LM and C is channel count.
        int rowOffset = CeltConstants.MaxBands * (2 * lm + (_channels - 1));
        var caps = CeltConstants.CacheCaps50;
        var eBands = CeltConstants.EBands;
        for (int i = 0; i < CeltConstants.MaxBands; i++)
        {
            int n = (eBands[i + 1] - eBands[i]) << lm;
            _caps[i] = ((caps[rowOffset + i] + 64) * _channels * n) >> 2;
        }
    }

    private long DecodeDynAlloc(ref OpusRangeDecoder rd, long totalBitsFrac, int lm)
    {
        // Port of libopus dyn_alloc loop (celt/celt_decoder.c). All
        // budget accounting is in fractional bits (1/(1<<BitRes) bit).
        // Bands outside [StartBand, EndBand) get zero boost.
        int dynallocLogP = CeltConstants.DynAllocLogPStart;
        var eBands = CeltConstants.EBands;

        for (int i = Mode.StartBand; i < Mode.EndBand; i++)
        {
            int width = _channels * (eBands[i + 1] - eBands[i]) << lm;
            // quanta = min(width<<BITRES, max(6<<BITRES, width))
            int quanta = Math.Min(width << CeltConstants.BitRes,
                                  Math.Max(6 << CeltConstants.BitRes, width));
            int dynallocLoopLogP = dynallocLogP;
            int boost = 0;
            while (rd.TellFrac() + ((long)dynallocLoopLogP << CeltConstants.BitRes) < totalBitsFrac
                   && boost < _caps[i])
            {
                int flag = rd.DecodeBitLogP(dynallocLoopLogP);
                if (flag == 0) break;
                boost += quanta;
                totalBitsFrac -= quanta;
                dynallocLoopLogP = 1;
            }
            _boost[i] = boost;
            if (boost > 0)
                dynallocLogP = Math.Max(2, dynallocLogP - 1);
        }
        return totalBitsFrac;
    }

    private void DecodeCoarseEnergy(ref OpusRangeDecoder rd, int totalBits, bool intra, int lm)
    {
        int alphaCoefQ15 = intra ? 0 : CeltConstants.PredCoef[lm];
        int betaQ15 = intra ? CeltConstants.BetaIntra : CeltConstants.BetaCoef[lm];
        int probOffset = lm * 84 + (intra ? 42 : 0);
        var probModel = CeltConstants.EProbModel.Slice(probOffset, 42);

        Span<float> prev = stackalloc float[2];
        prev[0] = 0f;
        prev[1] = 0f;

        for (int i = Mode.StartBand; i < Mode.EndBand; i++)
        {
            for (int c = 0; c < _channels; c++)
            {
                int budget = totalBits - rd.Tell();
                int qi;
                if (budget - 15 >= 0)
                {
                    int pi = 2 * Math.Min(i, 20);
                    uint fs = (uint)(probModel[pi] << 7);
                    int decay = probModel[pi + 1] << 6;
                    qi = rd.DecodeLaplace(fs, decay);
                }
                else if (budget - 2 >= 0)
                {
                    int sym = rd.DecodeIcdf(CeltConstants.SmallEnergyIcdf, 2);
                    qi = (sym >> 1) ^ -(sym & 1);
                }
                else if (budget - 1 >= 0)
                {
                    qi = -rd.DecodeBitLogP(1);
                }
                else
                {
                    qi = -1;
                }

                int bandIdx = c * CeltConstants.MaxBands + i;
                float curOldE = MathF.Max(CeltConstants.DbMinClamp, _oldLogE[bandIdx]);

                // Fixed-point semantics translated literally to float math:
                //   q   = qi << DB_SHIFT        (= qi * 1024)
                //   tmp = (alpha * curOldE) / 256 + prev[c] + q * 128
                //   old = tmp / 128
                //   prev[c] = prev[c] + q * 128 - beta * qi
                float q = qi * CeltConstants.DbUnit;
                float tmp = (alphaCoefQ15 * curOldE) * (1f / 256f) + prev[c] + q * 128f;
                _oldLogE[bandIdx] = tmp * (1f / 128f);
                prev[c] = prev[c] + q * 128f - betaQ15 * qi;
            }
        }
    }

    /// <summary>Clear all decode history (call after a seek).</summary>
    public void Reset()
    {
        IsFirstFrame = true;
        SamplesProduced = 0;
        Array.Clear(_oldLogE);
        Array.Clear(_tfRes);
        Array.Clear(_caps);
        Array.Clear(_boost);
        LastFrameWasSilent = false;
        LastFrameWasTransient = false;
        LastFrameUsedIntra = false;
        LastPostFilter = CeltPostFilterParams.Disabled;
        LastSpreadDecision = CeltConstants.SpreadNormal;
        LastAllocTrim = CeltConstants.AllocTrimDefault;
    }
}
