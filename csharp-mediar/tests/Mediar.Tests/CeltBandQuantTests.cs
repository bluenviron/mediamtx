using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Mediar.Codecs.Opus.Encoder;
using Mediar.Codecs.Opus.Encoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Round-trip tests for <see cref="CeltBandQuant.QuantBandSimple"/>
/// against the decoder's <see cref="CeltShape.AlgUnquant"/>. Covers the
/// Phase B2 v1 path (mono, no TF / no Haar / no split / no folding) —
/// the full <c>QuantBand</c> / <c>QuantBandStereo</c> / partition split
/// path will be covered by additional theory rows when B2.1 lands.
/// </summary>
public sealed class CeltBandQuantTests
{
    [Theory]
    [InlineData(8, 4, 0)]
    [InlineData(16, 6, 0)]
    [InlineData(16, 6, 2)]
    [InlineData(10, 4, 1)]
    public void QuantBandSimple_RoundTrips_Through_AlgUnquant(int n, int k, int spread)
    {
        var x = new float[n];
        float norm = 0f;
        for (int i = 0; i < n; i++)
        {
            x[i] = MathF.Sin(0.21f * i + 0.5f) + 0.3f * MathF.Cos(0.07f * i);
            norm += x[i] * x[i];
        }
        norm = MathF.Sqrt(norm);
        for (int i = 0; i < n; i++) x[i] /= norm;

        var encShape = (float[])x.Clone();
        var buf = new byte[64];
        var enc = new OpusRangeEncoder(buf);
        uint maskEnc = CeltBandQuant.QuantBandSimple(ref enc, encShape, n, k, spread, blocks: 1, gain: 1f, complexity: 8);
        enc.Finish();

        var dec = new OpusRangeDecoder(buf.AsSpan(0, enc.ByteCount));
        var decShape = new float[n];
        uint maskDec = CeltShape.AlgUnquant(decShape, n, k, spread, B: 1, ref dec, gain: 1f);

        Assert.Equal(maskEnc, maskDec);
        for (int i = 0; i < n; i++)
            Assert.True(MathF.Abs(encShape[i] - decShape[i]) < 1e-5f,
                $"shape[{i}] enc={encShape[i]} dec={decShape[i]}");
    }

    [Fact]
    public void QuantBandSimple_N1_RoundTrips_Sign()
    {
        foreach (float v in new[] { 1f, -1f })
        {
            var x = new float[] { v };
            var buf = new byte[8];
            var enc = new OpusRangeEncoder(buf);
            uint mask = CeltBandQuant.QuantBandSimple(ref enc, x, N: 1, pulses: 1, spread: 0, blocks: 1, gain: 1f, complexity: 0);
            enc.Finish();
            Assert.Equal(1u, mask);
            Assert.Equal(v, x[0]);
        }
    }
}
