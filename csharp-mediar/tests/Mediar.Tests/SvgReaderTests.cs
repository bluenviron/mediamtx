using System.IO.Compression;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Vector;
using Xunit;

namespace Mediar.Tests;

public class SvgReaderTests
{
    [Fact]
    public void Parses_Plain_Svg_With_Width_Height()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="200" height="120"><rect/></svg>""";
        var bytes = Encoding.UTF8.GetBytes(xml);
        using var r = SvgReader.Open(new MemoryStream(bytes), ImageFormat.Svgz, ownsStream: true);

        Assert.Equal(200, r.Info.Width);
        Assert.Equal(120, r.Info.Height);
        Assert.False(r.WasCompressed);
        Assert.Contains("<svg", r.SvgXml);
    }

    [Fact]
    public void Parses_Svgz_Compressed_Source()
    {
        var xml = """<svg width="50" height="40"></svg>""";
        using var ms = new MemoryStream();
        using (var gz = new GZipStream(ms, CompressionLevel.Optimal, leaveOpen: true))
        {
            var bytes = Encoding.UTF8.GetBytes(xml);
            gz.Write(bytes);
        }
        ms.Position = 0;

        using var r = SvgReader.Open(ms, ImageFormat.Svgz, ownsStream: true);
        Assert.True(r.WasCompressed);
        Assert.Equal(50, r.Info.Width);
        Assert.Equal(40, r.Info.Height);
    }

    [Fact]
    public void Falls_Back_To_ViewBox_When_No_Width_Height()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 200"></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        Assert.Equal(300, r.Info.Width);
        Assert.Equal(200, r.Info.Height);
    }

    [Fact]
    public void Resolves_Unit_Suffixes()
    {
        // 1in = 96px
        var xml = """<svg width="2in" height="1in"></svg>""";
        using var r = SvgReader.Open(new MemoryStream(Encoding.UTF8.GetBytes(xml)), ImageFormat.Svgz, ownsStream: true);
        Assert.Equal(192, r.Info.Width);
        Assert.Equal(96, r.Info.Height);
    }
}
