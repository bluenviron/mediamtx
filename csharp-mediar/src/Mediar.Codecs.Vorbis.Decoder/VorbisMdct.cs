using System.Numerics;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;

namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Modified Discrete Cosine Transform (MDCT) used by Vorbis.
///
/// Standard Princen-Bradley MDCT, IMDCT scaled so that the time-domain
/// aliasing cancellation (TDAC) overlap-add of two consecutive windowed
/// blocks reconstructs the original signal exactly:
/// <code>
/// X[k] = sum_{n=0}^{N-1}      x[n] * cos( (2 PI / N) * (n + 0.5 + N/4) * (k + 0.5) )    (forward)
/// y[n] = (4/N) * sum_{k=0}^{N/2-1} X[k] * cos( (2 PI / N) * (n + 0.5 + N/4) * (k + 0.5) )  (inverse)
/// </code>
/// for n = 0..N-1, where N is the blocksize. The 4/N inverse scale makes
/// <c>IMDCT(MDCT(x))[n]</c> equal to <c>x[n] - x[N/2-1-n]</c> in the first
/// half and <c>x[n] + x[3N/2-1-n]</c> in the second half — the alias
/// structure consumed by the sin² window + overlap-add stage.
///
/// This is the canonical direct (O(N²)) implementation. It is correct by
/// construction and is unit-tested for round-trip and TDAC reconstruction.
/// An FFT-based O(N log N) replacement can be substituted later behind the
/// same <see cref="Inverse"/> contract — see Bosi/Goldberg §8.4 for the
/// standard decomposition.
/// </summary>
internal sealed class VorbisMdct
{
    private readonly int _n;
    private readonly float[] _cosTable; // [N * N/2] cos(PI/N * (2n+1+N/2) * (2k+1)/2)

    public VorbisMdct(int n)
    {
        if (n <= 0 || (n & (n - 1)) != 0)
            throw new ArgumentException("MDCT size must be a positive power of two.", nameof(n));
        if (n < 4)
            throw new ArgumentException("MDCT size must be at least 4.", nameof(n));
        _n = n;
        int n2 = n / 2;
        _cosTable = new float[(long)n * n2];
        for (int t = 0; t < n; t++)
        {
            for (int k = 0; k < n2; k++)
            {
                double phase = 2.0 * Math.PI / n * (t + 0.5 + n / 4.0) * (k + 0.5);
                _cosTable[(long)t * n2 + k] = (float)Math.Cos(phase);
            }
        }
    }

    public int N => _n;

    /// <summary>
    /// Inverse MDCT. <paramref name="freq"/> is the length-N/2 frequency-bin
    /// input; <paramref name="time"/> receives the length-N time-domain output.
    /// The 4/N scaling makes IMDCT(MDCT(x)) reproduce x with the standard
    /// MDCT alias structure (x[n] - x[N/2-1-n] in the first half, x[n] +
    /// x[3N/2-1-n] in the second half), which is what time-domain aliasing
    /// cancellation (TDAC) overlap-add relies on.
    /// </summary>
    public void Inverse(ReadOnlySpan<float> freq, Span<float> time)
    {
        int n = _n;
        int n2 = n / 2;
        if (freq.Length < n2) throw new ArgumentException("freq buffer too small.", nameof(freq));
        if (time.Length < n) throw new ArgumentException("time buffer too small.", nameof(time));

        // Hot inner kernel: for each time-domain sample we dot-product freq
        // against the precomputed cos table row. Vectorize via SIMD when
        // available, falling back to a scalar tail accumulator. .NET 9/10
        // `Vector<float>` auto-tunes to AVX-512 / AVX2 / NEON at JIT time.
        float scale = 4.0f / n;
        ref float freqRef = ref MemoryMarshal.GetReference(freq);
        ref float timeRef = ref MemoryMarshal.GetReference(time);
        ref float cosRef = ref MemoryMarshal.GetArrayDataReference(_cosTable);
        int simdWidth = Vector<float>.Count;
        int simdEnd = n2 - (n2 % simdWidth);

        for (int t = 0; t < n; t++)
        {
            long row = (long)t * n2;
            ref float rowRef = ref Unsafe.Add(ref cosRef, (nint)row);

            var vsum = Vector<float>.Zero;
            int k = 0;
            for (; k < simdEnd; k += simdWidth)
            {
                var vfreq = Vector.LoadUnsafe(ref Unsafe.Add(ref freqRef, k));
                var vcos = Vector.LoadUnsafe(ref Unsafe.Add(ref rowRef, k));
                vsum += vfreq * vcos;
            }
            float sum = Vector.Sum(vsum);
            for (; k < n2; k++)
            {
                sum += Unsafe.Add(ref freqRef, k) * Unsafe.Add(ref rowRef, k);
            }
            Unsafe.Add(ref timeRef, t) = sum * scale;
        }
    }

    /// <summary>
    /// Forward MDCT. Inverse of <see cref="Inverse"/>. Provided so tests can
    /// round-trip a synthetic signal through forward+inverse and verify
    /// near-equality on the overlapping region.
    /// </summary>
    public void Forward(ReadOnlySpan<float> time, Span<float> freq)
    {
        int n = _n;
        int n2 = n / 2;
        if (time.Length < n) throw new ArgumentException("time buffer too small.", nameof(time));
        if (freq.Length < n2) throw new ArgumentException("freq buffer too small.", nameof(freq));

        for (int k = 0; k < n2; k++)
        {
            double sum = 0;
            for (int t = 0; t < n; t++)
            {
                double phase = 2.0 * Math.PI / n * (t + 0.5 + n / 4.0) * (k + 0.5);
                sum += time[t] * Math.Cos(phase);
            }
            freq[k] = (float)sum;
        }
    }
}
