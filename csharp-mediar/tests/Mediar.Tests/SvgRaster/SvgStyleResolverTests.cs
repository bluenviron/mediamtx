using System.Xml.Linq;
using Mediar.Codecs.SvgRaster;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgStyleResolverTests
{
    private static XElement El(string xml) => XElement.Parse(xml);

    [Fact]
    public void Default_Fill_Is_Black()
    {
        var s = SvgStyleResolver.Resolve(El("<rect/>"), new SvgStyle());
        Assert.IsType<SolidPaint>(s.Fill);
        var fill = (SolidPaint)s.Fill;
        Assert.Equal(0f, fill.Color.R);
        Assert.Equal(1f, fill.Color.A);
    }

    [Fact]
    public void Default_Stroke_Is_None()
    {
        var s = SvgStyleResolver.Resolve(El("<rect/>"), new SvgStyle());
        Assert.Equal(Paint.None, s.Stroke);
    }

    [Fact]
    public void Presentation_Attribute_Sets_Fill()
    {
        var s = SvgStyleResolver.Resolve(El("<rect fill=\"red\"/>"), new SvgStyle());
        var fill = Assert.IsType<SolidPaint>(s.Fill);
        Assert.Equal(1f, fill.Color.R);
        Assert.Equal(0f, fill.Color.G);
    }

    [Fact]
    public void Inline_Style_Beats_Presentation_Attribute()
    {
        var s = SvgStyleResolver.Resolve(El("<rect fill=\"red\" style=\"fill:blue\"/>"), new SvgStyle());
        var fill = (SolidPaint)s.Fill;
        Assert.Equal(1f, fill.Color.B);
        Assert.Equal(0f, fill.Color.R);
    }

    [Fact]
    public void Inherits_From_Parent_When_Unspecified()
    {
        var parent = new SvgStyle { Fill = new SolidPaint(RgbaColor.FromBytes(0, 255, 0)) };
        var s = SvgStyleResolver.Resolve(El("<g/>"), parent);
        var fill = (SolidPaint)s.Fill;
        Assert.Equal(1f, fill.Color.G);
    }

    [Fact]
    public void CurrentColor_Resolves_To_Color_Property()
    {
        var s = SvgStyleResolver.Resolve(El("<rect color=\"yellow\" fill=\"currentColor\"/>"), new SvgStyle());
        var fill = (SolidPaint)s.Fill;
        Assert.Equal(1f, fill.Color.R);
        Assert.Equal(1f, fill.Color.G);
        Assert.Equal(0f, fill.Color.B);
    }

    [Fact]
    public void Opacity_Cascades_Multiplicatively()
    {
        var parent = new SvgStyle { Opacity = 0.5f };
        var s = SvgStyleResolver.Resolve(El("<g opacity=\"0.5\"/>"), parent);
        Assert.Equal(0.25f, s.Opacity, 3);
    }

    [Fact]
    public void Fill_None_Maps_To_PaintNone()
    {
        var s = SvgStyleResolver.Resolve(El("<rect fill=\"none\"/>"), new SvgStyle());
        Assert.Equal(Paint.None, s.Fill);
    }

    [Fact]
    public void Stroke_Width_Cascades()
    {
        var s = SvgStyleResolver.Resolve(El("<rect stroke=\"black\" stroke-width=\"3\"/>"), new SvgStyle());
        Assert.Equal(3f, s.StrokeWidth);
    }

    [Fact]
    public void Stroke_LineCap_Round()
    {
        var s = SvgStyleResolver.Resolve(El("<rect stroke=\"black\" stroke-linecap=\"round\"/>"), new SvgStyle());
        Assert.Equal(LineCap.Round, s.StrokeLineCap);
    }

    [Fact]
    public void Stroke_LineJoin_Bevel()
    {
        var s = SvgStyleResolver.Resolve(El("<rect stroke=\"black\" stroke-linejoin=\"bevel\"/>"), new SvgStyle());
        Assert.Equal(LineJoin.Bevel, s.StrokeLineJoin);
    }

    [Fact]
    public void FillRule_EvenOdd()
    {
        var s = SvgStyleResolver.Resolve(El("<path fill-rule=\"evenodd\"/>"), new SvgStyle());
        Assert.Equal(FillRule.EvenOdd, s.FillRule);
    }

    [Fact]
    public void Display_None_Disables()
    {
        var s = SvgStyleResolver.Resolve(El("<g display=\"none\"/>"), new SvgStyle());
        Assert.False(s.Display);
    }

    [Fact]
    public void Visibility_Hidden_Disables()
    {
        var s = SvgStyleResolver.Resolve(El("<g visibility=\"hidden\"/>"), new SvgStyle());
        Assert.False(s.Visibility);
    }

    [Fact]
    public void Url_Paint_Reference_Resolved_Via_Callback()
    {
        var gradient = new LinearGradientPaint(0, 0, 1, 0, new[] { new GradientStop(0f, RgbaColor.White) });
        var s = SvgStyleResolver.Resolve(
            El("<rect fill=\"url(#g1)\"/>"),
            new SvgStyle(),
            id => id == "g1" ? gradient : null);
        Assert.Same(gradient, s.Fill);
    }

    [Fact]
    public void Resolve_Null_Element_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => SvgStyleResolver.Resolve(null!, new SvgStyle()));
    }

    [Fact]
    public void Resolve_Null_Parent_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => SvgStyleResolver.Resolve(El("<rect/>"), null!));
    }

    [Fact]
    public void Stroke_DashOffset_Cascades_From_Parent()
    {
        var parent = new SvgStyle { StrokeDashOffset = 7f };
        var s = SvgStyleResolver.Resolve(El("<g/>"), parent);
        Assert.Equal(7f, s.StrokeDashOffset);
    }

    [Fact]
    public void Stroke_MiterLimit_Override_Replaces_Parent()
    {
        var parent = new SvgStyle { StrokeMiterLimit = 4f };
        var s = SvgStyleResolver.Resolve(El("<g stroke-miterlimit=\"10\"/>"), parent);
        Assert.Equal(10f, s.StrokeMiterLimit);
    }

    [Fact]
    public void Stroke_DashArray_Parses_Comma_Separated_Values()
    {
        var s = SvgStyleResolver.Resolve(El("<rect stroke-dasharray=\"5, 3, 2\"/>"), new SvgStyle());
        Assert.NotNull(s.StrokeDashArray);
        Assert.Equal(3, s.StrokeDashArray!.Count);
        Assert.Equal(5f, s.StrokeDashArray[0]);
        Assert.Equal(3f, s.StrokeDashArray[1]);
        Assert.Equal(2f, s.StrokeDashArray[2]);
    }

    [Fact]
    public void Fill_Opacity_Independent_From_Cascading_Opacity()
    {
        var parent = new SvgStyle { Opacity = 0.5f };
        var s = SvgStyleResolver.Resolve(El("<rect fill-opacity=\"0.4\"/>"), parent);
        // Opacity inherits from parent (0.5) since unset on element.
        Assert.Equal(0.5f, s.Opacity, 3);
        // fill-opacity is element-local (does NOT inherit per SVG opacity rules).
        Assert.Equal(0.4f, s.FillOpacity, 3);
    }

    [Fact]
    public void Unresolved_Url_Falls_Back_To_Parent_Fill_Paint()
    {
        var parentFill = new SolidPaint(RgbaColor.FromBytes(0, 255, 0));
        var parent = new SvgStyle { Fill = parentFill };
        var s = SvgStyleResolver.Resolve(
            El("<rect fill=\"url(#missing)\"/>"),
            parent,
            _ => null);
        Assert.Same(parentFill, s.Fill);
    }

    [Fact]
    public void Inline_Style_With_Malformed_Declaration_Is_Skipped()
    {
        // The "garbage" entry (no colon) should be ignored; the valid one
        // ("fill:lime") should win.
        var s = SvgStyleResolver.Resolve(El("<rect style=\"garbage; fill:lime\"/>"), new SvgStyle());
        var fill = Assert.IsType<SolidPaint>(s.Fill);
        Assert.Equal(0f, fill.Color.R);
        Assert.Equal(1f, fill.Color.G);
        Assert.Equal(0f, fill.Color.B);
    }
}
