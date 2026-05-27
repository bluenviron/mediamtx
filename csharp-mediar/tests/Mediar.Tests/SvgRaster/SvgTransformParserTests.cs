using System.Numerics;
using Mediar.Codecs.SvgRaster;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgTransformParserTests
{
    [Fact]
    public void Empty_Returns_Identity()
    {
        Assert.Equal(Matrix3x2.Identity, SvgTransformParser.Parse(""));
        Assert.Equal(Matrix3x2.Identity, SvgTransformParser.Parse(null));
    }

    [Fact]
    public void Translate_Single_Argument_Defaults_Y_To_Zero()
    {
        var m = SvgTransformParser.Parse("translate(10)");
        var p = Vector2.Transform(Vector2.Zero, m);
        Assert.Equal(new Vector2(10, 0), p);
    }

    [Fact]
    public void Translate_Two_Arguments()
    {
        var m = SvgTransformParser.Parse("translate(10, 20)");
        var p = Vector2.Transform(Vector2.Zero, m);
        Assert.Equal(new Vector2(10, 20), p);
    }

    [Fact]
    public void Scale_Single_Argument_Is_Uniform()
    {
        var m = SvgTransformParser.Parse("scale(2)");
        var p = Vector2.Transform(new Vector2(3, 4), m);
        Assert.Equal(new Vector2(6, 8), p);
    }

    [Fact]
    public void Scale_Two_Arguments_Per_Axis()
    {
        var m = SvgTransformParser.Parse("scale(2, 3)");
        var p = Vector2.Transform(new Vector2(1, 1), m);
        Assert.Equal(new Vector2(2, 3), p);
    }

    [Fact]
    public void Rotate_90_Degrees_Maps_X_To_Y()
    {
        var m = SvgTransformParser.Parse("rotate(90)");
        var p = Vector2.Transform(new Vector2(1, 0), m);
        Assert.Equal(0f, p.X, 4);
        Assert.Equal(1f, p.Y, 4);
    }

    [Fact]
    public void Rotate_With_Center_Pivots_Around_Point()
    {
        // Rotate 180 around (5,0) takes (10,0) to (0,0).
        var m = SvgTransformParser.Parse("rotate(180, 5, 0)");
        var p = Vector2.Transform(new Vector2(10, 0), m);
        Assert.Equal(0f, p.X, 3);
        Assert.Equal(0f, p.Y, 3);
    }

    [Fact]
    public void SkewX_Skews_X_Based_On_Y()
    {
        var m = SvgTransformParser.Parse("skewX(45)");
        var p = Vector2.Transform(new Vector2(0, 1), m);
        Assert.Equal(1f, p.X, 4);
        Assert.Equal(1f, p.Y, 4);
    }

    [Fact]
    public void SkewY_Skews_Y_Based_On_X()
    {
        var m = SvgTransformParser.Parse("skewY(45)");
        var p = Vector2.Transform(new Vector2(1, 0), m);
        Assert.Equal(1f, p.X, 4);
        Assert.Equal(1f, p.Y, 4);
    }

    [Fact]
    public void Matrix_Direct_Form()
    {
        var m = SvgTransformParser.Parse("matrix(1 0 0 1 10 20)");
        var p = Vector2.Transform(new Vector2(1, 1), m);
        Assert.Equal(new Vector2(11, 21), p);
    }

    [Fact]
    public void Composite_Right_To_Left_Application()
    {
        // SVG: translate(100, 0) scale(2)
        // Applied right-to-left: scale first, then translate.
        var m = SvgTransformParser.Parse("translate(100, 0) scale(2)");
        var p = Vector2.Transform(new Vector2(1, 0), m);
        // scale: (2,0); translate: (102, 0).
        Assert.Equal(102f, p.X, 3);
        Assert.Equal(0f, p.Y, 3);
    }
}
