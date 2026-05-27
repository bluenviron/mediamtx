using System.Numerics;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class Path2DTests
{
    [Fact]
    public void New_Path_Is_Empty()
    {
        var p = new Path2D();
        Assert.True(p.IsEmpty);
        Assert.Empty(p.Segments);
    }

    [Fact]
    public void MoveTo_LineTo_Records_Two_Segments()
    {
        var p = new Path2D().MoveTo(1, 2).LineTo(3, 4);
        Assert.Equal(2, p.Segments.Count);
        Assert.Equal(PathVerb.MoveTo, p.Segments[0].Verb);
        Assert.Equal(new Vector2(1, 2), p.Segments[0].P0);
        Assert.Equal(PathVerb.LineTo, p.Segments[1].Verb);
        Assert.Equal(new Vector2(3, 4), p.Segments[1].P0);
    }

    [Fact]
    public void LineTo_Without_MoveTo_Adds_Implicit_Origin()
    {
        var p = new Path2D().LineTo(5, 5);
        Assert.Equal(2, p.Segments.Count);
        Assert.Equal(PathVerb.MoveTo, p.Segments[0].Verb);
        Assert.Equal(Vector2.Zero, p.Segments[0].P0);
    }

    [Fact]
    public void Close_Returns_To_SubPathStart()
    {
        var p = new Path2D().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10).Close();
        Assert.Equal(4, p.Segments.Count);
        Assert.Equal(PathVerb.Close, p.Segments[3].Verb);
        Assert.Equal(new Vector2(0, 0), p.Segments[3].P0);
    }

    [Fact]
    public void Close_On_Empty_Is_NoOp()
    {
        var p = new Path2D().Close();
        Assert.True(p.IsEmpty);
    }

    [Fact]
    public void QuadTo_Records_Control_And_Endpoint()
    {
        var p = new Path2D().MoveTo(0, 0).QuadTo(new Vector2(5, 10), new Vector2(10, 0));
        Assert.Equal(PathVerb.QuadTo, p.Segments[1].Verb);
        Assert.Equal(new Vector2(5, 10), p.Segments[1].P0);
        Assert.Equal(new Vector2(10, 0), p.Segments[1].P1);
    }

    [Fact]
    public void CubicTo_Records_Both_Controls_And_Endpoint()
    {
        var p = new Path2D().MoveTo(0, 0).CubicTo(new Vector2(1, 1), new Vector2(2, 2), new Vector2(3, 3));
        Assert.Equal(PathVerb.CubicTo, p.Segments[1].Verb);
        Assert.Equal(new Vector2(1, 1), p.Segments[1].P0);
        Assert.Equal(new Vector2(2, 2), p.Segments[1].P1);
        Assert.Equal(new Vector2(3, 3), p.Segments[1].P2);
    }

    [Fact]
    public void SmoothCubicTo_Reflects_Previous_Cubic_Control()
    {
        // After cubic to (3,3) with last control (2,2) at current (3,3),
        // reflected control is 2*(3,3) - (2,2) = (4,4).
        var p = new Path2D()
            .MoveTo(0, 0)
            .CubicTo(new Vector2(1, 1), new Vector2(2, 2), new Vector2(3, 3))
            .SmoothCubicTo(new Vector2(5, 5), new Vector2(6, 6));
        // The Smooth call expanded to CubicTo with first control reflected.
        var seg = p.Segments[2];
        Assert.Equal(PathVerb.CubicTo, seg.Verb);
        Assert.Equal(new Vector2(4, 4), seg.P0);
    }

    [Fact]
    public void SmoothCubicTo_Without_Previous_Cubic_Uses_Current_Point()
    {
        var p = new Path2D()
            .MoveTo(0, 0)
            .LineTo(3, 3)
            .SmoothCubicTo(new Vector2(5, 5), new Vector2(6, 6));
        var seg = p.Segments[2];
        Assert.Equal(new Vector2(3, 3), seg.P0); // first control = current point
    }

    [Fact]
    public void SmoothQuadTo_Reflects_Previous_Quad_Control()
    {
        var p = new Path2D()
            .MoveTo(0, 0)
            .QuadTo(new Vector2(2, 2), new Vector2(4, 4))
            .SmoothQuadTo(new Vector2(8, 8));
        var seg = p.Segments[2];
        Assert.Equal(PathVerb.QuadTo, seg.Verb);
        Assert.Equal(new Vector2(6, 6), seg.P0); // 2*(4,4) - (2,2)
    }

    [Fact]
    public void GetBounds_Includes_All_Control_Points()
    {
        var p = new Path2D()
            .MoveTo(0, 0)
            .CubicTo(new Vector2(-5, -10), new Vector2(10, 20), new Vector2(5, 5));
        var (minX, minY, maxX, maxY) = p.GetBounds();
        Assert.Equal(-5f, minX);
        Assert.Equal(-10f, minY);
        Assert.Equal(10f, maxX);
        Assert.Equal(20f, maxY);
    }

    [Fact]
    public void Append_Concatenates_Segments()
    {
        var a = new Path2D().MoveTo(0, 0).LineTo(1, 1);
        var b = new Path2D().MoveTo(2, 2).LineTo(3, 3);
        a.Append(b);
        Assert.Equal(4, a.Segments.Count);
    }
}
