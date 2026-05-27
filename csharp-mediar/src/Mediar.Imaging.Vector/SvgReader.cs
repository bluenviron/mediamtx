using System.IO.Compression;
using System.Text;
using System.Text.RegularExpressions;
using Mediar.Codecs.SvgRaster;
using Mediar.Vector;

namespace Mediar.Imaging.Vector;

/// <summary>
/// Reader for plain SVG and gzip-compressed SVGZ files. The SVG XML
/// source is exposed verbatim via <see cref="SvgXml"/>; canvas
/// dimensions are parsed from the root <c>&lt;svg&gt;</c> element's
/// <c>width</c> / <c>height</c> / <c>viewBox</c> attributes using the
/// SVG 96-DPI unit convention. Calls to
/// <see cref="ReadFramesAsync"/> rasterize the document via the
/// <see cref="Mediar.Codecs.SvgRaster.SvgRenderer"/> codec engine into
/// a Bgra32 frame.
/// </summary>
public sealed partial class SvgReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format { get; }
    /// <inheritdoc/>
    public ImageInfo Info { get; }
    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;
    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    /// <summary>The raw SVG XML source.</summary>
    public string SvgXml { get; }

    /// <summary>True if the source was gzip-compressed (.svgz).</summary>
    public bool WasCompressed { get; }

    private SvgReader(Stream s, bool owns, ImageFormat fmt, ImageInfo info, string svg, bool compressed)
    {
        _stream = s; _ownsStream = owns;
        Format = fmt; Info = info; SvgXml = svg; WasCompressed = compressed;
    }

    /// <summary>Open an SVG or SVGZ file from a path.</summary>
    public static SvgReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ImageFormatExtensions.FromExtension(path), ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an SVG or SVGZ file from a stream.</summary>
    public static SvgReader Open(Stream stream, ImageFormat expected = ImageFormat.Svgz, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] b = ms.ToArray();

        bool compressed = b.Length >= 2 && b[0] == 0x1F && b[1] == 0x8B;
        string xml;
        if (compressed)
        {
            using var src = new MemoryStream(b);
            using var gz = new GZipStream(src, CompressionMode.Decompress);
            using var sr = new MemoryStream();
            gz.CopyTo(sr);
            xml = Encoding.UTF8.GetString(sr.ToArray());
        }
        else
        {
            xml = Encoding.UTF8.GetString(b);
        }

        var (w, h) = ExtractDimensions(xml);
        var info = new ImageInfo
        {
            Width = w,
            Height = h,
            Format = expected,
            FrameCount = 1,
        };
        return new SvgReader(stream, ownsStream, expected, info, xml, compressed);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        var frame = await Task.Run(() => Info.Width > 0 && Info.Height > 0
            ? SvgRenderer.Render(SvgXml, Info.Width, Info.Height, RgbaColor.Transparent)
            : SvgRenderer.Render(SvgXml, RgbaColor.Transparent), cancellationToken).ConfigureAwait(false);
        yield return frame;
    }

    /// <summary>
    /// Render the SVG at a custom output resolution. Convenience wrapper
    /// around <see cref="SvgRenderer.Render(string, int, int, RgbaColor)"/>
    /// that doesn't require the caller to take a dependency on
    /// Mediar.Vector.
    /// </summary>
    public ImageFrame RenderAt(int width, int height) =>
        SvgRenderer.Render(SvgXml, width, height, RgbaColor.Transparent);

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    [GeneratedRegex(@"<svg\b[^>]*?\bwidth\s*=\s*[""']([^""']+)[""']", RegexOptions.IgnoreCase | RegexOptions.Singleline)]
    private static partial Regex SvgWidthRegex();

    [GeneratedRegex(@"<svg\b[^>]*?\bheight\s*=\s*[""']([^""']+)[""']", RegexOptions.IgnoreCase | RegexOptions.Singleline)]
    private static partial Regex SvgHeightRegex();

    [GeneratedRegex(@"<svg\b[^>]*?\bviewBox\s*=\s*[""']\s*[-\d.]+\s+[-\d.]+\s+([-\d.]+)\s+([-\d.]+)", RegexOptions.IgnoreCase | RegexOptions.Singleline)]
    private static partial Regex SvgViewBoxRegex();

    private static (int W, int H) ExtractDimensions(string xml)
    {
        double w = 0, h = 0;
        var mw = SvgWidthRegex().Match(xml);
        var mh = SvgHeightRegex().Match(xml);
        if (mw.Success) w = ParseLength(mw.Groups[1].Value);
        if (mh.Success) h = ParseLength(mh.Groups[1].Value);
        if (w == 0 || h == 0)
        {
            var vb = SvgViewBoxRegex().Match(xml);
            if (vb.Success)
            {
                if (w == 0) double.TryParse(vb.Groups[1].Value, System.Globalization.CultureInfo.InvariantCulture, out w);
                if (h == 0) double.TryParse(vb.Groups[2].Value, System.Globalization.CultureInfo.InvariantCulture, out h);
            }
        }
        return ((int)Math.Round(w), (int)Math.Round(h));
    }

    private static double ParseLength(string s)
    {
        s = s.Trim();
        double factor = 1.0;
        if (s.EndsWith("px", StringComparison.OrdinalIgnoreCase)) { factor = 1.0; s = s[..^2]; }
        else if (s.EndsWith("pt", StringComparison.OrdinalIgnoreCase)) { factor = 96.0 / 72.0; s = s[..^2]; }
        else if (s.EndsWith("pc", StringComparison.OrdinalIgnoreCase)) { factor = 16.0; s = s[..^2]; }
        else if (s.EndsWith("mm", StringComparison.OrdinalIgnoreCase)) { factor = 96.0 / 25.4; s = s[..^2]; }
        else if (s.EndsWith("cm", StringComparison.OrdinalIgnoreCase)) { factor = 96.0 / 2.54; s = s[..^2]; }
        else if (s.EndsWith("in", StringComparison.OrdinalIgnoreCase)) { factor = 96.0; s = s[..^2]; }
        else if (s.EndsWith('%')) return 0;
        return double.TryParse(s.Trim(), System.Globalization.NumberStyles.Float,
                               System.Globalization.CultureInfo.InvariantCulture, out double v) ? v * factor : 0;
    }
}

