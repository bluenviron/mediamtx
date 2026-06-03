using Mediar.Codecs.Opus.Decoder.Celt;

namespace Mediar.Codecs.Opus.Encoder.Celt;

/// <summary>
/// Encoder-side CELT band-energy quantiser. Ports the three-stage write
/// path that mirrors the decoder's <c>DecodeCoarseEnergy</c>,
/// <c>UnquantFineEnergy</c>, and <c>UnquantEnergyFinalise</c>:
/// <list type="bullet">
///   <item><description>Coarse pass — Laplace-coded predicted residual using
///     the shared <c>e_prob_model</c> tables (libopus
///     <c>celt/quant_bands.c:quant_coarse_energy</c>).</description></item>
///   <item><description>Fine pass — per-band raw refinement bits at the
///     bit count chosen by the allocator (libopus <c>quant_fine_energy</c>).</description></item>
///   <item><description>Finalise pass — spend any remaining bit budget on
///     additional ±½-LSB fine refinements (libopus
///     <c>quant_energy_finalise</c>).</description></item>
/// </list>
/// All three routines mirror the matching decoder code in
/// <c>Mediar.Codecs.Opus.Decoder.Celt</c> step-for-step so the encoder
/// and decoder always agree on the per-frame energy envelope, and reuse
/// the decoder's spec-derived tables (<see cref="CeltConstants"/>) via
/// the encoder project's <c>InternalsVisibleTo</c> grant.
/// </summary>
/// <remarks>
/// References:
/// <list type="bullet">
///   <item><description>RFC 6716 §4.3.2.1 — energy quantisation.</description></item>
///   <item><description>libopus <c>celt/quant_bands.c</c>.</description></item>
/// </list>
/// </remarks>
internal static class CeltEnergyQuant
{
    /// <summary>
    /// Encode the coarse band energies. Mirrors
    /// <c>DecodeCoarseEnergy</c> in the decoder: for each (band, channel)
    /// it picks <c>qi</c> = round(target − predicted), encodes it as a
    /// Laplace residual, and updates the running prediction state.
    /// </summary>
    /// <param name="enc">Range encoder.</param>
    /// <param name="logE">Per-band target log-energy (Q10) the encoder
    /// wants to convey; layout <c>logE[c * logStride + i]</c>.</param>
    /// <param name="oldLogE">Previous-frame quantised log-energy
    /// (Q10) — read for prediction, then overwritten in place with the
    /// new frame's quantised log-energy (matching the decoder's state).</param>
    /// <param name="totalBits">Total bits available in the packet (used
    /// to decide between the Laplace path and the small-budget fallbacks).</param>
    /// <param name="intra">Whether the frame is intra-coded (skip
    /// inter-frame prediction).</param>
    /// <param name="lm">CELT layer-mode index (0..3).</param>
    /// <param name="start">First band.</param>
    /// <param name="end">One past the last band.</param>
    /// <param name="channels">1 (mono) or 2 (stereo).</param>
    /// <param name="logStride">Per-channel stride for <paramref name="logE"/>
    /// and <paramref name="oldLogE"/>.</param>
    public static void QuantCoarseEnergy(
        ref OpusRangeEncoder enc,
        ReadOnlySpan<float> logE,
        Span<float> oldLogE,
        int totalBits,
        bool intra,
        int lm,
        int start,
        int end,
        int channels,
        int logStride)
    {
        if (channels is not (1 or 2))
            throw new ArgumentOutOfRangeException(nameof(channels));
        if (logE.Length < channels * logStride)
            throw new ArgumentException("logE too short.", nameof(logE));
        if (oldLogE.Length < channels * logStride)
            throw new ArgumentException("oldLogE too short.", nameof(oldLogE));

        int alphaCoefQ15 = intra ? 0 : CeltConstants.PredCoef[lm];
        int betaQ15 = intra ? CeltConstants.BetaIntra : CeltConstants.BetaCoef[lm];
        int probOffset = lm * 84 + (intra ? 42 : 0);
        var probModel = CeltConstants.EProbModel.Slice(probOffset, 42);

        Span<float> prev = stackalloc float[2];
        prev[0] = 0f;
        prev[1] = 0f;

        for (int i = start; i < end; i++)
        {
            for (int c = 0; c < channels; c++)
            {
                int bandIdx = c * logStride + i;
                float curOldE = MathF.Max(CeltConstants.DbMinClamp, oldLogE[bandIdx]);

                // Predicted log-energy in the same Q10 frame as oldLogE.
                // From decoder: tmp = (alpha * curOldE)/256 + prev[c] + q*128
                //               oldLogE_new = tmp / 128
                // Solving for q given target == oldLogE_new:
                //   q = (target * 128 - (alpha * curOldE)/256 - prev[c]) / 128
                // And qi = round(q / DbUnit) is the residual symbol.
                float target = logE[bandIdx];
                float predicted = ((alphaCoefQ15 * curOldE) * (1f / 256f) + prev[c]) * (1f / 128f);
                float residual = (target - predicted) / CeltConstants.DbUnit;
                int qi = (int)MathF.Round(residual);

                int budget = totalBits - enc.Tell();
                if (budget - 15 >= 0)
                {
                    int pi = 2 * Math.Min(i, 20);
                    uint fs = (uint)(probModel[pi] << 7);
                    int decay = probModel[pi + 1] << 6;
                    qi = enc.EncodeLaplace(qi, fs, decay);
                }
                else if (budget - 2 >= 0)
                {
                    if (qi < -1) qi = -1;
                    if (qi > 1) qi = 1;
                    int sym = (qi >> 31) ^ (qi << 1);
                    if (sym > 2) sym = 2;
                    enc.EncodeIcdf(sym, CeltConstants.SmallEnergyIcdf, 2);
                    qi = (sym >> 1) ^ -(sym & 1);
                }
                else if (budget - 1 >= 0)
                {
                    if (qi > 0) qi = 0;
                    if (qi < -1) qi = -1;
                    enc.EncodeBitLogP(-qi, 1);
                }
                else
                {
                    qi = -1;
                }

                float q = qi * CeltConstants.DbUnit;
                float tmp = (alphaCoefQ15 * curOldE) * (1f / 256f) + prev[c] + q * 128f;
                oldLogE[bandIdx] = tmp * (1f / 128f);
                prev[c] = prev[c] + q * 128f - betaQ15 * qi;
            }
        }
    }

    /// <summary>
    /// Encode the per-band fine refinement bits. Mirrors
    /// <c>UnquantFineEnergy</c> in the decoder: for each band with
    /// <paramref name="fineQuant"/>[i] &gt; 0, emit <c>fineQuant[i]</c>
    /// raw bits per channel encoding the residual between the
    /// target log-energy and the coarse-quantised <paramref name="oldLogE"/>.
    /// The same routine then updates <paramref name="oldLogE"/> with the
    /// fine offset so subsequent calls see the encoder/decoder agreed
    /// state.
    /// </summary>
    public static void QuantFineEnergy(
        ref OpusRangeEncoder enc,
        ReadOnlySpan<float> logE,
        Span<float> oldLogE,
        ReadOnlySpan<int> fineQuant,
        int start,
        int end,
        int channels,
        int logStride)
    {
        const int Half = 1 << (CeltConstants.DbShift - 1);
        for (int i = start; i < end; i++)
        {
            int eb = fineQuant[i];
            if (eb <= 0) continue;
            int max = 1 << eb;
            for (int c = 0; c < channels; c++)
            {
                int bandIdx = c * logStride + i;
                // Inverse of decoder offset:
                //   offset = (((q2 << DbShift) + Half) >> eb) - Half
                // Solve for q2 given target offset = (logE - oldLogE):
                //   q2 = round((offset + Half) << eb >> DbShift)
                // but we instead pick q2 minimising |offset_actual - target|.
                float target = logE[bandIdx] - oldLogE[bandIdx];
                int q2 = (int)MathF.Round(((target + Half) * max) / CeltConstants.DbUnit);
                if (q2 < 0) q2 = 0;
                if (q2 >= max) q2 = max - 1;
                enc.EncodeBitsRaw((uint)q2, eb);
                int offsetI = (((q2 << CeltConstants.DbShift) + Half) >> eb) - Half;
                oldLogE[bandIdx] += offsetI;
            }
        }
    }

    /// <summary>
    /// Spend any range-coder budget left after PVQ on additional ±½-LSB
    /// fine refinements. Mirror of the decoder's
    /// <see cref="CeltBands.UnquantEnergyFinalise"/>.
    /// </summary>
    public static void QuantEnergyFinalise(
        ref OpusRangeEncoder enc,
        ReadOnlySpan<float> logE,
        Span<float> oldLogE,
        ReadOnlySpan<int> fineQuant,
        ReadOnlySpan<int> finePriority,
        int start,
        int end,
        int channels,
        int logStride,
        ref int bitsLeft)
    {
        for (int prio = 0; prio < 2; prio++)
        {
            for (int i = start; i < end && bitsLeft >= channels; i++)
            {
                int eb = fineQuant[i];
                if (eb >= CeltConstants.MaxFineBits || finePriority[i] != prio)
                    continue;
                for (int c = 0; c < channels; c++)
                {
                    int bandIdx = c * logStride + i;
                    int magnitude = 1 << (8 - eb);
                    int q2 = logE[bandIdx] > oldLogE[bandIdx] ? 1 : 0;
                    enc.EncodeBitsRaw((uint)q2, 1);
                    float offset = q2 == 1 ? magnitude : -magnitude;
                    oldLogE[bandIdx] += offset;
                    bitsLeft--;
                }
            }
        }
    }
}
