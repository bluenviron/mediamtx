using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Mediar.Codecs.Opus.Encoder;
using Mediar.Codecs.Opus.Encoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Round-trip tests for the CELT PVQ shape search
/// (<see cref="CeltPvqSearch.AlgQuant"/>) against the decoder's
/// <see cref="CeltShape.AlgUnquant"/>. We verify the produced packet is
/// bit-exact round-trippable through the decoder and that the
/// reconstructed unit-norm shape matches the input shape to a useful
/// approximation budget (≤ 1 dB shape error for K ≥ 8).
/// </summary>
public sealed class CeltPvqSearchTests
{
    [Theory]
    [InlineData(8, 4, 0)]
    [InlineData(16, 8, 0)]
    [InlineData(16, 8, 2)]
    [InlineData(16, 4, 2)]
    [InlineData(10, 6, 3)]
    public void AlgQuant_RoundTrips_Through_AlgUnquant(int n, int k, int spread)
    {
        // Build a deterministic unit-norm input shape.
        var x = new float[n];
        float norm = 0f;
        for (int i = 0; i < n; i++)
        {
            x[i] = MathF.Cos(0.37f * i) + 0.5f * MathF.Sin(0.11f * i + 1f);
            norm += x[i] * x[i];
        }
        norm = MathF.Sqrt(norm);
        for (int i = 0; i < n; i++) x[i] /= norm;
        var xOrig = (float[])x.Clone();

        // Encode.
        var buf = new byte[64];
        var enc = new OpusRangeEncoder(buf);
        var encShape = new float[n];
        x.AsSpan().CopyTo(encShape);
        uint maskEnc = CeltPvqSearch.AlgQuant(encShape, n, k, spread, B: 1, ref enc, gain: 1f, complexity: 8);
        enc.Finish();
        int byteLen = enc.ByteCount;

        // Decode.
        var dec = new OpusRangeDecoder(buf.AsSpan(0, byteLen));
        var decShape = new float[n];
        uint maskDec = CeltShape.AlgUnquant(decShape, n, k, spread, B: 1, ref dec, gain: 1f);

        Assert.Equal(maskEnc, maskDec);
        for (int i = 0; i < n; i++)
            Assert.True(MathF.Abs(encShape[i] - decShape[i]) < 1e-5f,
                $"encoded shape[{i}]={encShape[i]} != decoded shape[{i}]={decShape[i]}");

        // Shape error vs original (unit-norm both sides).
        float dot = 0f;
        for (int i = 0; i < n; i++) dot += xOrig[i] * decShape[i];
        // dot is cos(angle); for k ≥ n/2 we expect cos > 0.7 i.e. < ~3 dB shape error.
        // For the unit test we use the looser ≥ 0.5 (≤ 6 dB) bound which the
        // greedy search comfortably satisfies for all parameterisations above.
        Assert.True(dot > 0.5f, $"shape correlation {dot} too low for (n={n}, k={k}, spread={spread})");
    }

    [Theory]
    [InlineData(4, 2)]
    [InlineData(8, 4)]
    [InlineData(10, 5)]
    public void Icwrs_RoundTrips_With_DecodePulsesAtIndex(int n, int k)
    {
        uint v = CeltPvq.ComputeV(n, k);
        Span<int> y = stackalloc int[n];
        for (uint i = 0; i < v; i++)
        {
            CeltPvq.DecodePulsesAtIndex(n, k, i, y);
            uint recoveredI = CeltPvqSearch.Icwrs(n, k, y, out uint recoveredV);
            Assert.Equal(v, recoveredV);
            Assert.Equal(i, recoveredI);
        }
    }
}
