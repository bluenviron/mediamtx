using Mediar.Codecs.Alac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AlacPredictorTests
{
    [Fact]
    public void Unpc_NumCoeffsZero_CopiesResidualsVerbatim()
    {
        // numactive==0 means "no predictor" — residuals ARE the samples.
        int[] residuals = { 10, 5, -3, 7, 0 };
        var output = new int[5];
        var coeffs = Array.Empty<short>();

        AlacPredictor.Unpc(residuals, output, residuals.Length, coeffs,
            numCoeffs: 0, chanBits: 16, denShift: 9);

        Assert.Equal(residuals, output);
    }

    [Fact]
    public void Unpc_Active31_CumulativeSumSignExtendsToChanBits()
    {
        // numCoeffs==31 is Apple's identity-sum marker. chanBits=8 means each
        // partial sum is sign-extended into the 8-bit signed range.
        int[] residuals = { 100, 100, 100 };
        var output = new int[3];

        AlacPredictor.Unpc(residuals, output, residuals.Length,
            Array.Empty<short>(), numCoeffs: 31, chanBits: 8, denShift: 0);

        Assert.Equal(100, output[0]);
        // 100 + 100 = 200, sign-extended as 8-bit: 200 - 256 = -56
        Assert.Equal(-56, output[1]);
        // -56 + 100 = 44, fits in 8-bit
        Assert.Equal(44, output[2]);
    }

    [Fact]
    public void Unpc_NumCoeffsOne_StableInputStaysStable()
    {
        // With numCoeffs=1, coef=256, denShift=8: warm-up copies samples;
        // when the input is constant the predictor predicts the same value
        // and the residual is 0, producing a stable output.
        const int a = 1234;
        int[] residuals = { a, 0, 0, 0, 0 };
        var output = new int[5];
        var coeffs = new short[] { 256 };

        AlacPredictor.Unpc(residuals, output, residuals.Length, coeffs,
            numCoeffs: 1, chanBits: 16, denShift: 8);

        var expected = new[] { a, a, a, a, a };
        Assert.Equal(expected, output);
    }

    [Fact]
    public void Unpc_Active31_ActsAsIdentitySum()
    {
        // numCoeffs==31 is Apple's identity-sum marker used for the first pass
        // when mode != 0: cumulative sum of residuals, no FIR prediction.
        int[] residuals = { 1, 2, 3, 4, 5 };
        var output = new int[5];

        AlacPredictor.Unpc(residuals, output, residuals.Length,
            Array.Empty<short>(), numCoeffs: 31, chanBits: 16, denShift: 0);

        var expected = new[] { 1, 3, 6, 10, 15 };
        Assert.Equal(expected, output);
    }
}
