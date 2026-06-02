using Mediar.Codecs.Alac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AlacMatrixTests
{
    [Fact]
    public void Unmix_MixResZero_IsPassthrough()
    {
        int[] u = { 100, -50, 32_000, -32_000 };
        int[] v = { 200, 1, -1, 12_345 };
        int[] l = new int[4];
        int[] r = new int[4];

        AlacMatrix.Unmix(u, v, l, r, 4, mixBits: 8, mixRes: 0);

        Assert.Equal(u, l);
        Assert.Equal(v, r);
    }

    [Fact]
    public void Unmix_MixResNonZero_InvertsEncoderRotation()
    {
        // Build a known L/R pair, run the encoder rotation, then unmix and
        // check we recover (L, R). Encoder (Apple ref):
        //   u = ((mixRes * L) >> mixBits) + R + ((1 << (mixBits-1)) * 0)
        // Wait — Apple's encoder mix16 is:
        //   u = (mixRes * (L - R) + (1 << (mixBits-1))) >> mixBits + R  ... no.
        // The decoder formula is l = u + v - ((v * mixres) >> mixbits); r = l - v.
        // Solving: v = L - R; u = L - v + ((v * mixres) >> mixbits)
        //                       = R + ((v * mixres) >> mixbits)
        // That is the matching encoder.
        const int mixBits = 8;
        const int mixRes = 32;

        int[] origL = { 0, 100, -200, 5_000, -5_000, 1, 0, -1 };
        int[] origR = { 0, 50, 100, -3_000, 3_000, -1, 1, 0 };

        int n = origL.Length;
        var u = new int[n];
        var v = new int[n];
        for (int i = 0; i < n; i++)
        {
            v[i] = origL[i] - origR[i];
            u[i] = origR[i] + ((v[i] * mixRes) >> mixBits);
        }

        var l = new int[n];
        var r = new int[n];
        AlacMatrix.Unmix(u, v, l, r, n, mixBits, mixRes);

        Assert.Equal(origL, l);
        Assert.Equal(origR, r);
    }
}
