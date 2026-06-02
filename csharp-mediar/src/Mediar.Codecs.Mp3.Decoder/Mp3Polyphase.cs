namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// Layer III polyphase synthesis filterbank per ISO 11172-3 §2.4.3.4.11
/// and Annex B Algorithm B.5. Consumes 32 subband samples per row and produces
/// 32 PCM samples per row via an internal 1024-sample V-buffer and a 512-tap
/// dewindow operation.
/// </summary>
/// <remarks>
/// <para>
/// One instance owns the V-buffer for one channel. State persists across
/// granules and frames; <see cref="Reset"/> clears it (call on seek or
/// start-of-stream).
/// </para>
/// <para>
/// The 512-element D-window used here is generated analytically from a
/// windowed-sinc lowpass prototype centered at π/64. This produces coherent
/// output for any subband input but is NOT bit-exact against the ISO 11172-3
/// Annex B Table B.4 values used by reference decoders. For silence inputs
/// the output is identically zero regardless of window values. To upgrade
/// to bit-exact ISO conformance, replace <see cref="DWindow"/> with the 512
/// floats from ISO 11172-3 Annex B Table B.4.
/// </para>
/// </remarks>
internal sealed class Mp3Polyphase
{
    /// <summary>1024-sample V-buffer (16 history blocks × 64 samples).</summary>
    private readonly float[] _v = new float[1024];

    private static readonly float[] DWindow = BuildDWindow();

    public void Reset()
    {
        Array.Clear(_v, 0, _v.Length);
    }

    /// <summary>
    /// Synthesize 32 PCM samples from one row of 32 subband samples.
    /// </summary>
    public void SynthesizeRow(ReadOnlySpan<float> subbands32, Span<float> pcm32)
    {
        if (subbands32.Length != 32 || pcm32.Length != 32)
            throw new ArgumentException("Polyphase row requires 32 subbands and 32 PCM slots.");

        // Shift V down by 64.
        Buffer.BlockCopy(_v, 0, _v, 64 * sizeof(float), (1024 - 64) * sizeof(float));

        // Matrixing: V[i] = sum_j N[i, j] * S[j] for i in 0..63.
        for (int i = 0; i < 64; i++)
        {
            float sum = 0;
            for (int j = 0; j < 32; j++) sum += Mp3Tables.N[i, j] * subbands32[j];
            _v[i] = sum;
        }

        // Build U: U[i*64+j] = V[i*128+j], U[i*64+32+j] = V[i*128+96+j], i 0..7 j 0..31.
        Span<float> u = stackalloc float[512];
        for (int i = 0; i < 8; i++)
        {
            for (int j = 0; j < 32; j++)
            {
                u[i * 64 + j] = _v[i * 128 + j];
                u[i * 64 + 32 + j] = _v[i * 128 + 96 + j];
            }
        }

        // Window: W[k] = U[k] * D[k] and sum into 32 PCM outputs.
        for (int j = 0; j < 32; j++)
        {
            float sum = 0;
            for (int i = 0; i < 16; i++)
                sum += u[j + 32 * i] * DWindow[j + 32 * i];
            pcm32[j] = sum;
        }
    }

    /// <summary>
    /// Build a windowed-sinc approximation of the ISO 11172-3 Annex B Table B.4
    /// D-window. Cutoff is π/64 (= 32 subband bandwidth), and the prototype is
    /// shaped by a Hann window. For ISO bit-exact conformance replace with the
    /// 512 reference floats from Table B.4.
    /// </summary>
    private static float[] BuildDWindow()
    {
        var d = new float[512];
        const double Center = 255.5;
        for (int n = 0; n < 512; n++)
        {
            double x = n - Center;
            double sinc = x == 0 ? 1.0 / 64.0 : Math.Sin(Math.PI * x / 64.0) / (Math.PI * x);
            double hann = 0.5 - 0.5 * Math.Cos(2.0 * Math.PI * n / 511.0);
            d[n] = (float)(sinc * hann * 32.0);
        }
        return d;
    }
}
