using System.Numerics;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class GradientEvaluatorTests
{
    [Fact]
    public void Solid_Returns_Color_Everywhere()
    {
        var s = new SolidEvaluator(RgbaColor.FromBytes(255, 128, 0));
        var c = s.Evaluate(42f, 99f);
        Assert.Equal(255, (int)MathF.Round(c.R * 255));
        Assert.Equal(128, (int)MathF.Round(c.G * 255));
        Assert.Equal(0, (int)MathF.Round(c.B * 255));
    }

    [Fact]
    public void Linear_Gradient_Endpoint_Returns_End_Color()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Pad);
        var c0 = grad.Evaluate(0, 0);
        var c1 = grad.Evaluate(10, 0);
        Assert.Equal(1f, c0.R, 2);
        Assert.Equal(1f, c1.B, 2);
    }

    [Fact]
    public void Linear_Gradient_Mid_Is_Blend()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(5, 0);
        Assert.Equal(0.5f, c.R, 2);
        Assert.Equal(0.5f, c.B, 2);
    }

    [Fact]
    public void Pad_Spread_Clamps_Below_Zero()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(-100, 0);
        Assert.Equal(1f, c.R, 2);
    }

    [Fact]
    public void Repeat_Spread_Wraps()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Repeat);
        // t = 1.5 should wrap to t = 0.5 ~ mid color.
        var c = grad.Evaluate(15, 0);
        Assert.Equal(0.5f, c.R, 2);
        Assert.Equal(0.5f, c.B, 2);
    }

    [Fact]
    public void Reflect_Spread_Mirrors()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Reflect);
        // t = 1.5 with reflect should equal t = 0.5 mirrored = 0.5 of way from 1 back to 0 = mid color.
        var c = grad.Evaluate(15, 0);
        Assert.Equal(0.5f, c.R, 2);
        Assert.Equal(0.5f, c.B, 2);
    }

    [Fact]
    public void Radial_Gradient_Center_Is_First_Stop()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new RadialGradientEvaluator(new Vector2(0, 0), 10f, new Vector2(0, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(0, 0);
        Assert.Equal(1f, c.R, 2);
    }

    [Fact]
    public void Radial_Gradient_At_Boundary_Is_Last_Stop()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new RadialGradientEvaluator(new Vector2(0, 0), 10f, new Vector2(0, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(10, 0);
        Assert.Equal(1f, c.B, 2);
    }

    [Fact]
    public void Stops_Are_Sorted_Internally()
    {
        // Out-of-order stops should still produce monotone result.
        var stops = new[]
        {
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Pad);
        var c0 = grad.Evaluate(0, 0);
        var c1 = grad.Evaluate(10, 0);
        Assert.Equal(1f, c0.R, 2);
        Assert.Equal(1f, c1.B, 2);
    }
}
