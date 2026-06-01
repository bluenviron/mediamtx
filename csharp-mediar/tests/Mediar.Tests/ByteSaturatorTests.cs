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
    public void Scalar_EmptySource_NoOp()
    {
        byte[] dst = new byte[3] { 1, 2, 3 };
        ScalarByteSaturator.Instance.Saturate(ReadOnlySpan<int>.Empty, dst);
        // Tail untouched.
        Assert.Equal(1, dst[0]);
        Assert.Equal(2, dst[1]);
        Assert.Equal(3, dst[2]);
    }

    [Fact]
    public void Scalar_DestinationLargerThanSource_LeavesTail()
    {
        int[] src = { 100, 200 };
        byte[] dst = new byte[] { 7, 7, 9, 9 };
        ScalarByteSaturator.Instance.Saturate(src, dst);
        Assert.Equal(100, dst[0]);
        Assert.Equal(200, dst[1]);
        Assert.Equal(9, dst[2]);
        Assert.Equal(9, dst[3]);
    }

    [Fact]
    public void Scalar_Instance_IsSingleton()
    {
        Assert.Same(ScalarByteSaturator.Instance, ScalarByteSaturator.Instance);
        Assert.Equal(AccelerationTier.Scalar, ScalarByteSaturator.Instance.IsaTier);
    }

    [Fact]
    public void Sse2_Instance_HasCorrectTier()
    {
        Assert.Same(Sse2ByteSaturator.Instance, Sse2ByteSaturator.Instance);
        Assert.Equal(AccelerationTier.Sse2, Sse2ByteSaturator.Instance.IsaTier);
    }

    [Fact]
    public void Sse2_ThrowsWhenDestinationTooShort()
    {
        if (!Sse2ByteSaturator.IsSupported) return;
        Assert.Throws<ArgumentException>(() =>
            Sse2ByteSaturator.Instance.Saturate(new int[16], new byte[15]));
    }

    [Fact]
    public void Sse2_EmptySource_NoOp()
    {
        if (!Sse2ByteSaturator.IsSupported) return;
        byte[] dst = new byte[2] { 9, 9 };
        Sse2ByteSaturator.Instance.Saturate(ReadOnlySpan<int>.Empty, dst);
        Assert.Equal(9, dst[0]);
        Assert.Equal(9, dst[1]);
    }

    [Fact]
    public void Avx2_Instance_HasCorrectTier()
    {
        Assert.Same(Avx2ByteSaturator.Instance, Avx2ByteSaturator.Instance);
        Assert.Equal(AccelerationTier.Avx2, Avx2ByteSaturator.Instance.IsaTier);
    }

    [Fact]
    public void Avx2_ThrowsWhenDestinationTooShort()
    {
        if (!Avx2ByteSaturator.IsSupported) return;
        Assert.Throws<ArgumentException>(() =>
            Avx2ByteSaturator.Instance.Saturate(new int[32], new byte[31]));
    }

    [Fact]
    public void Avx2_EmptySource_NoOp()
    {
        if (!Avx2ByteSaturator.IsSupported) return;
        byte[] dst = new byte[2] { 9, 9 };
        Avx2ByteSaturator.Instance.Saturate(ReadOnlySpan<int>.Empty, dst);
        Assert.Equal(9, dst[0]);
        Assert.Equal(9, dst[1]);
    }

    [Fact]
    public void Neon_Instance_HasCorrectTier()
    {
        Assert.Same(NeonByteSaturator.Instance, NeonByteSaturator.Instance);
        Assert.Equal(AccelerationTier.Neon, NeonByteSaturator.Instance.IsaTier);
    }

    [Fact]
    public void Neon_ThrowsWhenDestinationTooShort()
    {
        if (!NeonByteSaturator.IsSupported) return;
        Assert.Throws<ArgumentException>(() =>
            NeonByteSaturator.Instance.Saturate(new int[16], new byte[15]));
    }

    [Fact]
    public void Neon_EmptySource_NoOp()
    {
        if (!NeonByteSaturator.IsSupported) return;
        byte[] dst = new byte[2] { 9, 9 };
        NeonByteSaturator.Instance.Saturate(ReadOnlySpan<int>.Empty, dst);
        Assert.Equal(9, dst[0]);
        Assert.Equal(9, dst[1]);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(7)]
    [InlineData(15)]
    public void Scalar_AndAllSimd_HandleShortLengths(int length)
    {
        int[] src = MakeDeterministicInputs(length);
        byte[] sca = new byte[length];
        ScalarByteSaturator.Instance.Saturate(src, sca);

        if (Sse2ByteSaturator.IsSupported)
        {
            byte[] sse = new byte[length];
            Sse2ByteSaturator.Instance.Saturate(src, sse);
            Assert.Equal(sca, sse);
        }
        if (Avx2ByteSaturator.IsSupported)
        {
            byte[] avx = new byte[length];
            Avx2ByteSaturator.Instance.Saturate(src, avx);
            Assert.Equal(sca, avx);
        }
        if (NeonByteSaturator.IsSupported)
        {
            byte[] neon = new byte[length];
            NeonByteSaturator.Instance.Saturate(src, neon);
            Assert.Equal(sca, neon);
        }
    }

    [Fact]
    public void Saturate_LargeBuffer_AllTiersAgree()
    {
        int[] src = MakeDeterministicInputs(4096);
        byte[] sca = new byte[src.Length];
        ScalarByteSaturator.Instance.Saturate(src, sca);

        if (Sse2ByteSaturator.IsSupported)
        {
            byte[] sse = new byte[src.Length];
            Sse2ByteSaturator.Instance.Saturate(src, sse);
            Assert.Equal(sca, sse);
        }
        if (Avx2ByteSaturator.IsSupported)
        {
            byte[] avx = new byte[src.Length];
            Avx2ByteSaturator.Instance.Saturate(src, avx);
            Assert.Equal(sca, avx);
        }
        if (NeonByteSaturator.IsSupported)
        {
            byte[] neon = new byte[src.Length];
            NeonByteSaturator.Instance.Saturate(src, neon);
            Assert.Equal(sca, neon);
        }
    }

    [Fact]
    public void Saturate_AllZeros_AllZeros()
    {
        int[] src = new int[200];
        byte[] dst = new byte[200];
        Kernels.ByteSaturator.Saturate(src, dst);
        foreach (var b in dst) Assert.Equal(0, b);
    }

    [Fact]
    public void Saturate_AllInRange_PassesThrough()
    {
        int[] src = Enumerable.Range(0, 256).ToArray();
        byte[] dst = new byte[256];
        Kernels.ByteSaturator.Saturate(src, dst);
        for (int i = 0; i < 256; i++) Assert.Equal((byte)i, dst[i]);
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
