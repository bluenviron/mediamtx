using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class RgbaColorTests
{
    [Fact]
    public void Constants_Have_Expected_Values()
    {
        Assert.Equal(new RgbaColor(0, 0, 0, 0), RgbaColor.Transparent);
        Assert.Equal(new RgbaColor(0, 0, 0, 1), RgbaColor.Black);
        Assert.Equal(new RgbaColor(1, 1, 1, 1), RgbaColor.White);
    }

    [Fact]
    public void FromBytes_Normalizes_To_UnitInterval()
    {
        var c = RgbaColor.FromBytes(255, 128, 0, 200);
        Assert.Equal(1f, c.R);
        Assert.Equal(128f / 255f, c.G, 5);
        Assert.Equal(0f, c.B);
        Assert.Equal(200f / 255f, c.A, 5);
    }

    [Fact]
    public void FromBytes_Default_Alpha_Is_Opaque()
    {
        var c = RgbaColor.FromBytes(10, 20, 30);
        Assert.Equal(1f, c.A);
    }

    [Fact]
    public void ToBgra32_Packs_Channels_In_BGRA_Memory_Order()
    {
        // Red opaque
        uint packed = RgbaColor.FromBytes(255, 0, 0).ToBgra32();
        // Little-endian uint stored as bytes B G R A => 00 00 FF FF
        Assert.Equal(0u, packed & 0xFF);                // B byte
        Assert.Equal(0u, (packed >> 8) & 0xFF);          // G byte
        Assert.Equal(255u, (packed >> 16) & 0xFF);       // R byte
        Assert.Equal(255u, (packed >> 24) & 0xFF);       // A byte
    }

    [Fact]
    public void ToRgba32_Packs_Channels_In_RGBA_Memory_Order()
    {
        uint packed = RgbaColor.FromBytes(255, 0, 0).ToRgba32();
        Assert.Equal(255u, packed & 0xFF);               // R byte
        Assert.Equal(0u, (packed >> 8) & 0xFF);          // G byte
        Assert.Equal(0u, (packed >> 16) & 0xFF);         // B byte
        Assert.Equal(255u, (packed >> 24) & 0xFF);       // A byte
    }

    [Fact]
    public void WithOpacity_Scales_Alpha_And_Clamps()
    {
        var c = new RgbaColor(1, 1, 1, 1);
        Assert.Equal(0.5f, c.WithOpacity(0.5f).A);
        Assert.Equal(1f, c.WithOpacity(2f).A);       // clamps to 1
        Assert.Equal(0f, c.WithOpacity(-0.5f).A);     // clamps to 0
    }

    [Fact]
    public void Lerp_Mid_Is_Average()
    {
        var a = new RgbaColor(0, 0, 0, 0);
        var b = new RgbaColor(1, 1, 1, 1);
        var m = RgbaColor.Lerp(a, b, 0.5f);
        Assert.Equal(0.5f, m.R);
        Assert.Equal(0.5f, m.G);
        Assert.Equal(0.5f, m.B);
        Assert.Equal(0.5f, m.A);
    }

    [Fact]
    public void Lerp_Clamps_T()
    {
        var a = new RgbaColor(0, 0, 0, 0);
        var b = new RgbaColor(1, 1, 1, 1);
        Assert.Equal(a, RgbaColor.Lerp(a, b, -1f));
        Assert.Equal(b, RgbaColor.Lerp(a, b, 2f));
    }

    [Fact]
    public void ToBgra32_Handles_NaN_Safely()
    {
        // Components that are NaN should resolve to 0 (not throw).
        var c = new RgbaColor(float.NaN, 0, 0, 1);
        uint packed = c.ToBgra32();
        Assert.Equal(0u, (packed >> 16) & 0xFF);
    }

    [Fact]
    public void ToBgra32_Clamps_Negative_And_Greater_Than_One()
    {
        var c = new RgbaColor(-1f, 2f, 0.5f, 1f);
        uint packed = c.ToBgra32();
        Assert.Equal(0u, (packed >> 16) & 0xFF);  // R clamped to 0
        Assert.Equal(255u, (packed >> 8) & 0xFF); // G clamped to 1
        Assert.InRange((packed >> 0) & 0xFF, 127u, 129u);
    }

    [Fact]
    public void ToRgba32_All_Channels_Set()
    {
        // Distinct channels prove the packing order.
        uint packed = RgbaColor.FromBytes(10, 20, 30, 40).ToRgba32();
        Assert.Equal(10u, packed & 0xFF);
        Assert.Equal(20u, (packed >> 8) & 0xFF);
        Assert.Equal(30u, (packed >> 16) & 0xFF);
        Assert.Equal(40u, (packed >> 24) & 0xFF);
    }

    [Fact]
    public void Lerp_T_Zero_Returns_A()
    {
        var a = new RgbaColor(0.1f, 0.2f, 0.3f, 0.4f);
        var b = new RgbaColor(1f, 1f, 1f, 1f);
        var r = RgbaColor.Lerp(a, b, 0f);
        Assert.Equal(a, r);
    }

    [Fact]
    public void Lerp_T_One_Returns_B()
    {
        var a = new RgbaColor(0.1f, 0.2f, 0.3f, 0.4f);
        var b = new RgbaColor(1f, 1f, 1f, 1f);
        var r = RgbaColor.Lerp(a, b, 1f);
        Assert.Equal(b, r);
    }

    [Fact]
    public void WithOpacity_Zero_Yields_Transparent_Alpha()
    {
        var c = new RgbaColor(1, 1, 1, 1);
        Assert.Equal(0f, c.WithOpacity(0f).A);
    }

    [Fact]
    public void WithOpacity_Preserves_RGB_Channels()
    {
        var c = new RgbaColor(0.25f, 0.5f, 0.75f, 1f);
        var faded = c.WithOpacity(0.5f);
        Assert.Equal(0.25f, faded.R);
        Assert.Equal(0.5f, faded.G);
        Assert.Equal(0.75f, faded.B);
        Assert.Equal(0.5f, faded.A);
    }

    [Fact]
    public void Record_Equality_Holds_On_Identical_Components()
    {
        Assert.Equal(new RgbaColor(0.1f, 0.2f, 0.3f, 0.4f),
                     new RgbaColor(0.1f, 0.2f, 0.3f, 0.4f));
    }

    [Fact]
    public void Record_Inequality_On_Differing_Component()
    {
        Assert.NotEqual(new RgbaColor(0.1f, 0.2f, 0.3f, 0.4f),
                        new RgbaColor(0.1f, 0.2f, 0.3f, 0.5f));
    }

    [Fact]
    public void With_Expression_Replaces_Channel()
    {
        var c = RgbaColor.White with { A = 0.25f };
        Assert.Equal(1f, c.R);
        Assert.Equal(1f, c.G);
        Assert.Equal(1f, c.B);
        Assert.Equal(0.25f, c.A);
    }

    [Theory]
    [InlineData(0, 0)]
    [InlineData(1, 1)]
    [InlineData(127, 127)]
    [InlineData(128, 128)]
    [InlineData(254, 254)]
    [InlineData(255, 255)]
    public void FromBytes_Then_ToBgra32_Round_Trips_All_Byte_Values(byte input, byte expected)
    {
        var c = RgbaColor.FromBytes(input, input, input, 255);
        uint packed = c.ToBgra32();
        Assert.Equal((uint)expected, packed & 0xFF);
    }

    [Fact]
    public void FromBytes_Equivalence_With_Direct_Construction()
    {
        var a = RgbaColor.FromBytes(255, 0, 128, 64);
        var b = new RgbaColor(1f, 0f, 128f / 255f, 64f / 255f);
        Assert.Equal(a, b);
    }
}
