using Mediar.Acceleration;
using Mediar.Acceleration.Arm;
using Mediar.Acceleration.X86;
using Xunit;

namespace Mediar.Tests;

public sealed class ByteSaturatorTests
{
    [Fact]
    public void Scalar_ClampsToByteRange()
    {
        int[] src = [-1000, -1, 0, 1, 127, 128, 254, 255, 256, 1000, int.MinValue, int.MaxValue];
        byte[] expected = [0, 0, 0, 1, 127, 128, 254, 255, 255, 255, 0, 255];
        byte[] dst = new byte[src.Length];
        ScalarByteSaturator.Instance.Saturate(src, dst);
        Assert.Equal(expected, dst);
    }

    [Fact]
    public void Scalar_ThrowsWhenDestinationTooShort()
    {
        int[] src = new int[16];
        byte[] dst = new byte[15];
        Assert.Throws<ArgumentException>(() => ScalarByteSaturator.Instance.Saturate(src, dst));
    }

    [Theory]
    [InlineData(16)]
    [InlineData(31)]
    [InlineData(32)]
    [InlineData(33)]
    [InlineData(64)]
    [InlineData(100)]
    [InlineData(256)]
    [InlineData(257)]
    public void Sse2_MatchesScalarReference(int length)
    {
        if (!Sse2ByteSaturator.IsSupported) return;
        int[] src = MakeDeterministicInputs(length);
        byte[] sca = new byte[length];
        byte[] sse = new byte[length];
        ScalarByteSaturator.Instance.Saturate(src, sca);
        Sse2ByteSaturator.Instance.Saturate(src, sse);
        Assert.Equal(sca, sse);
    }

    [Theory]
    [InlineData(32)]
    [InlineData(33)]
    [InlineData(64)]
    [InlineData(95)]
    [InlineData(96)]
    [InlineData(127)]
    [InlineData(256)]
    [InlineData(513)]
    public void Avx2_MatchesScalarReference(int length)
    {
        if (!Avx2ByteSaturator.IsSupported) return;
        int[] src = MakeDeterministicInputs(length);
        byte[] sca = new byte[length];
        byte[] avx = new byte[length];
        ScalarByteSaturator.Instance.Saturate(src, sca);
        Avx2ByteSaturator.Instance.Saturate(src, avx);
        Assert.Equal(sca, avx);
    }

    [Theory]
    [InlineData(16)]
    [InlineData(32)]
    [InlineData(33)]
    [InlineData(100)]
    [InlineData(256)]
    public void Neon_MatchesScalarReference(int length)
    {
        if (!NeonByteSaturator.IsSupported) return;
        int[] src = MakeDeterministicInputs(length);
        byte[] sca = new byte[length];
        byte[] neon = new byte[length];
        ScalarByteSaturator.Instance.Saturate(src, sca);
        NeonByteSaturator.Instance.Saturate(src, neon);
        Assert.Equal(sca, neon);
    }

    [Fact]
    public void Dispatcher_ResolvesBestAvailableKernel()
    {
        IByteSaturator k = Kernels.ByteSaturator;
        Assert.NotNull(k);

        AccelerationTier expected = AccelerationTier.Scalar;
        if (Avx2ByteSaturator.IsSupported) expected = AccelerationTier.Avx2;
        else if (Sse2ByteSaturator.IsSupported) expected = AccelerationTier.Sse2;
        else if (NeonByteSaturator.IsSupported) expected = AccelerationTier.Neon;
        Assert.Equal(expected, k.IsaTier);
    }

    [Fact]
    public void ActiveKernel_RoundtripsRepresentativeData()
    {
        int[] src = MakeDeterministicInputs(1024);
        byte[] expected = new byte[src.Length];
        ScalarByteSaturator.Instance.Saturate(src, expected);

        byte[] actual = new byte[src.Length];
        Kernels.ByteSaturator.Saturate(src, actual);
        Assert.Equal(expected, actual);
    }

    private static int[] MakeDeterministicInputs(int length)
    {
        int[] src = new int[length];
        var rng = new Random(0xC0FFEE);
        for (int i = 0; i < length; i++)
        {
            int pick = rng.Next(8);
            src[i] = pick switch
            {
                0 => int.MinValue,
                1 => int.MaxValue,
                2 => -1,
                3 => 0,
                4 => 255,
                5 => 256,
                6 => rng.Next(-512, 768),
                _ => rng.Next(-10_000_000, 10_000_000),
            };
        }
        return src;
    }
}
