using System.Numerics;
using Mediar.Codecs.SvgRaster;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgPathDataParserTests
{
    [Fact]
    public void Empty_Returns_Empty_Path()
    {
        var p = SvgPathDataParser.Parse("");
        Assert.True(p.IsEmpty);
    }

    [Fact]
    public void Null_Returns_Empty_Path()
    {
        var p = SvgPathDataParser.Parse(null);
        Assert.True(p.IsEmpty);
    }

    [Fact]
    public void Simple_MoveTo_LineTo()
    {
        var p = SvgPathDataParser.Parse("M 10 20 L 30 40");
        Assert.Equal(2, p.Segments.Count);
        Assert.Equal(PathVerb.MoveTo, p.Segments[0].Verb);
        Assert.Equal(new Vector2(10, 20), p.Segments[0].P0);
        Assert.Equal(new Vector2(30, 40), p.Segments[1].P0);
    }

    [Fact]
    public void Implicit_LineTo_After_MoveTo()
    {
        // "M 0 0 10 10 20 20" - subsequent pairs after M are implicit L.
        var p = SvgPathDataParser.Parse("M 0 0 10 10 20 20");
        Assert.Equal(3, p.Segments.Count);
        Assert.Equal(PathVerb.LineTo, p.Segments[1].Verb);
        Assert.Equal(PathVerb.LineTo, p.Segments[2].Verb);
        Assert.Equal(new Vector2(20, 20), p.Segments[2].P0);
    }

    [Fact]
    public void Relative_Coordinates()
    {
        // m 10 10 l 5 5 -> moves to (10,10), line to (15,15)
        var p = SvgPathDataParser.Parse("m 10 10 l 5 5");
        Assert.Equal(new Vector2(10, 10), p.Segments[0].P0);
        Assert.Equal(new Vector2(15, 15), p.Segments[1].P0);
    }

    [Fact]
    public void Horizontal_And_Vertical_Lines()
    {
        var p = SvgPathDataParser.Parse("M 0 0 H 10 V 20");
        Assert.Equal(new Vector2(10, 0), p.Segments[1].P0);
        Assert.Equal(new Vector2(10, 20), p.Segments[2].P0);
    }

    [Fact]
    public void Cubic_Bezier_Curves()
    {
        var p = SvgPathDataParser.Parse("M 0 0 C 10 10 20 20 30 30");
        Assert.Equal(PathVerb.CubicTo, p.Segments[1].Verb);
        Assert.Equal(new Vector2(10, 10), p.Segments[1].P0);
        Assert.Equal(new Vector2(20, 20), p.Segments[1].P1);
        Assert.Equal(new Vector2(30, 30), p.Segments[1].P2);
    }

    [Fact]
    public void Quadratic_Bezier_Curves()
    {
        var p = SvgPathDataParser.Parse("M 0 0 Q 5 10 10 0");
        Assert.Equal(PathVerb.QuadTo, p.Segments[1].Verb);
        Assert.Equal(new Vector2(5, 10), p.Segments[1].P0);
        Assert.Equal(new Vector2(10, 0), p.Segments[1].P1);
    }

    [Fact]
    public void SmoothCubic_Reflects_Previous_Control()
    {
        var p = SvgPathDataParser.Parse("M 0 0 C 0 5 5 5 5 0 S 10 -5 10 0");
        // After C ending at (5,0) with last control (5,5), reflected = (5,-5).
        var smooth = p.Segments[2];
        Assert.Equal(PathVerb.CubicTo, smooth.Verb);
        Assert.Equal(new Vector2(5, -5), smooth.P0);
    }

    [Fact]
    public void Arc_Command_With_Flags()
    {
        var p = SvgPathDataParser.Parse("M 0 0 A 10 10 0 0 1 10 10");
        // Arc decomposed to one or more cubics; ensure we end at (10,10).
        var segs = PathFlattener.Flatten(p, Matrix3x2.Identity, 0.1f).ToList();
        Assert.Equal(10f, segs[^1].P1.X, 1);
        Assert.Equal(10f, segs[^1].P1.Y, 1);
    }

    [Fact]
    public void Close_Path()
    {
        var p = SvgPathDataParser.Parse("M 0 0 L 10 0 L 10 10 Z");
        Assert.Equal(4, p.Segments.Count);
        Assert.Equal(PathVerb.Close, p.Segments[3].Verb);
    }

    [Fact]
    public void Commands_Without_Whitespace()
    {
        var p = SvgPathDataParser.Parse("M10,20L30,40");
        Assert.Equal(new Vector2(10, 20), p.Segments[0].P0);
        Assert.Equal(new Vector2(30, 40), p.Segments[1].P0);
    }

    [Fact]
    public void Negative_Numbers_Without_Separator()
    {
        // "L10-5" — the minus sign acts as separator.
        var p = SvgPathDataParser.Parse("M0 0L10-5");
        Assert.Equal(new Vector2(10, -5), p.Segments[1].P0);
    }

    [Fact]
    public void Decimal_Numbers_Without_Separator()
    {
        // "1.5.5" should parse as 1.5 and 0.5
        var p = SvgPathDataParser.Parse("M0 0L1.5.5");
        Assert.Equal(1.5f, p.Segments[1].P0.X);
        Assert.Equal(0.5f, p.Segments[1].P0.Y);
    }

    [Fact]
    public void Whitespace_Only_Returns_Empty_Path()
    {
        var p = SvgPathDataParser.Parse("   \t\r\n  ");
        Assert.True(p.IsEmpty);
    }

    [Fact]
    public void Relative_Cubic_Bezier_Coordinates_Are_Relative_To_Current()
    {
        // After M 10 10, "c 0 5 5 5 5 0" -> cubic ending at (15,10).
        var p = SvgPathDataParser.Parse("M 10 10 c 0 5 5 5 5 0");
        var seg = p.Segments[1];
        Assert.Equal(PathVerb.CubicTo, seg.Verb);
        Assert.Equal(new Vector2(10, 15), seg.P0);
        Assert.Equal(new Vector2(15, 15), seg.P1);
        Assert.Equal(new Vector2(15, 10), seg.P2);
    }

    [Fact]
    public void SmoothQuad_T_Reflects_Previous_Quad_Control()
    {
        // After Q 5 5 10 0 (control (5,5), endpoint (10,0)),
        // a T at (15,0) reflects to control = 2*(10,0)-(5,5) = (15,-5).
        var p = SvgPathDataParser.Parse("M 0 0 Q 5 5 10 0 T 15 0");
        var smooth = p.Segments[2];
        Assert.Equal(PathVerb.QuadTo, smooth.Verb);
        Assert.Equal(new Vector2(15, -5), smooth.P0);
        Assert.Equal(new Vector2(15, 0), smooth.P1);
    }

    [Fact]
    public void SmoothQuad_T_Without_Previous_Quad_Uses_Current_Point()
    {
        // M, then L, then T — no preceding Q so first control = current point.
        var p = SvgPathDataParser.Parse("M 0 0 L 3 3 T 6 3");
        var smooth = p.Segments[2];
        Assert.Equal(PathVerb.QuadTo, smooth.Verb);
        Assert.Equal(new Vector2(3, 3), smooth.P0);
        Assert.Equal(new Vector2(6, 3), smooth.P1);
    }

    [Fact]
    public void Relative_Horizontal_And_Vertical_Lines()
    {
        // M 5 5, h 10 -> (15,5); v -3 -> (15,2).
        var p = SvgPathDataParser.Parse("M 5 5 h 10 v -3");
        Assert.Equal(new Vector2(15, 5), p.Segments[1].P0);
        Assert.Equal(new Vector2(15, 2), p.Segments[2].P0);
    }

    [Fact]
    public void Two_Subpaths_Are_Each_Recorded()
    {
        // M 0 0 L 10 0 M 0 5 L 10 5 -> two sub-paths.
        var p = SvgPathDataParser.Parse("M 0 0 L 10 0 M 0 5 L 10 5");
        Assert.Equal(4, p.Segments.Count);
        Assert.Equal(PathVerb.MoveTo, p.Segments[2].Verb);
        Assert.Equal(new Vector2(0, 5), p.Segments[2].P0);
    }

    [Fact]
    public void Close_Followed_By_Implicit_LineTo_Continues_From_SubPath_Start()
    {
        // After "Z" the current point returns to the last MoveTo (0,0).
        // The trailing implicit lineto via "L" should start from there.
        var p = SvgPathDataParser.Parse("M 0 0 L 10 0 Z L 5 5");
        var last = p.Segments[^1];
        Assert.Equal(PathVerb.LineTo, last.Verb);
        Assert.Equal(new Vector2(5, 5), last.P0);
    }
}
