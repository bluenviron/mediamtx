using Mediar.Acceleration;
using Xunit;

namespace Mediar.Tests.Jpeg;

public sealed class JpegIdctSimdTests
{
    [Fact]
    public void Scalar_Zeros_ProduceLevelShifted128()
    {
        var coeffs = new short[64];
        var output = new byte[64];
        ScalarIdct8x8.Instance.Idct8x8(coeffs, output, 8);
        for (int i = 0; i < 64; i++) Assert.Equal((byte)128, output[i]);
    }

    [Fact]
    public void Dispatcher_HasIdct()
    {
        Assert.NotNull(Kernels.Idct8x8);
    }
}
