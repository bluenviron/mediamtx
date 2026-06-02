using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Vector;
using Xunit;

namespace Mediar.Tests.SvgRaster;

public class SvgReaderRasterizationTests
{
    [Fact]
    public async Task ReadFramesAsync_Now_Produces_Frame()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="red"/></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        Assert.True(r.CanDecodePixels);
        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync())
            frames.Add(f);
        Assert.Single(frames);
        Assert.Equal(10, frames[0].Width);
        Assert.Equal(10, frames[0].Height);
        Assert.Equal(PixelFormat.Bgra32, frames[0].PixelFormat);
        // Centre pixel must be opaque red.
        int i = 5 * frames[0].Stride + 5 * 4;
        var d = frames[0].Pixels.Span;
        Assert.Equal(255, d[i + 2]); // R
        Assert.Equal(0, d[i + 1]);   // G
        Assert.Equal(0, d[i + 0]);   // B
        Assert.Equal(255, d[i + 3]); // A
    }

    [Fact]
    public void RenderAt_Custom_Resolution()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="green"/></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        var frame = r.RenderAt(50, 50);
        Assert.Equal(50, frame.Width);
        Assert.Equal(50, frame.Height);
        // Centre - green.
        int i = 25 * frame.Stride + 25 * 4;
        var d = frame.Pixels.Span;
        Assert.Equal(128, d[i + 1]);
    }

    [Fact]
    public async Task Circle_Renders_Inside_Bounding_Box()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><circle cx="10" cy="10" r="8" fill="blue"/></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        var frame = await GetFirstAsync(r);
        // Centre pixel must be blue, corner must be background.
        AssertPixel(frame, 10, 10, expectedB: 255);
        AssertPixel(frame, 0, 0, expectedA: 0);
    }

    [Fact]
    public async Task ReadFramesAsync_Yields_Single_Frame_For_Static_Svg()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="5" height="5"><rect width="5" height="5" fill="black"/></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        int count = 0;
        await foreach (var f in r.ReadFramesAsync()) count++;
        Assert.Equal(1, count);
    }

    [Fact]
    public void RenderAt_Different_Sizes_Both_Honored()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="red"/></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        var small = r.RenderAt(20, 20);
        var big = r.RenderAt(100, 100);
        Assert.Equal(20, small.Width);
        Assert.Equal(100, big.Width);
    }

    [Fact]
    public async Task Polygon_Triangle_Fills_Center()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><polygon points="10,2 18,18 2,18" fill="lime"/></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        var frame = await GetFirstAsync(r);
        AssertPixel(frame, 10, 14, expectedG: 255);
    }

    [Fact]
    public void Multiple_Open_Calls_Yield_Independent_Readers()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="5" height="5"><rect width="5" height="5" fill="white"/></svg>""";
        byte[] bytes = Encoding.UTF8.GetBytes(xml);
        using var a = SvgReader.Open(new MemoryStream(bytes), ImageFormat.Svgz, ownsStream: true);
        using var b = SvgReader.Open(new MemoryStream(bytes), ImageFormat.Svgz, ownsStream: true);
        Assert.NotSame(a, b);
        Assert.True(a.CanDecodePixels);
        Assert.True(b.CanDecodePixels);
    }

    private static async Task<ImageFrame> GetFirstAsync(SvgReader r)
    {
        await foreach (var f in r.ReadFramesAsync()) return f;
        throw new InvalidOperationException("no frame");
    }

    private static void AssertPixel(ImageFrame frame, int x, int y, int expectedR = -1, int expectedG = -1, int expectedB = -1, int expectedA = -1)
    {
        int i = y * frame.Stride + x * 4;
        var d = frame.Pixels.Span;
        if (expectedB >= 0) Assert.InRange(d[i + 0], expectedB - 4, expectedB + 4);
        if (expectedG >= 0) Assert.InRange(d[i + 1], expectedG - 4, expectedG + 4);
        if (expectedR >= 0) Assert.InRange(d[i + 2], expectedR - 4, expectedR + 4);
        if (expectedA >= 0) Assert.Equal(expectedA, d[i + 3]);
    }
}
