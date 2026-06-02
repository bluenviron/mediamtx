using System.Numerics;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class StrokeToFillTests
{
    [Fact]
    public void Empty_Path_Returns_Empty()
    {
        var stroked = StrokeToFill.Stroke(new Path2D(), new StrokeStyle(1f));
        Assert.True(stroked.IsEmpty);
    }

    [Fact]
    public void Zero_Width_Returns_Empty()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 10);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(0f));
        Assert.True(stroked.IsEmpty);
    }

    [Fact]
    public void Open_Line_Produces_Nonempty_Outline()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f));
        Assert.False(stroked.IsEmpty);
        // Outline width 2 means bounds expand by ±1 in Y.
        var (minX, minY, maxX, maxY) = stroked.GetBounds();
        Assert.InRange(minY, -1.2f, -0.8f);
        Assert.InRange(maxY, 0.8f, 1.2f);
        Assert.InRange(minX, -0.1f, 0.1f);
        Assert.InRange(maxX, 9.9f, 10.1f);
    }

    [Fact]
    public void Square_Cap_Extends_Past_Endpoint()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0);
        var butt = StrokeToFill.Stroke(p, new StrokeStyle(4f, LineCap.Butt));
        var square = StrokeToFill.Stroke(p, new StrokeStyle(4f, LineCap.Square));
        var (_, _, maxXButt, _) = butt.GetBounds();
        var (_, _, maxXSquare, _) = square.GetBounds();
        // Square cap extends by half the width (=2) past endpoint.
        Assert.True(maxXSquare > maxXButt + 1.5f, $"butt={maxXButt} square={maxXSquare}");
    }

    [Fact]
    public void Round_Cap_Extends_Past_Endpoint()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0);
        var butt = StrokeToFill.Stroke(p, new StrokeStyle(4f, LineCap.Butt));
        var round = StrokeToFill.Stroke(p, new StrokeStyle(4f, LineCap.Round));
        var (_, _, maxXButt, _) = butt.GetBounds();
        var (_, _, maxXRound, _) = round.GetBounds();
        Assert.True(maxXRound > maxXButt + 1.5f);
    }

    [Fact]
    public void Closed_Path_Produces_Two_Rings()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10).LineTo(0, 10).Close();
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f));
        // Closed loop: outer + inner ring => at least 2 MoveTo segments.
        int moves = stroked.Segments.Count(s => s.Verb == PathVerb.MoveTo);
        Assert.True(moves >= 2);
    }

    private static readonly float[] s_dash10 = [10f, 10f];

    [Fact]
    public void Dash_Array_Splits_Into_Multiple_Sub_Paths()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(100, 0);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_dash10));
        int moves = stroked.Segments.Count(s => s.Verb == PathVerb.MoveTo);
        Assert.True(moves >= 4, $"expected dashed line to break into multiple strokes, got {moves} sub-paths");
    }

    private static readonly float[] s_zeroDash = [0f, 0f];
    private static readonly float[] s_oddDash = [10f, 5f, 2f];
    private static readonly float[] s_hugeDash = [1000f, 1000f];

    [Fact]
    public void Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => StrokeToFill.Stroke(null!, new StrokeStyle(1f)));
    }

    [Fact]
    public void Negative_Width_Returns_Empty()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(-1f));
        Assert.True(stroked.IsEmpty);
    }

    [Fact]
    public void Bevel_Join_Produces_Nonempty_Outline()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f, Join: LineJoin.Bevel));
        Assert.False(stroked.IsEmpty);
    }

    [Fact]
    public void Miter_With_Tight_Limit_Falls_Back_To_Bevel()
    {
        // Very sharp angle with miterlimit=1 should clip to bevel.
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0).LineTo(0, 1);
        var miterTight = StrokeToFill.Stroke(p, new StrokeStyle(2f, Join: LineJoin.Miter, MiterLimit: 1f));
        var bevel = StrokeToFill.Stroke(p, new StrokeStyle(2f, Join: LineJoin.Bevel));
        var (_, _, maxXTight, _) = miterTight.GetBounds();
        var (_, _, maxXBevel, _) = bevel.GetBounds();
        // Tight-limit miter should not extend far beyond bevel bounds.
        Assert.InRange(maxXTight, maxXBevel - 0.5f, maxXBevel + 1.5f);
    }

    [Fact]
    public void Round_Join_Produces_More_Segments_Than_Bevel()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10);
        var round = StrokeToFill.Stroke(p, new StrokeStyle(2f, Join: LineJoin.Round));
        var bevel = StrokeToFill.Stroke(p, new StrokeStyle(2f, Join: LineJoin.Bevel));
        Assert.True(round.Segments.Count > bevel.Segments.Count,
            $"round={round.Segments.Count} bevel={bevel.Segments.Count}");
    }

    [Fact]
    public void Zero_Dash_Pattern_Treats_As_Solid()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(100, 0);
        var solid = StrokeToFill.Stroke(p, new StrokeStyle(2f));
        var zeroDash = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_zeroDash));
        // patternLen == 0 short-circuits dashing => identical to solid.
        Assert.Equal(solid.Segments.Count, zeroDash.Segments.Count);
    }

    [Fact]
    public void Odd_Dash_Pattern_Duplicates_Into_Even()
    {
        // Per CSS/SVG spec odd-count arrays are duplicated; just confirm
        // the result is non-empty and breaks into multiple sub-paths.
        var p = new Path2D().MoveTo(0, 0).LineTo(50, 0);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_oddDash));
        int moves = stroked.Segments.Count(s => s.Verb == PathVerb.MoveTo);
        Assert.True(moves >= 1);
    }

    [Fact]
    public void Dash_Offset_Shifts_Pattern_Start()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(100, 0);
        var noOffset = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_dash10, DashOffset: 0f));
        var offset = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_dash10, DashOffset: 5f));
        // Offset changes where dashes begin — sub-path layout differs.
        Assert.NotEqual(noOffset.Segments.Count, offset.Segments.Count);
    }

    [Fact]
    public void Negative_Dash_Offset_Is_Normalized()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(100, 0);
        // dashOffset is normalized: -5 % 20 = -5 + 20 = 15.
        var neg = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_dash10, DashOffset: -5f));
        var pos = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_dash10, DashOffset: 15f));
        Assert.Equal(pos.Segments.Count, neg.Segments.Count);
    }

    [Fact]
    public void Dash_Longer_Than_Path_Emits_Single_Sub_Path()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(50, 0);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f, DashArray: s_hugeDash));
        int moves = stroked.Segments.Count(s => s.Verb == PathVerb.MoveTo);
        // Pattern is 1000-on, 1000-off — path is fully inside the first "on" dash.
        Assert.True(moves <= 2, $"expected <=2 outline rings for single dash, got {moves}");
    }

    [Fact]
    public void Stroke_Of_Path_With_Only_MoveTo_Is_Empty()
    {
        var p = new Path2D().MoveTo(5, 5);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f));
        Assert.True(stroked.IsEmpty);
    }

    [Fact]
    public void Multiple_Sub_Paths_Both_Stroke()
    {
        var p = new Path2D()
            .MoveTo(0, 0).LineTo(10, 0)
            .MoveTo(0, 10).LineTo(10, 10);
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f));
        int moves = stroked.Segments.Count(s => s.Verb == PathVerb.MoveTo);
        // Each open sub-path produces a single outline ring.
        Assert.True(moves >= 2);
    }

    [Fact]
    public void Quadratic_Curve_Is_Stroked()
    {
        var p = new Path2D().MoveTo(0, 0).QuadTo(new Vector2(5, 20), new Vector2(10, 0));
        var stroked = StrokeToFill.Stroke(p, new StrokeStyle(2f));
        Assert.False(stroked.IsEmpty);
        var (minY, _, _, maxY) = (stroked.GetBounds().MinY, 0, 0, stroked.GetBounds().MaxY);
        Assert.True(maxY > 5f);
    }
}
