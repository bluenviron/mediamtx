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
}
