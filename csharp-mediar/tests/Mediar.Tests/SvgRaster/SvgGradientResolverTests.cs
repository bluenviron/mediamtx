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
}
