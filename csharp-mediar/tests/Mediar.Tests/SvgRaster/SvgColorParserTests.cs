using Mediar.Codecs.SvgRaster;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgColorParserTests
{
    [Theory]
    [InlineData("red", 255, 0, 0, 255)]
    [InlineData("black", 0, 0, 0, 255)]
    [InlineData("white", 255, 255, 255, 255)]
    [InlineData("aliceblue", 0xF0, 0xF8, 0xFF, 255)]
    [InlineData("CornflowerBlue", 0x64, 0x95, 0xED, 255)]
    public void Named_Colors(string s, int r, int g, int b, int a)
    {
        Assert.True(SvgColorParser.TryParse(s, out var c));
        Assert.Equal(r, (int)MathF.Round(c.R * 255));
        Assert.Equal(g, (int)MathF.Round(c.G * 255));
        Assert.Equal(b, (int)MathF.Round(c.B * 255));
        Assert.Equal(a, (int)MathF.Round(c.A * 255));
    }

    [Theory]
    [InlineData("#000", 0, 0, 0, 255)]
    [InlineData("#FFF", 255, 255, 255, 255)]
    [InlineData("#F00", 255, 0, 0, 255)]
    [InlineData("#abc", 0xAA, 0xBB, 0xCC, 255)]
    [InlineData("#FFFF", 255, 255, 255, 255)]
    [InlineData("#F008", 255, 0, 0, 0x88)]
    public void Short_Hex(string s, int r, int g, int b, int a)
    {
        Assert.True(SvgColorParser.TryParse(s, out var c));
        Assert.Equal(r, (int)MathF.Round(c.R * 255));
        Assert.Equal(g, (int)MathF.Round(c.G * 255));
        Assert.Equal(b, (int)MathF.Round(c.B * 255));
        Assert.Equal(a, (int)MathF.Round(c.A * 255));
    }

    [Theory]
    [InlineData("#FF8000", 255, 128, 0, 255)]
    [InlineData("#12345678", 0x12, 0x34, 0x56, 0x78)]
    [InlineData("#abcdef", 0xAB, 0xCD, 0xEF, 255)]
    public void Long_Hex(string s, int r, int g, int b, int a)
    {
        Assert.True(SvgColorParser.TryParse(s, out var c));
        Assert.Equal(r, (int)MathF.Round(c.R * 255));
        Assert.Equal(g, (int)MathF.Round(c.G * 255));
        Assert.Equal(b, (int)MathF.Round(c.B * 255));
        Assert.Equal(a, (int)MathF.Round(c.A * 255));
    }

    [Theory]
    [InlineData("rgb(255, 0, 0)", 255, 0, 0, 255)]
    [InlineData("rgb(50%, 0%, 100%)", 128, 0, 255, 255)]
    [InlineData("rgba(255, 0, 0, 0.5)", 255, 0, 0, 128)]
    public void Rgb_Function(string s, int r, int g, int b, int a)
    {
        Assert.True(SvgColorParser.TryParse(s, out var c));
        Assert.Equal(r, (int)MathF.Round(c.R * 255));
        Assert.Equal(g, (int)MathF.Round(c.G * 255));
        Assert.Equal(b, (int)MathF.Round(c.B * 255));
        Assert.InRange((int)MathF.Round(c.A * 255), a - 1, a + 1);
    }

    [Fact]
    public void Transparent_Keyword()
    {
        Assert.True(SvgColorParser.TryParse("transparent", out var c));
        Assert.Equal(0f, c.A);
    }

    [Fact]
    public void None_Returns_False()
    {
        Assert.False(SvgColorParser.TryParse("none", out _));
    }

    [Fact]
    public void Empty_Returns_False()
    {
        Assert.False(SvgColorParser.TryParse("", out _));
    }

    [Fact]
    public void Unknown_Returns_False()
    {
        Assert.False(SvgColorParser.TryParse("notarealcolor", out _));
    }

    [Fact]
    public void CurrentColor_Returns_Sentinel()
    {
        Assert.True(SvgColorParser.TryParse("currentColor", out var c));
        Assert.Equal(SvgColorParser.CurrentColorSentinel, c);
    }
}
