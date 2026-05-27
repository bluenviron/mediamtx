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
}
