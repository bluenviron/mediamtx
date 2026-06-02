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

    [Fact]
    public void Linear_Null_Stops_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), null!, GradientSpread.Pad));
    }

    [Fact]
    public void Radial_Null_Stops_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            new RadialGradientEvaluator(new Vector2(0, 0), 10f, new Vector2(0, 0), null!, GradientSpread.Pad));
    }

    [Fact]
    public void Linear_Empty_Stops_Returns_Transparent_Everywhere()
    {
        // SortAndFillStops fills an empty array with a single transparent stop.
        var grad = new LinearGradientEvaluator(
            new Vector2(0, 0), new Vector2(10, 0), Array.Empty<GradientStop>(), GradientSpread.Pad);
        var c = grad.Evaluate(5, 0);
        Assert.Equal(0f, c.A, 3);
    }

    [Fact]
    public void Linear_Zero_Length_Returns_First_Stop_Color()
    {
        // p1 == p2 -> _lenSq < epsilon -> returns first stop unchanged.
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(10, 20, 30)),
            new GradientStop(1f, RgbaColor.FromBytes(200, 100, 50)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(5, 5), new Vector2(5, 5), stops, GradientSpread.Pad);
        var c = grad.Evaluate(999, -999);
        Assert.Equal(10f / 255f, c.R, 2);
        Assert.Equal(20f / 255f, c.G, 2);
        Assert.Equal(30f / 255f, c.B, 2);
    }

    [Fact]
    public void Linear_Pad_Above_One_Returns_Last_Color()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(500, 0);
        Assert.Equal(1f, c.B, 2);
    }

    [Fact]
    public void Linear_Single_Stop_Returns_Stop_Color_Everywhere()
    {
        // Single stop -> SortAndFillStops pads with same color at both ends.
        var stops = new[] { new GradientStop(0.5f, RgbaColor.FromBytes(0, 255, 0)) };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Pad);
        Assert.Equal(1f, grad.Evaluate(0, 0).G, 2);
        Assert.Equal(1f, grad.Evaluate(5, 0).G, 2);
        Assert.Equal(1f, grad.Evaluate(10, 0).G, 2);
    }

    [Fact]
    public void Reflect_Negative_T_Matches_Mirror_Position()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new LinearGradientEvaluator(new Vector2(0, 0), new Vector2(10, 0), stops, GradientSpread.Reflect);
        // t = -0.5 should reflect to t = 0.5 -> mid color.
        var c = grad.Evaluate(-5, 0);
        Assert.Equal(0.5f, c.R, 2);
        Assert.Equal(0.5f, c.B, 2);
    }

    [Fact]
    public void Radial_Zero_Radius_Returns_Last_Stop()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new RadialGradientEvaluator(new Vector2(0, 0), 0f, new Vector2(0, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(5, 5);
        Assert.Equal(1f, c.B, 2);
    }

    [Fact]
    public void Radial_Far_Outside_With_Pad_Returns_Last_Stop()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new RadialGradientEvaluator(new Vector2(0, 0), 10f, new Vector2(0, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(1000, 0);
        Assert.Equal(1f, c.B, 2);
    }

    [Fact]
    public void Radial_Mid_Distance_Is_Blend()
    {
        var stops = new[]
        {
            new GradientStop(0f, RgbaColor.FromBytes(255, 0, 0)),
            new GradientStop(1f, RgbaColor.FromBytes(0, 0, 255)),
        };
        var grad = new RadialGradientEvaluator(new Vector2(0, 0), 10f, new Vector2(0, 0), stops, GradientSpread.Pad);
        var c = grad.Evaluate(5, 0);
        Assert.Equal(0.5f, c.R, 2);
        Assert.Equal(0.5f, c.B, 2);
    }
}
