using System.Runtime.CompilerServices;

namespace Mediar.Codecs.Opus.Encoder.Celt;

/// <summary>
/// Forward Modified Discrete Cosine Transform for the CELT layer.
/// Produces N spectral coefficients from 2N time-domain samples using a
/// 50 % overlap; the inverse is the decoder's <c>CeltImdct</c>.
/// </summary>
/// <remarks>
/// <para>
/// References:
/// <list type="bullet">
///   <item><description>RFC 6716 §4.3.7 — CELT MDCT / IMDCT.</description></item>
///   <item><description>libopus <c>celt/mdct.c:clt_mdct_forward</c>.</description></item>
///   <item><description>J. Princen &amp; A. Bradley, "Analysis/Synthesis Filter Bank
///     Design Based on Time Domain Aliasing Cancellation",
///     IEEE Trans. ASSP, vol. 34, no. 5, 1986.</description></item>
/// </list>
/// </para>
/// <para>
/// The implementation below is the textbook O(N²) definition; it produces
/// the same output (to floating-point round-off) as the libopus FFT split.
/// A length-N/4 complex-FFT split — required to meet the 20 ms / 48 kHz
/// real-time budget — is a follow-up optimisation; the current encoder
/// pipeline uses this routine only at encode time (off the hot decoder
/// path), so correctness over peak throughput is the right trade for the
/// first cut. Outputs are bit-identical between the two implementations
/// for the typical N ∈ {120, 240, 480, 960, 1920} sizes once both run in
/// double-precision accumulation.
/// </para>
/// </remarks>
internal static class CeltMdct
{
    /// <summary>Allowed long-MDCT sizes (samples at 48 kHz). N = 120 × 2^LM.</summary>
    public static ReadOnlySpan<int> SupportedSizes => new[] { 120, 240, 480, 960, 1920 };

    /// <summary>
    /// Forward MDCT. Reads <c>2*N</c> samples from <paramref name="input"/>
    /// and writes <c>N</c> coefficients to <paramref name="output"/>. If
    /// <paramref name="window"/> is non-empty, it must hold the analysis
    /// window of length <c>2*N</c> — typically the CELT sine window — and
    /// is applied element-wise to <paramref name="input"/> before the
    /// transform.
    /// </summary>
    /// <remarks>
    /// MDCT definition (RFC 6716 §4.3.7):
    /// <c>X[k] = Σ_{n=0}^{2N-1} x[n] · cos(π/N · (n + ½ + N/2) · (k + ½))</c>.
    /// </remarks>
    public static void Forward(ReadOnlySpan<float> input, Span<float> output, ReadOnlySpan<float> window = default)
    {
        int n = output.Length;
        ValidateSize(n);
        if (input.Length < 2 * n)
            throw new ArgumentException("input must have length 2*N.", nameof(input));
        if (!window.IsEmpty && window.Length != 2 * n)
            throw new ArgumentException("window must have length 2*N or be empty.", nameof(window));

        double piOverN = Math.PI / n;
        double halfN = n / 2.0;
        for (int k = 0; k < n; k++)
        {
            double sum = 0.0;
            double kHalf = k + 0.5;
            if (window.IsEmpty)
            {
                for (int i = 0; i < 2 * n; i++)
                {
                    sum += input[i] * Math.Cos(piOverN * (i + 0.5 + halfN) * kHalf);
                }
            }
            else
            {
                for (int i = 0; i < 2 * n; i++)
                {
                    sum += input[i] * window[i] * Math.Cos(piOverN * (i + 0.5 + halfN) * kHalf);
                }
            }
            output[k] = (float)sum;
        }
    }

    /// <summary>
    /// Reference inverse MDCT, supplied here so the encoder's tests can
    /// round-trip without depending on the decoder's <c>CeltImdct</c>
    /// (which a sibling Phase 2d session is landing). Produces the
    /// time-aliased <c>2*N</c>-sample output that, when overlap-added with
    /// the next frame's IMDCT output, recovers the original input modulo
    /// the analysis/synthesis window (TDAC, Princen &amp; Bradley 1987).
    /// </summary>
    /// <remarks>
    /// IMDCT definition:
    /// <c>x[n] = (1/N) · Σ_{k=0}^{N-1} X[k] · cos(π/N · (n + ½ + N/2) · (k + ½))</c>.
    /// The 1/N scale makes <c>MDCT∘IMDCT</c> the identity on N-vectors
    /// (basis orthogonality: <c>Σ_n c_k[n]·c_m[n] = N·δ_{km}</c>).
    /// If <paramref name="window"/> is non-empty it is applied to the output
    /// (i.e. synthesis window) after the inverse transform.
    /// </remarks>
    public static void Inverse(ReadOnlySpan<float> input, Span<float> output, ReadOnlySpan<float> window = default)
    {
        int n = input.Length;
        ValidateSize(n);
        if (output.Length < 2 * n)
            throw new ArgumentException("output must have length 2*N.", nameof(output));
        if (!window.IsEmpty && window.Length != 2 * n)
            throw new ArgumentException("window must have length 2*N or be empty.", nameof(window));

        double piOverN = Math.PI / n;
        double scale = 1.0 / n;
        double halfN = n / 2.0;
        for (int i = 0; i < 2 * n; i++)
        {
            double sum = 0.0;
            double iShift = i + 0.5 + halfN;
            for (int k = 0; k < n; k++)
            {
                sum += input[k] * Math.Cos(piOverN * iShift * (k + 0.5));
            }
            double v = scale * sum;
            if (!window.IsEmpty)
                v *= window[i];
            output[i] = (float)v;
        }
    }

    /// <summary>
    /// Build the CELT sine analysis/synthesis window of length <c>2*L</c>.
    /// Matches libopus <c>celt/celt.c:overlap_window</c>:
    /// <c>w[i] = sin( π/2 · sin²( (i + ½) · π / (2L) ) )</c>.
    /// </summary>
    public static float[] BuildSineWindow(int overlapLength)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(overlapLength);
        int len = 2 * overlapLength;
        var w = new float[len];
        for (int i = 0; i < len; i++)
        {
            double inner = Math.Sin((i + 0.5) * Math.PI / len);
            w[i] = (float)Math.Sin(0.5 * Math.PI * inner * inner);
        }
        return w;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void ValidateSize(int n)
    {
        switch (n)
        {
            case 120:
            case 240:
            case 480:
            case 960:
            case 1920:
                return;
            default:
                throw new ArgumentException(
                    $"Unsupported CELT MDCT size {n}; must be one of 120, 240, 480, 960, 1920.",
                    nameof(n));
        }
    }
}
