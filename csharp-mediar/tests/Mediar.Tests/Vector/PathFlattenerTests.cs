using System.Numerics;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class PathFlattenerTests
{
    [Fact]
    public void Empty_Path_Produces_No_Segments()
    {
        var path = new Path2D();
        Assert.Empty(PathFlattener.Flatten(path, Matrix3x2.Identity).ToList());
    }

    [Fact]
    public void LineTo_Emits_Single_LineSegment()
    {
        var path = new Path2D().MoveTo(0, 0).LineTo(10, 10);
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity).ToList();
        Assert.Single(segs);
        Assert.Equal(new Vector2(0, 0), segs[0].P0);
        Assert.Equal(new Vector2(10, 10), segs[0].P1);
    }

    [Fact]
    public void Quad_Is_Subdivided_Into_Multiple_Lines()
    {
        var path = new Path2D().MoveTo(0, 0).QuadTo(new Vector2(5, 20), new Vector2(10, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.1f).ToList();
        Assert.True(segs.Count > 4, $"expected subdivision but got {segs.Count} segments");

        // First start must be the origin, last endpoint must be the curve endpoint.
        Assert.Equal(new Vector2(0, 0), segs[0].P0);
        Assert.Equal(new Vector2(10, 0), segs[^1].P1);
    }

    [Fact]
    public void Cubic_Is_Subdivided_To_Reach_Endpoint()
    {
        var path = new Path2D().MoveTo(0, 0).CubicTo(new Vector2(0, 30), new Vector2(30, 30), new Vector2(30, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.1f).ToList();
        Assert.Equal(new Vector2(30, 0), segs[^1].P1);
    }

    [Fact]
    public void Close_Emits_Closing_Line_Back_To_SubPath_Start()
    {
        var path = new Path2D().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10).Close();
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity).ToList();
        Assert.Equal(3, segs.Count);
        Assert.Equal(new Vector2(10, 10), segs[2].P0);
        Assert.Equal(new Vector2(0, 0), segs[2].P1);
    }

    [Fact]
    public void Transform_Is_Applied_To_All_Points()
    {
        var path = new Path2D().MoveTo(0, 0).LineTo(1, 0);
        var m = Matrix3x2.CreateScale(10f) * Matrix3x2.CreateTranslation(5f, 0f);
        var segs = PathFlattener.Flatten(path, m).ToList();
        Assert.Single(segs);
        Assert.Equal(new Vector2(5, 0), segs[0].P0);
        Assert.Equal(new Vector2(15, 0), segs[0].P1);
    }

    [Fact]
    public void Higher_Zoom_Increases_Subdivision_Count()
    {
        var path = new Path2D().MoveTo(0, 0).QuadTo(new Vector2(5, 20), new Vector2(10, 0));
        int low = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.5f).Count();
        int high = PathFlattener.Flatten(path, Matrix3x2.CreateScale(20f), 0.5f).Count();
        Assert.True(high > low, $"expected zoom to increase subdivision, got low={low}, high={high}");
    }

    [Fact]
    public void MoveTo_Only_Path_Produces_No_Segments()
    {
        var path = new Path2D().MoveTo(5, 5);
        Assert.Empty(PathFlattener.Flatten(path, Matrix3x2.Identity).ToList());
    }

    [Fact]
    public void Close_Without_Movement_Emits_Nothing()
    {
        // MoveTo + Close — cur == sub, so closing line is suppressed.
        var path = new Path2D().MoveTo(5, 5).Close();
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity).ToList();
        Assert.Empty(segs);
    }

    [Fact]
    public void Multiple_Sub_Paths_Each_Emit_Their_Own_Close()
    {
        var path = new Path2D()
            .MoveTo(0, 0).LineTo(10, 0).Close()
            .MoveTo(20, 0).LineTo(30, 0).Close();
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity).ToList();
        // Each sub-path: 1 LineTo + 1 Close => 2 segments, total 4.
        Assert.Equal(4, segs.Count);
        Assert.Equal(new Vector2(0, 0), segs[1].P1);
        Assert.Equal(new Vector2(20, 0), segs[3].P1);
    }

    [Fact]
    public void Collinear_Quad_Control_Yields_Single_Line()
    {
        // Control on the chord midpoint => deflection is zero.
        var path = new Path2D().MoveTo(0, 0).QuadTo(new Vector2(5, 0), new Vector2(10, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity).ToList();
        Assert.Single(segs);
        Assert.Equal(new Vector2(0, 0), segs[0].P0);
        Assert.Equal(new Vector2(10, 0), segs[0].P1);
    }

    [Fact]
    public void Collinear_Cubic_Controls_Yield_Single_Line()
    {
        // Both control points on the chord => no subdivision needed.
        var path = new Path2D().MoveTo(0, 0).CubicTo(
            new Vector2(3, 0), new Vector2(7, 0), new Vector2(10, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity).ToList();
        Assert.Single(segs);
        Assert.Equal(new Vector2(10, 0), segs[0].P1);
    }

    [Fact]
    public void Very_Large_Tolerance_Yields_Single_Segment_Per_Curve()
    {
        var path = new Path2D().MoveTo(0, 0).CubicTo(
            new Vector2(0, 100), new Vector2(100, 100), new Vector2(100, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 10000f).ToList();
        Assert.Single(segs);
        Assert.Equal(new Vector2(100, 0), segs[0].P1);
    }

    [Fact]
    public void Tight_Tolerance_Always_Reaches_Endpoint()
    {
        // Even at depth-cap tolerance the endpoint must be exact.
        var path = new Path2D().MoveTo(0, 0).CubicTo(
            new Vector2(0, 1000), new Vector2(1000, 1000), new Vector2(1000, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.001f).ToList();
        Assert.Equal(new Vector2(1000, 0), segs[^1].P1);
    }

    [Fact]
    public void Degenerate_Cubic_With_All_Equal_Points_Yields_Single_Segment()
    {
        var p = new Vector2(5, 5);
        var path = new Path2D().MoveTo(p).CubicTo(p, p, p);
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity).ToList();
        Assert.Single(segs);
        Assert.Equal(p, segs[0].P0);
        Assert.Equal(p, segs[0].P1);
    }

    [Fact]
    public void Translation_Only_Transform_Shifts_All_Points()
    {
        var path = new Path2D().MoveTo(0, 0).LineTo(10, 0);
        var segs = PathFlattener.Flatten(path, Matrix3x2.CreateTranslation(3, 7)).ToList();
        Assert.Single(segs);
        Assert.Equal(new Vector2(3, 7), segs[0].P0);
        Assert.Equal(new Vector2(13, 7), segs[0].P1);
    }

    [Fact]
    public void Rotation_Transform_Preserves_Length()
    {
        var path = new Path2D().MoveTo(0, 0).LineTo(10, 0);
        var rot = Matrix3x2.CreateRotation(MathF.PI / 2f);
        var segs = PathFlattener.Flatten(path, rot).ToList();
        Assert.Single(segs);
        Assert.Equal(0f, segs[0].P0.X, 4);
        Assert.Equal(0f, segs[0].P0.Y, 4);
        Assert.Equal(0f, segs[0].P1.X, 4);
        Assert.Equal(10f, segs[0].P1.Y, 4);
    }

    [Fact]
    public void Quad_With_Zero_Length_Chord_Stays_Bounded()
    {
        // Endpoint same as start with curved control — flattener must terminate.
        var path = new Path2D().MoveTo(0, 0).QuadTo(new Vector2(5, 5), new Vector2(0, 0));
        var segs = PathFlattener.Flatten(path, Matrix3x2.Identity, 0.5f).ToList();
        Assert.NotEmpty(segs);
        Assert.True(segs.Count < 1000);
    }
}
