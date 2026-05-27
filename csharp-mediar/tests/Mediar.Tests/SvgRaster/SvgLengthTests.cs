using Mediar.Codecs.SvgRaster;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgLengthTests
{
    [Fact]
    public void Plain_Numbers_Default_To_Px()
    {
        Assert.Equal(42f, SvgLength.Parse("42"));
        Assert.Equal(3.14f, SvgLength.Parse("3.14"));
    }

    [Theory]
    [InlineData("100px", 100f)]
    [InlineData("100pt", 100f * 96f / 72f)]
    [InlineData("100pc", 1600f)]
    [InlineData("1in", 96f)]
    [InlineData("2.54cm", 96f)]
    [InlineData("25.4mm", 96f)]
    [InlineData("1em", 16f)]
    [InlineData("1ex", 8f)]
    public void Unit_Suffixes(string text, float expected)
    {
        Assert.Equal(expected, SvgLength.Parse(text), 3);
    }

    [Fact]
    public void Percentage_Resolves_Against_Viewport()
    {
        Assert.Equal(50f, SvgLength.Parse("50%", viewport: 100f));
        Assert.Equal(25f, SvgLength.Parse("25%", viewport: 100f));
    }

    [Fact]
    public void Percentage_With_Zero_Viewport_Is_Zero()
    {
        Assert.Equal(0f, SvgLength.Parse("50%"));
    }

    [Fact]
    public void Empty_Returns_Default()
    {
        Assert.Equal(100f, SvgLength.Parse(null, defaultIfMissing: 100f));
        Assert.Equal(100f, SvgLength.Parse("", defaultIfMissing: 100f));
        Assert.Equal(100f, SvgLength.Parse("   ", defaultIfMissing: 100f));
    }

    [Fact]
    public void Invalid_Returns_Default()
    {
        Assert.Equal(5f, SvgLength.Parse("not-a-number", defaultIfMissing: 5f));
    }

    [Fact]
    public void Case_Insensitive_Units()
    {
        Assert.Equal(96f, SvgLength.Parse("1IN"), 3);
        Assert.Equal(96f, SvgLength.Parse("1In"), 3);
    }

    [Fact]
    public void Negative_Values_Parsed()
    {
        Assert.Equal(-10f, SvgLength.Parse("-10"));
        Assert.Equal(-10f, SvgLength.Parse("-10px"));
    }
}
