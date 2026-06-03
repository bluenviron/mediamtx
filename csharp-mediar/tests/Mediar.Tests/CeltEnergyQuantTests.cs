using Mediar.Codecs.Opus.Encoder;
using Mediar.Codecs.Opus.Encoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Smoke tests for <see cref="CeltEnergyQuant"/>. A full encode → decode
/// round-trip is provided by <c>CeltEncoderRoundTripTests</c> (which
/// drives the energy quant via the top-level <c>OpusDecoder.Decode</c>
/// path because the decoder's <c>DecodeCoarseEnergy</c> /
/// <c>UnquantFineEnergy</c> helpers are private to <c>CeltDecoder</c>).
/// </summary>
public sealed class CeltEnergyQuantTests
{
    [Theory]
    [InlineData(0, false)]   // LM=0 (2.5 ms), inter-frame predictor
    [InlineData(0, true)]    // LM=0, intra
    [InlineData(2, false)]   // LM=2 (10 ms)
    [InlineData(3, false)]   // LM=3 (20 ms)
    public void QuantCoarseEnergy_Writes_Without_Error(int lm, bool intra)
    {
        const int channels = 1;
        const int nbBands = 21;
        var logE = new float[channels * nbBands];
        var oldLogE = new float[channels * nbBands];
        for (int i = 0; i < nbBands; i++)
        {
            logE[i] = -12f + 0.5f * i;
            oldLogE[i] = -12f + 0.45f * i;
        }
        Array.Fill(oldLogE, -28f); // also try fresh-start path

        var buf = new byte[256];
        var enc = new OpusRangeEncoder(buf);
        int budgetBitsx8 = 2000;
        CeltEnergyQuant.QuantCoarseEnergy(ref enc, logE, oldLogE, budgetBitsx8, intra, lm, 0, nbBands, channels, nbBands);
        CeltEnergyQuant.QuantFineEnergy(ref enc, logE, oldLogE, new int[nbBands], 0, nbBands, channels, nbBands);
        enc.Finish();

        Assert.False(enc.HasError);
        Assert.True(enc.ByteCount > 0, "encoded packet must be non-empty");
    }
}
