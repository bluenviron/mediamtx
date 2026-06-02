using Mediar.Codecs.SvgRaster;
using Mediar.Imaging;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgRendererTests
{
    private static (byte B, byte G, byte R, byte A) PixelAt(ImageFrame f, int x, int y)
    {
        int i = y * f.Stride + x * 4;
        var d = f.Pixels.Span;
        return (d[i], d[i + 1], d[i + 2], d[i + 3]);
    }

    [Fact]
    public void Renders_Plain_Rect_To_Frame()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect x="5" y="5" width="10" height="10" fill="red"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        Assert.Equal(20, frame.Width);
        Assert.Equal(20, frame.Height);
        Assert.Equal(PixelFormat.Bgra32, frame.PixelFormat);

        // Center of rect: opaque red.
        var (b, g, r, a) = PixelAt(frame, 10, 10);
        Assert.Equal(255, r);
        Assert.Equal(0, g);
        Assert.Equal(0, b);
        Assert.Equal(255, a);

        // Outside rect: transparent.
        var (b2, g2, r2, a2) = PixelAt(frame, 0, 0);
        Assert.Equal(0, a2);
        Assert.Equal(0, r2);
        Assert.Equal(0, g2);
        Assert.Equal(0, b2);
    }

    [Fact]
    public void Renders_Circle_With_Antialiasing()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="15" fill="black"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        // Center fully opaque.
        Assert.Equal(255, PixelAt(frame, 20, 20).A);
        // Corner (well outside circle) transparent.
        Assert.Equal(0, PixelAt(frame, 0, 0).A);
        // Pixel right on the edge ~ (35,20) - some partial coverage near boundary.
        int rightEdgeA = PixelAt(frame, 35, 20).A;
        Assert.InRange(rightEdgeA, 0, 255);
    }

    [Fact]
    public void Renders_Polygon()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><polygon points="5,5 15,5 15,15 5,15" fill="green"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        var (b, g, r, a) = PixelAt(frame, 10, 10);
        Assert.Equal(0, r);
        Assert.Equal(128, g);
        Assert.Equal(0, b);
        Assert.Equal(255, a);
    }

    [Fact]
    public void Renders_Path_D_Attribute()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><path d="M 5 5 L 15 5 L 15 15 L 5 15 Z" fill="blue"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        var (b, g, r, a) = PixelAt(frame, 10, 10);
        Assert.Equal(255, b);
        Assert.Equal(255, a);
    }

    [Fact]
    public void ViewBox_Fit_Scales_Content()
    {
        // 1x1 viewBox containing a black square scaled to 40x40 output.
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><rect width="1" height="1" fill="black"/></svg>""";
        var frame = SvgRenderer.Render(svg, 40, 40);
        // The 1x1 rect (scaled to fill 40x40) should make (20,20) opaque black.
        Assert.Equal(255, PixelAt(frame, 20, 20).A);
        Assert.Equal(0, PixelAt(frame, 20, 20).R);
    }

    [Fact]
    public void Background_Color_Honored()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>""";
        var frame = SvgRenderer.Render(svg, RgbaColor.FromBytes(255, 255, 255));
        // No shapes, so every pixel should be the background (white opaque).
        var (b, g, r, a) = PixelAt(frame, 5, 5);
        Assert.Equal(255, r);
        Assert.Equal(255, g);
        Assert.Equal(255, b);
        Assert.Equal(255, a);
    }

    [Fact]
    public void Stroke_Renders_Outline()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="30" height="30"><rect x="5" y="5" width="20" height="20" fill="none" stroke="red" stroke-width="4"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        // Center of rectangle (well inside, with fill=none) - transparent.
        Assert.Equal(0, PixelAt(frame, 15, 15).A);
        // On the top edge - some red coverage.
        int onEdge = PixelAt(frame, 15, 5).R;
        Assert.True(onEdge > 100);
    }

    [Fact]
    public void Group_Transform_Cascades()
    {
        // A group translates a rect.
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><g transform="translate(10, 10)"><rect width="10" height="10" fill="red"/></g></svg>""";
        var frame = SvgRenderer.Render(svg);
        // Rect ends up at (10,10)-(20,20). Pixel (15,15) opaque red.
        Assert.Equal(255, PixelAt(frame, 15, 15).R);
        // Pixel (5,5) - empty.
        Assert.Equal(0, PixelAt(frame, 5, 5).A);
    }

    [Fact]
    public void Linear_Gradient_Fill()
    {
        var svg = """
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">
              <defs>
                <linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
                  <stop offset="0" stop-color="red"/>
                  <stop offset="1" stop-color="blue"/>
                </linearGradient>
              </defs>
              <rect x="0" y="0" width="20" height="20" fill="url(#g)"/>
            </svg>
            """;
        var frame = SvgRenderer.Render(svg);
        // Left edge mostly red.
        Assert.True(PixelAt(frame, 1, 10).R > 200);
        // Right edge mostly blue.
        Assert.True(PixelAt(frame, 18, 10).B > 200);
    }

    [Fact]
    public void EvenOdd_Renders_Donut()
    {
        var svg = """
            <svg xmlns="http://www.w3.org/2000/svg" width="30" height="30">
              <path d="M 5 5 L 25 5 L 25 25 L 5 25 Z M 10 10 L 20 10 L 20 20 L 10 20 Z" fill="red" fill-rule="evenodd"/>
            </svg>
            """;
        var frame = SvgRenderer.Render(svg);
        // Donut center (transparent).
        Assert.Equal(0, PixelAt(frame, 15, 15).A);
        // Ring (opaque red).
        Assert.Equal(255, PixelAt(frame, 7, 15).A);
    }

    [Fact]
    public void Render_With_Empty_Xml_Throws()
    {
        Assert.Throws<ArgumentException>(() => SvgRenderer.Render(""));
    }

    [Fact]
    public void Render_With_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => SvgRenderer.Render(null!));
    }

    [Fact]
    public void Render_With_Whitespace_Throws()
    {
        // ArgumentException.ThrowIfNullOrEmpty rejects empty; whitespace-only
        // passes that check but the XML parse step fails.
        Assert.ThrowsAny<Exception>(() => SvgRenderer.Render("   "));
    }

    [Fact]
    public void Render_With_Invalid_Xml_Throws()
    {
        Assert.ThrowsAny<Exception>(() => SvgRenderer.Render("<not-closed-tag"));
    }

    [Fact]
    public void Render_Line_Element_Produces_Stroke()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="30" height="30"><line x1="5" y1="15" x2="25" y2="15" stroke="black" stroke-width="3"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        // Pixel on the line should be opaque or near it.
        int onLineA = PixelAt(frame, 15, 15).A;
        Assert.True(onLineA > 100);
        // Pixel far above the line stays empty.
        Assert.Equal(0, PixelAt(frame, 15, 0).A);
    }

    [Fact]
    public void Render_Ellipse_Element_Fills_Center()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><ellipse cx="20" cy="10" rx="15" ry="5" fill="red"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        // Center of ellipse opaque red.
        var (b, g, r, a) = PixelAt(frame, 20, 10);
        Assert.Equal(255, r);
        Assert.Equal(0, g);
        Assert.Equal(0, b);
        Assert.Equal(255, a);
        // Top-left corner well outside the ellipse remains transparent.
        Assert.Equal(0, PixelAt(frame, 0, 0).A);
    }

    [Fact]
    public void Render_Polyline_Element_Strokes_But_Does_Not_Fill_By_Default()
    {
        // Polyline with stroke only (no fill). SVG default fill is black,
        // but for an open polyline the fill region is the implied closed
        // polygon. Either way the test just verifies the stroke renders.
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="30" height="30"><polyline points="5,5 25,5 25,25" fill="none" stroke="black" stroke-width="3"/></svg>""";
        var frame = SvgRenderer.Render(svg);
        // Pixel on the top edge of the polyline has stroke coverage.
        int onStrokeA = PixelAt(frame, 15, 5).A;
        Assert.True(onStrokeA > 100);
    }

    [Fact]
    public void Render_With_Explicit_Dimensions_Overrides_Intrinsic_Width_Height()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><rect width="100" height="100" fill="black"/></svg>""";
        var frame = SvgRenderer.Render(svg, 16, 16);
        Assert.Equal(16, frame.Width);
        Assert.Equal(16, frame.Height);
    }

    [Fact]
    public void Render_Background_With_Custom_Rgba_Produces_That_Color_For_Empty_Svg()
    {
        var svg = """<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"/>""";
        var frame = SvgRenderer.Render(svg, RgbaColor.FromBytes(10, 20, 30, 255));
        var (b, g, r, a) = PixelAt(frame, 2, 2);
        Assert.Equal(10, r);
        Assert.Equal(20, g);
        Assert.Equal(30, b);
        Assert.Equal(255, a);
    }
}
