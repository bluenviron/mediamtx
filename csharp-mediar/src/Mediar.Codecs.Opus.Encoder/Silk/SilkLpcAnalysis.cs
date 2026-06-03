namespace Mediar.Codecs.Opus.Encoder.Silk;

/// <summary>
/// SILK LPC analysis pipeline — Burg-method coefficient computation
/// plus bandwidth expansion and a basic stability guard, matching the
/// structure of libopus <c>silk/find_LPC.c</c> /
/// <c>silk/bwexpander.c</c>.
/// </summary>
/// <remarks>
/// <para>
/// The encoder calls <see cref="Analyze"/> once per analysis frame to
/// obtain a single "whole-frame" LPC vector that the closed-loop
/// quantiser and the NLSF MSVQ encoder will later subdivide into
/// sub-frame LPCs. Per-sub-frame interpolation between the previous
/// and current NLSF vector lives in <see cref="SilkLsfQuant"/> (a
/// later slice).
/// </para>
/// <para>
/// Bandwidth expansion (<see cref="BandwidthExpand"/>) scales each
/// coefficient by <c>chirp^i</c>, pulling LPC poles slightly toward
/// the origin. This (a) widens formant bandwidths, reducing
/// perceptual artefacts when the coefficients are quantised, and (b)
/// pre-conditions the filter for NLSF quantisation by avoiding poles
/// too close to the unit circle.
/// </para>
/// <para>References: libopus <c>silk/find_LPC.c</c>,
/// <c>silk/bwexpander_32.c</c>; RFC 6716 §4.2.7.5.</para>
/// </remarks>
public static class SilkLpcAnalysis
{
    /// <summary>Default bandwidth-expansion chirp factor used in libopus encoder.</summary>
    public const float DefaultChirp = 0.99f;

    /// <summary>
    /// Perform LPC analysis on an analysis window.
    /// </summary>
    /// <param name="window">Input samples (already windowed and pre-emphasised by the caller).</param>
    /// <param name="lpc">LPC coefficient destination, length <paramref name="order"/>.</param>
    /// <param name="order">LPC order — 10 for NB/MB, 16 for WB in SILK.</param>
    /// <param name="chirp">Bandwidth-expansion chirp factor (≤ 1). Use 1 to disable.</param>
    /// <returns>Residual energy after the LPC inverse filter.</returns>
    public static float Analyze(ReadOnlySpan<float> window, Span<float> lpc, int order, float chirp = DefaultChirp)
    {
        float residual = SilkAutocorr.Burg(window, lpc, order);
        if (chirp < 1f) BandwidthExpand(lpc, order, chirp);
        return residual;
    }

    /// <summary>
    /// Apply chirp bandwidth expansion: <c>a[i] ← chirp^(i+1) · a[i]</c>.
    /// </summary>
    public static void BandwidthExpand(Span<float> lpc, int order, float chirp)
    {
        if (chirp <= 0f || chirp > 1f)
            throw new ArgumentOutOfRangeException(nameof(chirp), "Chirp must be in (0, 1].");
        float factor = chirp;
        for (int i = 0; i < order; i++)
        {
            lpc[i] *= factor;
            factor *= chirp;
        }
    }

    /// <summary>
    /// Quick stability check via the Schur (a.k.a. reflection-coefficient)
    /// recursion: convert the direct-form LPC vector to reflection
    /// coefficients and report whether every <c>|k_i| &lt; 1</c>.
    /// </summary>
    /// <remarks>
    /// libopus uses a richer stability check (<c>silk_LPC_inverse_pred_gain</c>)
    /// that also bounds the prediction gain; that variant lands with the
    /// NLSF quantiser. For the foundation slice this Schur-test is
    /// sufficient to flag pathological windows that Burg pushed to the
    /// unit circle.
    /// </remarks>
    public static bool IsStable(ReadOnlySpan<float> lpc, int order)
    {
        if (order < 1) return true;
        // Recover reflection coefficients from direct-form LPC via the
        // step-down recursion (inverse of Levinson-Durbin).
        Span<float> a = order <= 64 ? stackalloc float[order] : new float[order];
        Span<float> tmp = order <= 64 ? stackalloc float[order] : new float[order];
        for (int i = 0; i < order; i++) a[i] = lpc[i];

        for (int m = order - 1; m >= 0; m--)
        {
            float k = a[m];
            if (float.IsNaN(k) || k >= 1f || k <= -1f) return false;
            float denom = 1f - k * k;
            if (denom <= 0f) return false;
            for (int i = 0; i < m; i++) tmp[i] = a[i];
            for (int i = 0; i < m; i++)
                a[i] = (tmp[i] - k * tmp[m - 1 - i]) / denom;
        }
        return true;
    }
}
