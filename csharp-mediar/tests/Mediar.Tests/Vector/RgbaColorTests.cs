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
}
