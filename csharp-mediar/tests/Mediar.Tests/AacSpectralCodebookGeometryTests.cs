using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSpectralCodebookGeometryTests
{
    private static readonly int[] Cb1FirstSymbol = new[] { -1, -1, -1, -1 };
    private static readonly int[] Cb1MidSymbol = new[] { 0, 0, 0, 0 };
    private static readonly int[] Cb1LastSymbol = new[] { 1, 1, 1, 1 };
    private static readonly int[] Cb3FirstSymbol = new[] { 0, 0, 0, 0 };
    private static readonly int[] Cb3LastSymbol = new[] { 2, 2, 2, 2 };
    private static readonly int[] Cb5MidSymbol = new[] { 0, 0 };
    private static readonly int[] Cb5FirstSymbol = new[] { -4, -4 };
    private static readonly int[] Cb5LastSymbol = new[] { 4, 4 };
    private static readonly int[] Cb7FirstSymbol = new[] { 0, 0 };
    private static readonly int[] Cb7Sym15 = new[] { 1, 7 };
    private static readonly int[] Cb7LastSymbol = new[] { 7, 7 };
    private static readonly int[] Cb9FirstSymbol = new[] { 0, 0 };
    private static readonly int[] Cb9Mid = new[] { 5, 7 };
    private static readonly int[] Cb9LastSymbol = new[] { 12, 12 };
    private static readonly int[] Cb11Zero = new[] { 0, 0 };
    private static readonly int[] Cb11XEscape = new[] { 16, 0 };
    private static readonly int[] Cb11YEscape = new[] { 0, 16 };
    private static readonly int[] Cb11BothEscape = new[] { 16, 16 };

    [Theory]
    [InlineData(0)]
    [InlineData(12)]
    [InlineData(13)]
    [InlineData(-1)]
    public void Get_OutOfRange_ReturnsNull(int codebook)
    {
        Assert.Null(AacSpectralCodebookGeometry.Get(codebook));
    }

    [Theory]
    [InlineData(1,  4, true,  1,  false, 81)]
    [InlineData(2,  4, true,  1,  false, 81)]
    [InlineData(3,  4, false, 2,  false, 81)]
    [InlineData(4,  4, false, 2,  false, 81)]
    [InlineData(5,  2, true,  4,  false, 81)]
    [InlineData(6,  2, true,  4,  false, 81)]
    [InlineData(7,  2, false, 7,  false, 64)]
    [InlineData(8,  2, false, 7,  false, 64)]
    [InlineData(9,  2, false, 12, false, 169)]
    [InlineData(10, 2, false, 12, false, 169)]
    [InlineData(11, 2, false, 16, true,  289)]
    public void Get_ReturnsCorrectGeometry(int cb, int dim, bool isSigned, int lav, bool esc, int count)
    {
        var g = AacSpectralCodebookGeometry.Get(cb);
        Assert.NotNull(g);
        Assert.Equal(cb, g!.CodebookNumber);
        Assert.Equal(dim, g.Dimension);
        Assert.Equal(isSigned, g.IsSigned);
        Assert.Equal(lav, g.LargestAbsoluteValue);
        Assert.Equal(esc, g.HasEscape);
        Assert.Equal(count, g.SymbolCount);
    }

    [Fact]
    public void Decompose_Cb1_FirstSymbol_AllMinusOne()
    {
        var g = AacSpectralCodebookGeometry.Get(1)!;
        Span<int> comp = stackalloc int[4];
        g.Decompose(0, comp);
        Assert.Equal(Cb1FirstSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb1_MiddleSymbol_AllZero()
    {
        var g = AacSpectralCodebookGeometry.Get(1)!;
        Span<int> comp = stackalloc int[4];
        g.Decompose(40, comp);
        Assert.Equal(Cb1MidSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb1_LastSymbol_AllPlusOne()
    {
        var g = AacSpectralCodebookGeometry.Get(1)!;
        Span<int> comp = stackalloc int[4];
        g.Decompose(80, comp);
        Assert.Equal(Cb1LastSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb3_Symbol0_AllZero()
    {
        var g = AacSpectralCodebookGeometry.Get(3)!;
        Span<int> comp = stackalloc int[4];
        g.Decompose(0, comp);
        Assert.Equal(Cb3FirstSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb3_LastSymbol_AllMaxAbsolute()
    {
        var g = AacSpectralCodebookGeometry.Get(3)!;
        Span<int> comp = stackalloc int[4];
        g.Decompose(80, comp);
        Assert.Equal(Cb3LastSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb5_Symbol_Centred()
    {
        var g = AacSpectralCodebookGeometry.Get(5)!;
        Span<int> comp = stackalloc int[2];
        g.Decompose(40, comp);
        Assert.Equal(Cb5MidSymbol, comp.ToArray());

        g.Decompose(0, comp);
        Assert.Equal(Cb5FirstSymbol, comp.ToArray());

        g.Decompose(80, comp);
        Assert.Equal(Cb5LastSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb7_Symbol_Unsigned()
    {
        var g = AacSpectralCodebookGeometry.Get(7)!;
        Span<int> comp = stackalloc int[2];
        g.Decompose(0, comp);
        Assert.Equal(Cb7FirstSymbol, comp.ToArray());

        g.Decompose(15, comp);
        Assert.Equal(Cb7Sym15, comp.ToArray());

        g.Decompose(63, comp);
        Assert.Equal(Cb7LastSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb9_Symbol_Unsigned()
    {
        var g = AacSpectralCodebookGeometry.Get(9)!;
        Span<int> comp = stackalloc int[2];
        g.Decompose(0, comp);
        Assert.Equal(Cb9FirstSymbol, comp.ToArray());

        g.Decompose(13 * 5 + 7, comp);
        Assert.Equal(Cb9Mid, comp.ToArray());

        g.Decompose(168, comp);
        Assert.Equal(Cb9LastSymbol, comp.ToArray());
    }

    [Fact]
    public void Decompose_Cb11_Symbol_Unsigned_WithEscapeMarker()
    {
        var g = AacSpectralCodebookGeometry.Get(11)!;
        Assert.True(g.HasEscape);
        Span<int> comp = stackalloc int[2];

        g.Decompose(0, comp);
        Assert.Equal(Cb11Zero, comp.ToArray());

        g.Decompose(272, comp);
        Assert.Equal(Cb11XEscape, comp.ToArray());

        g.Decompose(16, comp);
        Assert.Equal(Cb11YEscape, comp.ToArray());

        g.Decompose(288, comp);
        Assert.Equal(Cb11BothEscape, comp.ToArray());
    }

    [Fact]
    public void Decompose_NegativeSymbol_Throws()
    {
        var g = AacSpectralCodebookGeometry.Get(1)!;
        var buf = new int[4];
        Assert.Throws<ArgumentOutOfRangeException>(() => g.Decompose(-1, buf));
    }

    [Fact]
    public void Decompose_OverflowSymbol_Throws()
    {
        var g = AacSpectralCodebookGeometry.Get(7)!;
        var buf = new int[2];
        Assert.Throws<ArgumentOutOfRangeException>(() => g.Decompose(64, buf));
    }

    [Fact]
    public void Decompose_BufferTooSmall_Throws()
    {
        var g = AacSpectralCodebookGeometry.Get(1)!;
        var buf = new int[3];
        Assert.Throws<ArgumentOutOfRangeException>(() => g.Decompose(0, buf));
    }

    [Fact]
    public void Decompose_AllUnsignedCodebooks_AllSymbolsWithinLav()
    {
        Span<int> comp = stackalloc int[4];
        foreach (var cb in new[] { 3, 4, 7, 8, 9, 10, 11 })
        {
            var g = AacSpectralCodebookGeometry.Get(cb)!;
            for (int s = 0; s < g.SymbolCount; s++)
            {
                g.Decompose(s, comp);
                for (int j = 0; j < g.Dimension; j++)
                {
                    Assert.InRange(comp[j], 0, g.LargestAbsoluteValue);
                }
            }
        }
    }

    [Fact]
    public void Decompose_AllSignedCodebooks_AllSymbolsWithinLav()
    {
        Span<int> comp = stackalloc int[4];
        foreach (var cb in new[] { 1, 2, 5, 6 })
        {
            var g = AacSpectralCodebookGeometry.Get(cb)!;
            for (int s = 0; s < g.SymbolCount; s++)
            {
                g.Decompose(s, comp);
                for (int j = 0; j < g.Dimension; j++)
                {
                    Assert.InRange(comp[j], -g.LargestAbsoluteValue, g.LargestAbsoluteValue);
                }
            }
        }
    }
}
