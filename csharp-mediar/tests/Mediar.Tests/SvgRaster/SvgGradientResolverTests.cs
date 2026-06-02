using System.Xml.Linq;
using Mediar.Codecs.SvgRaster;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgGradientResolverTests
{
    private static XElement Doc(string xml) =>
        XDocument.Parse($"<svg xmlns=\"http://www.w3.org/2000/svg\">{xml}</svg>").Root!;

    [Fact]
    public void Linear_Gradient_With_Stops()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g1" x1="0" y1="0" x2="1" y2="0">
                <stop offset="0" stop-color="red"/>
                <stop offset="1" stop-color="blue"/>
              </linearGradient>
            </defs>
            """);
        var resolver = new SvgGradientResolver(root);
        var paint = resolver.Resolve("g1");
        var linear = Assert.IsType<LinearGradientPaint>(paint);
        Assert.Equal(2, linear.Stops.Count);
        Assert.Equal(0f, linear.X1);
        Assert.Equal(1f, linear.X2);
    }

    [Fact]
    public void Radial_Gradient_Defaults()
    {
        var root = Doc("""
            <defs>
              <radialGradient id="r1">
                <stop offset="0" stop-color="white"/>
                <stop offset="1" stop-color="black"/>
              </radialGradient>
            </defs>
            """);
        var resolver = new SvgGradientResolver(root);
        var paint = resolver.Resolve("r1");
        var radial = Assert.IsType<RadialGradientPaint>(paint);
        Assert.Equal(0.5f, radial.Cx);
        Assert.Equal(0.5f, radial.Cy);
        Assert.Equal(0.5f, radial.R);
    }

    [Fact]
    public void Stop_Offset_Percentage()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop offset="50%" stop-color="red"/>
                <stop offset="100%" stop-color="blue"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(0.5f, linear.Stops[0].Offset);
        Assert.Equal(1f, linear.Stops[1].Offset);
    }

    [Fact]
    public void Stop_Opacity_Multiplies_Alpha()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop offset="0" stop-color="red" stop-opacity="0.5"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(0.5f, linear.Stops[0].Color.A, 2);
    }

    [Fact]
    public void Href_Chain_Inherits_Stops()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="src">
                <stop offset="0" stop-color="red"/>
                <stop offset="1" stop-color="blue"/>
              </linearGradient>
              <linearGradient id="child" href="#src" x1="10" x2="20"/>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("child")!;
        Assert.Equal(2, linear.Stops.Count);
        Assert.Equal(10f, linear.X1);
        Assert.Equal(20f, linear.X2);
    }

    [Fact]
    public void GradientUnits_UserSpaceOnUse()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g" gradientUnits="userSpaceOnUse">
                <stop offset="0" stop-color="red"/>
                <stop offset="1" stop-color="blue"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(GradientUnits.UserSpaceOnUse, linear.Units);
    }

    [Fact]
    public void SpreadMethod_Reflect()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g" spreadMethod="reflect">
                <stop offset="0" stop-color="red"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(GradientSpread.Reflect, linear.Spread);
    }

    [Fact]
    public void Unknown_Id_Returns_Null()
    {
        var root = Doc("<defs/>");
        Assert.Null(new SvgGradientResolver(root).Resolve("missing"));
    }

    [Fact]
    public void Non_Gradient_Element_Returns_Null()
    {
        var root = Doc("<rect id=\"r1\"/>");
        Assert.Null(new SvgGradientResolver(root).Resolve("r1"));
    }

    [Fact]
    public void Null_Root_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new SvgGradientResolver(null!));
    }

    [Fact]
    public void Stop_Color_Defaults_To_Black()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop offset="0"/>
                <stop offset="1"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(0f, linear.Stops[0].Color.R);
        Assert.Equal(0f, linear.Stops[0].Color.G);
        Assert.Equal(0f, linear.Stops[0].Color.B);
        Assert.Equal(1f, linear.Stops[0].Color.A);
    }

    [Fact]
    public void Stop_Offset_Clamped_To_Unit_Interval()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop offset="-0.5" stop-color="red"/>
                <stop offset="200%" stop-color="blue"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(0f, linear.Stops[0].Offset);
        Assert.Equal(1f, linear.Stops[1].Offset);
    }

    [Fact]
    public void Stop_With_Inline_Style()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop offset="0" style="stop-color:red; stop-opacity:0.25"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(1f, linear.Stops[0].Color.R);
        Assert.Equal(0.25f, linear.Stops[0].Color.A, 2);
    }

    [Fact]
    public void SpreadMethod_Repeat()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g" spreadMethod="repeat">
                <stop offset="0" stop-color="red"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(GradientSpread.Repeat, linear.Spread);
    }

    [Fact]
    public void SpreadMethod_Pad_Is_Default()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop offset="0" stop-color="red"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(GradientSpread.Pad, linear.Spread);
    }

    [Fact]
    public void Linear_Without_Stops_Returns_Null()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g"/>
            </defs>
            """);
        Assert.Null(new SvgGradientResolver(root).Resolve("g"));
    }

    [Fact]
    public void Radial_Without_Stops_Returns_Null()
    {
        var root = Doc("""
            <defs>
              <radialGradient id="r"/>
            </defs>
            """);
        Assert.Null(new SvgGradientResolver(root).Resolve("r"));
    }

    [Fact]
    public void Radial_Custom_Focal()
    {
        var root = Doc("""
            <defs>
              <radialGradient id="r" cx="0.3" cy="0.4" r="0.6" fx="0.1" fy="0.2">
                <stop offset="0" stop-color="white"/>
              </radialGradient>
            </defs>
            """);
        var radial = (RadialGradientPaint)new SvgGradientResolver(root).Resolve("r")!;
        Assert.Equal(0.3f, radial.Cx);
        Assert.Equal(0.4f, radial.Cy);
        Assert.Equal(0.6f, radial.R);
        Assert.Equal(0.1f, radial.Fx);
        Assert.Equal(0.2f, radial.Fy);
    }

    [Fact]
    public void Radial_Focal_Defaults_To_Center()
    {
        var root = Doc("""
            <defs>
              <radialGradient id="r" cx="0.7" cy="0.8">
                <stop offset="0" stop-color="white"/>
              </radialGradient>
            </defs>
            """);
        var radial = (RadialGradientPaint)new SvgGradientResolver(root).Resolve("r")!;
        Assert.Equal(0.7f, radial.Fx);
        Assert.Equal(0.8f, radial.Fy);
    }

    [Fact]
    public void Href_Cycle_Is_Broken_By_Visited_Set()
    {
        // a -> b -> a — cycle must terminate without stack overflow.
        var root = Doc("""
            <defs>
              <linearGradient id="a" href="#b">
                <stop offset="0" stop-color="red"/>
              </linearGradient>
              <linearGradient id="b" href="#a"/>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("a")!;
        Assert.Single(linear.Stops);
    }

    [Fact]
    public void Href_To_Missing_Id_Falls_Back_To_Local_Stops()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g" href="#missing">
                <stop offset="0" stop-color="red"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Single(linear.Stops);
    }

    [Fact]
    public void Stop_Offset_Without_Attribute_Defaults_To_Zero()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop stop-color="red"/>
                <stop offset="1" stop-color="blue"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(0f, linear.Stops[0].Offset);
    }

    [Fact]
    public void GradientUnits_Defaults_To_ObjectBoundingBox()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g">
                <stop offset="0" stop-color="red"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(GradientUnits.ObjectBoundingBox, linear.Units);
    }

    [Fact]
    public void Percentage_Linear_Coords_Parsed()
    {
        var root = Doc("""
            <defs>
              <linearGradient id="g" x1="0%" x2="100%">
                <stop offset="0" stop-color="red"/>
              </linearGradient>
            </defs>
            """);
        var linear = (LinearGradientPaint)new SvgGradientResolver(root).Resolve("g")!;
        Assert.Equal(0f, linear.X1);
        Assert.Equal(1f, linear.X2);
    }
}
