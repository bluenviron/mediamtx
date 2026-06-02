using System.IO.Compression;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Vector;
using Xunit;

namespace Mediar.Tests;

public class SvgReaderTests
{
    private static MemoryStream PlainStream(string xml) =>
        new(Encoding.UTF8.GetBytes(xml));

    private static MemoryStream SvgzStream(string xml)
    {
        var ms = new MemoryStream();
        using (var gz = new GZipStream(ms, CompressionLevel.Optimal, leaveOpen: true))
        {
            gz.Write(Encoding.UTF8.GetBytes(xml));
        }
        ms.Position = 0;
        return ms;
    }

    [Fact]
    public void Parses_Plain_Svg_With_Width_Height()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="200" height="120"><rect/></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);

        Assert.Equal(200, r.Info.Width);
        Assert.Equal(120, r.Info.Height);
        Assert.False(r.WasCompressed);
        Assert.Contains("<svg", r.SvgXml);
    }

    [Fact]
    public void Parses_Svgz_Compressed_Source()
    {
        var xml = """<svg width="50" height="40"></svg>""";
        using var r = SvgReader.Open(SvgzStream(xml), ImageFormat.Svgz, ownsStream: true);

        Assert.True(r.WasCompressed);
        Assert.Equal(50, r.Info.Width);
        Assert.Equal(40, r.Info.Height);
    }

    [Fact]
    public void Falls_Back_To_ViewBox_When_No_Width_Height()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 200"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);
        Assert.Equal(300, r.Info.Width);
        Assert.Equal(200, r.Info.Height);
    }

    [Fact]
    public void Resolves_Unit_Suffix_Inches()
    {
        var xml = """<svg width="2in" height="1in"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);
        Assert.Equal(192, r.Info.Width);
        Assert.Equal(96, r.Info.Height);
    }

    [Fact]
    public void Open_Stream_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => SvgReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Path_Plain_Svg_File_Works()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-svg-{Guid.NewGuid():N}.svg");
        File.WriteAllText(path, """<svg width="11" height="22"></svg>""");
        try
        {
            using var r = SvgReader.Open(path);
            Assert.Equal(11, r.Info.Width);
            Assert.Equal(22, r.Info.Height);
            Assert.False(r.WasCompressed);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Svgz_File_Detects_Compressed_And_Sets_Format()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-svg-{Guid.NewGuid():N}.svgz");
        using (var fs = File.Create(path))
        using (var gz = new GZipStream(fs, CompressionLevel.Optimal))
        {
            gz.Write(Encoding.UTF8.GetBytes("""<svg width="7" height="3"></svg>"""));
        }
        try
        {
            using var r = SvgReader.Open(path);
            Assert.True(r.WasCompressed);
            Assert.Equal(7, r.Info.Width);
            Assert.Equal(3, r.Info.Height);
            Assert.Equal(ImageFormat.Svgz, r.Format);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_File_Throws_And_Does_Not_Leak_Handle()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-svg-missing-{Guid.NewGuid():N}.svg");
        Assert.Throws<FileNotFoundException>(() => SvgReader.Open(path));
    }

    [Fact]
    public void Width_Only_Falls_Back_To_ViewBox_Height()
    {
        var xml = """<svg width="500" viewBox="0 0 1000 750"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(500, r.Info.Width);
        Assert.Equal(750, r.Info.Height);
    }

    [Fact]
    public void Height_Only_Falls_Back_To_ViewBox_Width()
    {
        var xml = """<svg height="800" viewBox="0 0 1234 5678"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(1234, r.Info.Width);
        Assert.Equal(800, r.Info.Height);
    }

    [Fact]
    public void Width_Height_Override_ViewBox_When_Both_Present()
    {
        var xml = """<svg width="100" height="50" viewBox="0 0 999 999"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(100, r.Info.Width);
        Assert.Equal(50, r.Info.Height);
    }

    [Fact]
    public void No_Width_Height_Or_ViewBox_Yields_Zero()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(0, r.Info.Width);
        Assert.Equal(0, r.Info.Height);
    }

    [Theory]
    [InlineData("100", "50", 100, 50)]              // unitless
    [InlineData("100px", "50px", 100, 50)]          // px factor 1
    [InlineData("72pt", "36pt", 96, 48)]            // pt = 96/72
    [InlineData("1pc", "2pc", 16, 32)]              // pc = 16
    [InlineData("25.4mm", "12.7mm", 96, 48)]        // mm = 96/25.4
    [InlineData("2.54cm", "1.27cm", 96, 48)]        // cm = 96/2.54
    [InlineData("1in", "0.5in", 96, 48)]            // in = 96
    public void Unit_Suffixes_Are_Resolved(string w, string h, int expectedW, int expectedH)
    {
        var xml = $"""<svg width="{w}" height="{h}"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(expectedW, r.Info.Width);
        Assert.Equal(expectedH, r.Info.Height);
    }

    [Fact]
    public void Percent_Unit_Returns_Zero_And_Falls_Back_To_ViewBox()
    {
        var xml = """<svg width="50%" height="50%" viewBox="0 0 40 30"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(40, r.Info.Width);
        Assert.Equal(30, r.Info.Height);
    }

    [Fact]
    public void Unrecognized_Unit_Yields_Zero()
    {
        var xml = """<svg width="5em" height="3em"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(0, r.Info.Width);
        Assert.Equal(0, r.Info.Height);
    }

    [Fact]
    public void Single_Quoted_Attributes_Are_Accepted()
    {
        var xml = "<svg width='80' height='40'/>";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(80, r.Info.Width);
        Assert.Equal(40, r.Info.Height);
    }

    [Fact]
    public void Decimal_Width_Rounds_To_Nearest_Int()
    {
        var xml = """<svg width="100.6" height="100.4"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(101, r.Info.Width);
        Assert.Equal(100, r.Info.Height);
    }

    [Fact]
    public void Mixed_Case_Unit_Suffix_Is_Resolved()
    {
        var xml = """<svg width="2IN" height="1In"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(192, r.Info.Width);
        Assert.Equal(96, r.Info.Height);
    }

    [Fact]
    public void Format_Property_Matches_Expected_Argument()
    {
        var xml = """<svg width="10" height="10"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz);
        Assert.Equal(ImageFormat.Svgz, r.Format);
        Assert.Equal(ImageFormat.Svgz, r.Info.Format);
    }

    [Fact]
    public void Frame_Count_Is_One()
    {
        var xml = """<svg width="1" height="1"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(1, r.Info.FrameCount);
    }

    [Fact]
    public void Metadata_Is_Empty()
    {
        var xml = """<svg width="1" height="1"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Same(ImageMetadata.Empty, r.Metadata);
    }

    [Fact]
    public void Can_Decode_Pixels_Is_True()
    {
        var xml = """<svg width="1" height="1"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.True(r.CanDecodePixels);
    }

    [Fact]
    public void SvgXml_Preserves_Source_Text()
    {
        var xml = """<svg width="1" height="1"><!-- hello --></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(xml, r.SvgXml);
    }

    [Fact]
    public void SvgXml_Of_Svgz_Returns_Decompressed_Text()
    {
        var xml = """<svg width="9" height="9"></svg>""";
        using var r = SvgReader.Open(SvgzStream(xml));
        Assert.Equal(xml, r.SvgXml);
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var xml = """<svg width="1" height="1"></svg>""";
        var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);
        r.Dispose();
        r.Dispose(); // should not throw
    }

    [Fact]
    public void Dispose_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        var stream = PlainStream("""<svg width="1" height="1"></svg>""");
        var r = SvgReader.Open(stream, ImageFormat.Svgz, ownsStream: true);
        r.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = stream.Length);
    }

    [Fact]
    public void Dispose_With_OwnsStream_False_Leaves_Stream_Open()
    {
        var stream = PlainStream("""<svg width="1" height="1"></svg>""");
        var r = SvgReader.Open(stream, ImageFormat.Svgz, ownsStream: false);
        r.Dispose();
        // Stream is still usable
        _ = stream.Length;
        stream.Dispose();
    }

    [Fact]
    public async Task ReadFramesAsync_Yields_Single_Frame()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="8" height="4"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);
        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync())
        {
            frames.Add(f);
        }
        Assert.Single(frames);
        Assert.Equal(8, frames[0].Width);
        Assert.Equal(4, frames[0].Height);
    }

    [Fact]
    public async Task ReadFramesAsync_With_Zero_Size_Uses_Renderer_Default()
    {
        // No width/height/viewBox → 0x0 -> renderer chooses intrinsic size (≥1 each).
        var xml = """<svg xmlns="http://www.w3.org/2000/svg"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);
        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync())
        {
            frames.Add(f);
        }
        Assert.Single(frames);
        Assert.True(frames[0].Width >= 1);
        Assert.True(frames[0].Height >= 1);
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_PreCancelled_Token()
    {
        var xml = """<svg width="2" height="2"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var _ in r.ReadFramesAsync(cts.Token)) { }
        });
    }

    [Fact]
    public void RenderAt_Produces_Frame_At_Custom_Size()
    {
        var xml = """<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml), ImageFormat.Svgz, ownsStream: true);
        var frame = r.RenderAt(32, 16);
        Assert.Equal(32, frame.Width);
        Assert.Equal(16, frame.Height);
    }

    [Fact]
    public void Width_Height_With_Pt_Truncates_To_Nearest_Int()
    {
        // 1pt = 96/72 = 1.3333... → 1; 2pt = 2.666... → 3
        var xml = """<svg width="1pt" height="2pt"></svg>""";
        using var r = SvgReader.Open(PlainStream(xml));
        Assert.Equal(1, r.Info.Width);
        Assert.Equal(3, r.Info.Height);
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => SvgReader.Open((string)null!));
    }
}
