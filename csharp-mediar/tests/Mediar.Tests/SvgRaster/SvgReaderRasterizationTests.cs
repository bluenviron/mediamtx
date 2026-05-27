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
}
