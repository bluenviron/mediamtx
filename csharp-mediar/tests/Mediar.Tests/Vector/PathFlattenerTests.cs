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
}
