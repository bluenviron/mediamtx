using Mediar.Codecs.Opus.Encoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// MDCT(IMDCT(x)) round-trip tests for the Phase B2 CELT forward MDCT
/// (<see cref="CeltMdct"/>). The forward and inverse routines must
/// satisfy time-domain aliasing cancellation (Princen &amp; Bradley 1987)
/// when paired with the CELT sine window in a 50 %-overlapped pair of
/// frames.
/// </summary>
public sealed class CeltMdctRoundTripTests
{
    /// <summary>Single-frame MDCT(IMDCT(X)) on impulse / sine / noise.</summary>
    [Theory]
    [InlineData(240)]
    [InlineData(480)]
    [InlineData(960)]
    [InlineData(1920)]
    public void Mdct_Of_Imdct_Recovers_Spectrum(int n)
    {
        var spectrum = new float[n];
        var time = new float[2 * n];
        var recovered = new float[n];

        // Test 1: single-bin impulse.
        for (int k = 0; k < n; k++) spectrum[k] = 0f;
        spectrum[n / 4] = 1f;
        CeltMdct.Inverse(spectrum, time);
        CeltMdct.Forward(time, recovered);
        AssertClose(spectrum, recovered, tolerance: 1e-3f, label: "impulse");

        // Test 2: sine.
        for (int k = 0; k < n; k++)
            spectrum[k] = (float)Math.Sin(2 * Math.PI * k * 7 / n);
        CeltMdct.Inverse(spectrum, time);
        CeltMdct.Forward(time, recovered);
        AssertClose(spectrum, recovered, tolerance: 1e-2f, label: "sine");

        // Test 3: deterministic noise.
        var rng = new Random(seed: n);
        for (int k = 0; k < n; k++)
            spectrum[k] = (float)(rng.NextDouble() * 2 - 1);
        CeltMdct.Inverse(spectrum, time);
        CeltMdct.Forward(time, recovered);
        AssertClose(spectrum, recovered, tolerance: 1e-2f, label: "noise");
    }

    /// <summary>
    /// Two-frame TDAC: with the power-complementary sine window
    /// (<c>w[i]² + w[i+N]² = 1</c>), MDCT(w·x_f) → IMDCT → w then
    /// overlap-add of consecutive frames recovers the original signal in
    /// the overlap region modulo the IMDCT scale factor. Verifies the
    /// recovered signal is proportional to the input (constant ratio).
    /// </summary>
    [Theory]
    [InlineData(240)]
    [InlineData(480)]
    [InlineData(960)]
    public void Mdct_Pair_With_Window_Satisfies_Tdac(int n)
    {
        var window = CeltMdct.BuildSineWindow(n);

        var rng = new Random(seed: 17 * n);
        int signalLen = 3 * n;
        var signal = new float[signalLen];
        for (int i = 0; i < signalLen; i++)
            signal[i] = (float)(rng.NextDouble() * 2 - 1);

        var spectrum0 = new float[n];
        var spectrum1 = new float[n];
        CeltMdct.Forward(signal.AsSpan(0, 2 * n), spectrum0, window);
        CeltMdct.Forward(signal.AsSpan(n, 2 * n), spectrum1, window);

        var inverse0 = new float[2 * n];
        var inverse1 = new float[2 * n];
        CeltMdct.Inverse(spectrum0, inverse0, window);
        CeltMdct.Inverse(spectrum1, inverse1, window);

        // Recovered overlap region. Compute the proportional ratio between
        // reconstruction and input — TDAC says it must be constant (= 1/2
        // with our 1/N IMDCT scale).
        double sumRatio = 0.0;
        int count = 0;
        for (int i = 0; i < n; i++)
        {
            float reconstructed = inverse0[n + i] + inverse1[i];
            float expected = signal[n + i];
            if (Math.Abs(expected) > 0.1f)
            {
                sumRatio += reconstructed / expected;
                count++;
            }
        }
        double avgRatio = sumRatio / count;
        // Verify the ratio is consistent across all samples (TDAC).
        for (int i = 0; i < n; i++)
        {
            float reconstructed = inverse0[n + i] + inverse1[i];
            float expected = signal[n + i];
            float predicted = (float)(expected * avgRatio);
            Assert.True(Math.Abs(reconstructed - predicted) < 0.05f,
                $"TDAC inconsistency at i={i}: expected {predicted} got {reconstructed} (ratio={avgRatio:F4})");
        }
    }

    private static void AssertClose(ReadOnlySpan<float> expected, ReadOnlySpan<float> actual, float tolerance, string label)
    {
        Assert.Equal(expected.Length, actual.Length);
        for (int i = 0; i < expected.Length; i++)
        {
            float diff = Math.Abs(expected[i] - actual[i]);
            Assert.True(diff < tolerance,
                $"[{label}] mismatch at i={i}: expected={expected[i]} actual={actual[i]} diff={diff}");
        }
    }
}
