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

    [Fact]
    public void Leading_Trailing_Whitespace_Is_Trimmed()
    {
        Assert.True(SvgColorParser.TryParse("   red   ", out var c));
        Assert.Equal(1f, c.R);
        Assert.Equal(0f, c.G);
        Assert.Equal(0f, c.B);
    }

    [Fact]
    public void None_Is_Case_Insensitive()
    {
        Assert.False(SvgColorParser.TryParse("NONE", out _));
        Assert.False(SvgColorParser.TryParse("None", out _));
    }

    [Fact]
    public void Transparent_Is_Case_Insensitive()
    {
        Assert.True(SvgColorParser.TryParse("TRANSPARENT", out var c));
        Assert.Equal(0f, c.A);
    }

    [Fact]
    public void Empty_Hash_Returns_False()
    {
        Assert.False(SvgColorParser.TryParse("#", out _));
    }

    [Theory]
    [InlineData("#FF")]      // 2 digits
    [InlineData("#FFFFF")]   // 5 digits
    [InlineData("#FFFFFFF")] // 7 digits
    public void Hex_With_Invalid_Length_Returns_False(string s)
    {
        Assert.False(SvgColorParser.TryParse(s, out _));
    }

    [Fact]
    public void Hex_With_Invalid_Digit_Returns_False()
    {
        // Long-form hex uses byte.TryParse which rejects non-hex chars.
        Assert.False(SvgColorParser.TryParse("#GGGGGG", out _));
    }

    [Fact]
    public void Rgb_Out_Of_Range_Channels_Clamped()
    {
        Assert.True(SvgColorParser.TryParse("rgb(300, -10, 100)", out var c));
        Assert.Equal(255, (int)MathF.Round(c.R * 255));
        Assert.Equal(0, (int)MathF.Round(c.G * 255));
        Assert.Equal(100, (int)MathF.Round(c.B * 255));
    }

    [Fact]
    public void Rgb_Percentage_Above_100_Clamped()
    {
        Assert.True(SvgColorParser.TryParse("rgb(150%, 50%, 0%)", out var c));
        Assert.Equal(255, (int)MathF.Round(c.R * 255));
        Assert.Equal(128, (int)MathF.Round(c.G * 255));
    }

    [Fact]
    public void Rgb_Too_Few_Channels_Returns_False()
    {
        Assert.False(SvgColorParser.TryParse("rgb(255, 0)", out _));
    }

    [Fact]
    public void Rgba_Without_Alpha_Returns_False()
    {
        Assert.False(SvgColorParser.TryParse("rgba(255, 0, 0)", out _));
    }

    [Fact]
    public void Rgb_With_Slash_Separator_Treats_Last_As_Alpha()
    {
        // CSS-style space-and-slash separators.
        Assert.True(SvgColorParser.TryParse("rgb(255 0 0 / 0.5)", out var c));
        Assert.Equal(255, (int)MathF.Round(c.R * 255));
        Assert.InRange((int)MathF.Round(c.A * 255), 127, 129);
    }

    [Fact]
    public void Rgba_With_Percentage_Alpha()
    {
        Assert.True(SvgColorParser.TryParse("rgba(255, 0, 0, 50%)", out var c));
        Assert.InRange((int)MathF.Round(c.A * 255), 127, 129);
    }

    [Fact]
    public void Rgba_With_Negative_Alpha_Clamped_To_Zero()
    {
        Assert.True(SvgColorParser.TryParse("rgba(255, 0, 0, -0.5)", out var c));
        Assert.Equal(0f, c.A);
    }

    [Fact]
    public void Rgba_With_Alpha_Above_One_Clamped_To_One()
    {
        Assert.True(SvgColorParser.TryParse("rgba(255, 0, 0, 2.0)", out var c));
        Assert.Equal(1f, c.A);
    }

    [Fact]
    public void Rgb_With_Bad_Channel_Returns_False()
    {
        Assert.False(SvgColorParser.TryParse("rgb(foo, 0, 0)", out _));
    }
}
