using System.Globalization;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Xpm;

/// <summary>
/// Reader for X11 XPM3 pixmap files. XPM stores image data as C source
/// code; this reader parses the body of the embedded string array.
/// </summary>
public sealed class XpmReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly string[] _lines;
    private readonly int _hotspotX, _hotspotY;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Xpm;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    private XpmReader(Stream s, bool owns, string[] lines, int hx, int hy,
                      ImageInfo info, ImageMetadata meta)
    {
        _stream = s; _ownsStream = owns; _lines = lines;
        _hotspotX = hx; _hotspotY = hy;
        Info = info; Metadata = meta;
    }

    /// <summary>Open an XPM file by path.</summary>
    public static XpmReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an XPM from a stream.</summary>
    public static XpmReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var reader = new StreamReader(stream, Encoding.UTF8, true, 1024, leaveOpen: !ownsStream);
        var src = reader.ReadToEnd();
        var lines = new List<string>(256);
        int i = 0;
        while (i < src.Length)
        {
            int q = src.IndexOf('"', i);
            if (q < 0) break;
            int q2 = src.IndexOf('"', q + 1);
            if (q2 < 0) break;
            lines.Add(src[(q + 1)..q2]);
            i = q2 + 1;
        }
        if (lines.Count < 2) throw new ImageFormatException("Not an XPM (no string array found).");

        var hdr = lines[0].Split([' ', '\t'], StringSplitOptions.RemoveEmptyEntries);
        if (hdr.Length < 4) throw new ImageFormatException("Bad XPM header line: " + lines[0]);
        int width = int.Parse(hdr[0], CultureInfo.InvariantCulture);
        int height = int.Parse(hdr[1], CultureInfo.InvariantCulture);
        int colors = int.Parse(hdr[2], CultureInfo.InvariantCulture);
        int charsPerPixel = int.Parse(hdr[3], CultureInfo.InvariantCulture);
        int hx = hdr.Length >= 6 ? int.Parse(hdr[4], CultureInfo.InvariantCulture) : 0;
        int hy = hdr.Length >= 6 ? int.Parse(hdr[5], CultureInfo.InvariantCulture) : 0;

        if (lines.Count < 1 + colors + height)
        {
            throw new ImageFormatException("XPM truncated: header expects " + (1 + colors + height) + " lines, found " + lines.Count);
        }

        // Store the trimmed line list (header + color table + pixel data).
        var trimmed = new string[1 + colors + height];
        for (int k = 0; k < trimmed.Length; k++) trimmed[k] = lines[k];

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = 32,
            ChannelCount = 4,
            PixelFormat = PixelFormat.Rgba32,
            Format = ImageFormat.Xpm,
            HasAlpha = true,
            FrameCount = 1,
        };

        var meta = new ImageMetadata();
        _ = charsPerPixel;
        return new XpmReader(stream, ownsStream, trimmed, hx, hy, info, meta);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();

        var hdr = _lines[0].Split([' ', '\t'], StringSplitOptions.RemoveEmptyEntries);
        int width = int.Parse(hdr[0], CultureInfo.InvariantCulture);
        int height = int.Parse(hdr[1], CultureInfo.InvariantCulture);
        int colors = int.Parse(hdr[2], CultureInfo.InvariantCulture);
        int cpp = int.Parse(hdr[3], CultureInfo.InvariantCulture);

        var palette = new Dictionary<string, uint>(colors, StringComparer.Ordinal);
        for (int c = 0; c < colors; c++)
        {
            string line = _lines[1 + c];
            if (line.Length < cpp + 2) continue;
            string key = line[..cpp];
            var rest = line[cpp..];
            // Find color value following a 'c ' or 'm ' type token. Pick 'c' first.
            string? value = ExtractColor(rest, "c") ?? ExtractColor(rest, "g") ?? ExtractColor(rest, "g4") ?? ExtractColor(rest, "m");
            uint rgba = ParseColor(value);
            palette[key] = rgba;
        }

        int stride = width * 4;
        var (frame, buf) = ImageFrame.Rent(width, height, PixelFormat.Rgba32, stride);
        for (int y = 0; y < height; y++)
        {
            string line = _lines[1 + colors + y];
            int dstOff = y * stride;
            for (int x = 0; x < width; x++)
            {
                int o = x * cpp;
                if (o + cpp > line.Length) break;
                string key = line.Substring(o, cpp);
                uint c = palette.TryGetValue(key, out var v) ? v : 0;
                buf[dstOff + x * 4 + 0] = (byte)(c & 0xFF);
                buf[dstOff + x * 4 + 1] = (byte)((c >> 8) & 0xFF);
                buf[dstOff + x * 4 + 2] = (byte)((c >> 16) & 0xFF);
                buf[dstOff + x * 4 + 3] = (byte)((c >> 24) & 0xFF);
            }
        }
        yield return frame;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static string? ExtractColor(string s, string token)
    {
        int idx = 0;
        var parts = s.Split([' ', '\t'], StringSplitOptions.RemoveEmptyEntries);
        for (; idx < parts.Length - 1; idx++)
        {
            if (parts[idx] == token) return parts[idx + 1];
        }
        return null;
    }

    private static uint ParseColor(string? text)
    {
        if (string.IsNullOrEmpty(text)) return 0;
        if (text.Equals("None", StringComparison.OrdinalIgnoreCase)) return 0;
        if (text.StartsWith('#'))
        {
            string hex = text[1..];
            byte r = 0, g = 0, b = 0;
            switch (hex.Length)
            {
                case 6:
                    r = byte.Parse(hex.AsSpan(0, 2), NumberStyles.HexNumber);
                    g = byte.Parse(hex.AsSpan(2, 2), NumberStyles.HexNumber);
                    b = byte.Parse(hex.AsSpan(4, 2), NumberStyles.HexNumber);
                    break;
                case 12:
                    r = byte.Parse(hex.AsSpan(0, 2), NumberStyles.HexNumber);
                    g = byte.Parse(hex.AsSpan(4, 2), NumberStyles.HexNumber);
                    b = byte.Parse(hex.AsSpan(8, 2), NumberStyles.HexNumber);
                    break;
                case 3:
                    r = (byte)(byte.Parse(hex.AsSpan(0, 1), NumberStyles.HexNumber) * 0x11);
                    g = (byte)(byte.Parse(hex.AsSpan(1, 1), NumberStyles.HexNumber) * 0x11);
                    b = (byte)(byte.Parse(hex.AsSpan(2, 1), NumberStyles.HexNumber) * 0x11);
                    break;
                default:
                    return 0;
            }
            return 0xFF000000u | ((uint)b << 16) | ((uint)g << 8) | r;
        }
        // Named colors: support a tiny subset – black, white, red, green, blue.
        return text.ToLowerInvariant() switch
        {
            "black" => 0xFF000000u,
            "white" => 0xFFFFFFFFu,
            "red" => 0xFF0000FFu,
            "green" => 0xFF00FF00u,
            "blue" => 0xFFFF0000u,
            "transparent" or "none" => 0,
            _ => 0xFF808080u,
        };
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}
