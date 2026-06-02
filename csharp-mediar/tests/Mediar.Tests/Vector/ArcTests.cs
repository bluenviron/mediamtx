using System.Numerics;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class ArcTests
{
    [Fact]
    public void ZeroRadius_Becomes_Line()
    {
        var p = new Path2D().MoveTo(0, 0).ArcTo(0, 10, 0, false, true, new Vector2(10, 0));
        // MoveTo + LineTo => 2 segments.
        Assert.Equal(2, p.Segments.Count);
        Assert.Equal(PathVerb.LineTo, p.Segments[1].Verb);
    }

    [Fact]
    public void Equal_Endpoints_Is_NoOp()
    {
        var p = new Path2D().MoveTo(5, 5).ArcTo(3, 3, 0, false, true, new Vector2(5, 5));
        // Only the MoveTo remains.
        Assert.Single(p.Segments);
    }

    [Fact]
    public void Quarter_Circle_Produces_Cubic_That_Approximates_Endpoint()
    {
        // 90° clockwise sweep from (10,0) to (0,10) with radius 10.
        var path = new Path2D().MoveTo(10, 0).ArcTo(10, 10, 0, false, true, new Vector2(0, 10));
        Assert.True(path.Segments.Count >= 2);

        // Flatten and verify endpoint is reached.
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.1f).ToList();
        var last = segs[^1];
        Assert.Equal(0f, last.P1.X, 1);
        Assert.Equal(10f, last.P1.Y, 1);
    }

    [Fact]
    public void Quarter_Circle_Stays_On_Circle()
    {
        var path = new Path2D().MoveTo(10, 0).ArcTo(10, 10, 0, false, true, new Vector2(0, 10));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.05f).ToList();
        // All flattened sample points lie close to the unit circle of radius 10 about origin.
        foreach (var s in segs)
        {
            float d = MathF.Sqrt(s.P0.X * s.P0.X + s.P0.Y * s.P0.Y);
            Assert.InRange(d, 9.5f, 10.5f);
        }
    }

    [Fact]
    public void LargeArc_Selects_The_Greater_Sweep()
    {
        // Two arcs between same endpoints with radius 10 - largeArc=true should go the long way around.
        var startEnd = (start: new Vector2(10, 0), end: new Vector2(0, 10));
        var pSmall = new Path2D().MoveTo(startEnd.start).ArcTo(10, 10, 0, false, true, startEnd.end);
        var pLarge = new Path2D().MoveTo(startEnd.start).ArcTo(10, 10, 0, true, true, startEnd.end);

        // Sum flattened arc-length: large arc should be much longer.
        float lenSmall = ArcLength(pSmall);
        float lenLarge = ArcLength(pLarge);
        // A 90° quarter arc has length r*π/2 ≈ 15.7; the corresponding
        // 270° large arc has length r*3π/2 ≈ 47.1, i.e. 3× longer.
        Assert.True(lenLarge > lenSmall * 2.5f, $"large={lenLarge} small={lenSmall}");
    }

    private static float ArcLength(Path2D path)
    {
        float len = 0;
        foreach (var s in PathFlattener.Flatten(path, Matrix3x2.Identity, 0.05f))
            len += (s.P1 - s.P0).Length();
        return len;
    }

    [Fact]
    public void Sweep_Flag_Reverses_Direction()
    {
        // Two arcs from (10,0) to (0,10) — one CW, one CCW — differ.
        var cw = new Path2D().MoveTo(10, 0).ArcTo(10, 10, 0, false, true, new Vector2(0, 10));
        var ccw = new Path2D().MoveTo(10, 0).ArcTo(10, 10, 0, false, false, new Vector2(0, 10));
        Assert.NotEqual(cw.Segments[1].P0, ccw.Segments[1].P0);
    }

    [Fact]
    public void Negative_Radii_Are_Treated_As_Positive()
    {
        // SVG F.6.6.1: signs on radii are stripped.
        var pPos = new Path2D().MoveTo(10, 0).ArcTo(10, 10, 0, false, true, new Vector2(0, 10));
        var pNeg = new Path2D().MoveTo(10, 0).ArcTo(-10, -10, 0, false, true, new Vector2(0, 10));
        Assert.Equal(pPos.Segments.Count, pNeg.Segments.Count);
        for (int i = 0; i < pPos.Segments.Count; i++)
        {
            Assert.Equal(pPos.Segments[i].P0, pNeg.Segments[i].P0);
            Assert.Equal(pPos.Segments[i].P1, pNeg.Segments[i].P1);
            Assert.Equal(pPos.Segments[i].P2, pNeg.Segments[i].P2);
        }
    }

    [Fact]
    public void OutOfRange_Radii_Are_Scaled_Up_To_Span_Chord()
    {
        // Chord (10,0)->(-10,0) has length 20 but rx=ry=1 cannot span it.
        // The arc must still reach the endpoint after radius correction.
        var path = new Path2D().MoveTo(10, 0).ArcTo(1, 1, 0, false, true, new Vector2(-10, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.1f).ToList();
        var last = segs[^1];
        Assert.Equal(-10f, last.P1.X, 1);
        Assert.Equal(0f, last.P1.Y, 1);
    }

    [Fact]
    public void Implicit_MoveTo_When_No_Current_Point()
    {
        // ArcTo without an explicit MoveTo should call EnsureCurrent which
        // adds an implicit MoveTo at origin.
        var path = new Path2D().ArcTo(10, 10, 0, false, true, new Vector2(10, 10));
        Assert.Equal(PathVerb.MoveTo, path.Segments[0].Verb);
        Assert.Equal(Vector2.Zero, path.Segments[0].P0);
        Assert.True(path.Segments.Count > 1);
    }

    [Fact]
    public void Rotated_Ellipse_Still_Reaches_Endpoint()
    {
        // Same arc with 45° rotation — endpoint must still land where specified.
        var path = new Path2D().MoveTo(10, 0).ArcTo(10, 5, 45f, false, true, new Vector2(0, 10));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.1f).ToList();
        var last = segs[^1];
        Assert.Equal(0f, last.P1.X, 1);
        Assert.Equal(10f, last.P1.Y, 1);
    }

    [Fact]
    public void Three_Sequential_Arcs_Each_Append_Segments()
    {
        var path = new Path2D()
            .MoveTo(0, 0)
            .ArcTo(5, 5, 0, false, true, new Vector2(10, 0))
            .ArcTo(5, 5, 0, false, true, new Vector2(20, 0))
            .ArcTo(5, 5, 0, false, true, new Vector2(30, 0));
        // Each arc emits at least one CubicTo (90° = single cubic, 180° = two cubics).
        int cubics = path.Segments.Count(s => s.Verb == PathVerb.CubicTo);
        Assert.True(cubics >= 3);
    }

    [Fact]
    public void Large_Arc_Together_With_Large_Sweep_Gives_LargeArc()
    {
        // largeArc == sweep is one branch of the sign selection logic; ensure
        // the chosen sign produces a valid centre and the arc reaches end.
        var path = new Path2D().MoveTo(10, 0).ArcTo(10, 10, 0, true, true, new Vector2(0, 10));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.2f).ToList();
        var last = segs[^1];
        Assert.Equal(0f, last.P1.X, 1);
        Assert.Equal(10f, last.P1.Y, 1);
    }

    [Fact]
    public void NonCircular_Ellipse_Stays_On_Ellipse()
    {
        // Quarter ellipse rx=20, ry=5, axis-aligned, from (20,0) to (0,5).
        var path = new Path2D().MoveTo(20, 0).ArcTo(20, 5, 0f, false, true, new Vector2(0, 5));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.05f).ToList();
        foreach (var s in segs)
        {
            // (x/20)^2 + (y/5)^2 ≈ 1
            float v = (s.P0.X * s.P0.X) / 400f + (s.P0.Y * s.P0.Y) / 25f;
            Assert.InRange(v, 0.85f, 1.15f);
        }
    }
}
