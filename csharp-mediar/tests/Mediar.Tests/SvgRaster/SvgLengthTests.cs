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

    [Fact]
    public void Leading_Whitespace_Is_Trimmed()
    {
        Assert.Equal(42f, SvgLength.Parse("   42"));
        Assert.Equal(42f, SvgLength.Parse("\t42px"));
    }

    [Fact]
    public void Trailing_Whitespace_Is_Trimmed()
    {
        Assert.Equal(42f, SvgLength.Parse("42   "));
        Assert.Equal(42f, SvgLength.Parse("42px\t"));
    }

    [Fact]
    public void Plus_Sign_Parsed()
    {
        Assert.Equal(10f, SvgLength.Parse("+10"));
        Assert.Equal(10f, SvgLength.Parse("+10px"));
    }

    [Fact]
    public void Scientific_Notation_Parsed()
    {
        Assert.Equal(100f, SvgLength.Parse("1e2"));
        Assert.Equal(100f, SvgLength.Parse("1.0E2px"));
    }

    [Fact]
    public void Percentage_Above_100_Resolves_To_Multiple_Of_Viewport()
    {
        Assert.Equal(200f, SvgLength.Parse("200%", viewport: 100f));
    }

    [Fact]
    public void Percentage_Negative_Resolves()
    {
        Assert.Equal(-50f, SvgLength.Parse("-50%", viewport: 100f));
    }

    [Fact]
    public void Bare_Percent_Sign_Returns_Default()
    {
        Assert.Equal(5f, SvgLength.Parse("%", defaultIfMissing: 5f));
    }

    [Fact]
    public void Default_If_Missing_Is_Zero_When_Unspecified()
    {
        Assert.Equal(0f, SvgLength.Parse(null));
        Assert.Equal(0f, SvgLength.Parse(""));
        Assert.Equal(0f, SvgLength.Parse("garbage"));
    }

    [Theory]
    [InlineData("3PX", 3f)]
    [InlineData("3Pt", 4f)]
    [InlineData("3EM", 48f)]
    [InlineData("3eX", 24f)]
    public void Unit_Suffixes_Are_Case_Insensitive(string text, float expected)
    {
        Assert.Equal(expected, SvgLength.Parse(text), 3);
    }

    [Fact]
    public void Decimal_Without_Leading_Zero_Parsed()
    {
        Assert.Equal(0.5f, SvgLength.Parse(".5"));
        Assert.Equal(-0.5f, SvgLength.Parse("-.5px"));
    }

    [Fact]
    public void Internal_Whitespace_Between_Number_And_Unit_Parsed()
    {
        // "100 px" — after stripping the "px" suffix, "100 " parses as 100
        // thanks to the inner Trim().
        Assert.Equal(100f, SvgLength.Parse("100 px"));
    }
}
