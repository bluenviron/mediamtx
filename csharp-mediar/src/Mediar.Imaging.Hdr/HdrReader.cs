using System.Collections.Frozen;
using System.Globalization;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Hdr;

/// <summary>
/// Reader for Greg Ward's Radiance / RGBE (.hdr / .pic) high-dynamic-range
/// image format. Each pixel is decoded to 96-bit floating-point RGB
/// (<see cref="PixelFormat.Rgb96Float"/>).
/// </summary>
public sealed class HdrReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly int _pixelsStart;
    private readonly bool _yFlipped;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Hdr;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    private HdrReader(Stream s, bool owns, byte[] b, int pixelsStart, bool yFlipped,
                      ImageInfo info, ImageMetadata meta)
    {
        _stream = s; _ownsStream = owns; _bytes = b;
        _pixelsStart = pixelsStart; _yFlipped = yFlipped;
        Info = info; Metadata = meta;
    }

    /// <summary>Open a Radiance HDR file by path.</summary>
    public static HdrReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a Radiance HDR from a stream.</summary>
    public static HdrReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8 ||
            !(bytes[0] == '#' && bytes[1] == '?' &&
              (Match(bytes, "#?RADIANCE") || Match(bytes, "#?RGBE"))))
        {
            throw new ImageFormatException("Not a Radiance HDR file.");
        }

        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        int p = 0;
        while (p < bytes.Length)
        {
            int eol = FindEol(bytes, p);
            if (eol < 0) throw new ImageFormatException("Truncated HDR header.");
            string line = Encoding.ASCII.GetString(bytes, p, eol - p);
            p = eol + 1;
            if (line.Length == 0) break;
            int eq = line.IndexOf('=');
            if (eq > 0)
            {
                tags["HDR:" + line[..eq]] = line[(eq + 1)..];
            }
        }

        int dimEol = FindEol(bytes, p);
        if (dimEol < 0) throw new ImageFormatException("Missing HDR resolution line.");
        string dimLine = Encoding.ASCII.GetString(bytes, p, dimEol - p);
        var parts = dimLine.Split(' ');
        if (parts.Length != 4) throw new ImageFormatException("Bad HDR resolution line: " + dimLine);
        bool yMajor = parts[0].StartsWith('-') || parts[0].StartsWith('+');
        bool yFlipped = parts[0].StartsWith("-Y", StringComparison.Ordinal);
        int height = int.Parse(parts[1], CultureInfo.InvariantCulture);
        int width = int.Parse(parts[3], CultureInfo.InvariantCulture);
        if (!yMajor)
        {
            // X-major (rare) – treat dim parts as width × height transposed.
            (width, height) = (height, width);
        }

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = 96,
            ChannelCount = 3,
            PixelFormat = PixelFormat.Rgb96Float,
            Format = ImageFormat.Hdr,
            IsHdr = true,
            FrameCount = 1,
        };

        var meta = new ImageMetadata
        {
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };

        return new HdrReader(stream, ownsStream, bytes, dimEol + 1, yFlipped, info, meta);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        int width = Info.Width;
        int height = Info.Height;
        int stride = width * 3 * sizeof(float);
        var (frame, buf) = ImageFrame.Rent(width, height, PixelFormat.Rgb96Float, stride);

        int p = _pixelsStart;
        var rowRgbe = new byte[width * 4];
        for (int y = 0; y < height; y++)
        {
            DecodeScanline(_bytes, ref p, rowRgbe, width);
            int dstY = _yFlipped ? y : height - 1 - y;
            int dstOff = dstY * stride;
            for (int x = 0; x < width; x++)
            {
                byte r = rowRgbe[x * 4 + 0];
                byte g = rowRgbe[x * 4 + 1];
                byte b = rowRgbe[x * 4 + 2];
                byte e = rowRgbe[x * 4 + 3];
                float fr, fg, fb;
                if (e == 0)
                {
                    fr = fg = fb = 0;
                }
                else
                {
                    float scale = MathF.Pow(2f, e - 128) / 256f;
                    fr = r * scale;
                    fg = g * scale;
                    fb = b * scale;
                }
                int o = dstOff + x * 12;
                Unsafe.WriteUnaligned(ref buf[o + 0], fr);
                Unsafe.WriteUnaligned(ref buf[o + 4], fg);
                Unsafe.WriteUnaligned(ref buf[o + 8], fb);
            }
        }
        yield return frame;
    }

    private static void DecodeScanline(byte[] src, ref int p, byte[] dst, int width)
    {
        if (width < 8 || width > 0x7FFF || p + 4 > src.Length ||
            src[p] != 2 || src[p + 1] != 2 || (src[p + 2] & 0x80) != 0)
        {
            // Old-style or short scanline.
            for (int x = 0; x < width && p + 4 <= src.Length; x++)
            {
                dst[x * 4 + 0] = src[p++];
                dst[x * 4 + 1] = src[p++];
                dst[x * 4 + 2] = src[p++];
                dst[x * 4 + 3] = src[p++];
            }
            return;
        }
        int scanlineWidth = (src[p + 2] << 8) | src[p + 3];
        p += 4;
        if (scanlineWidth != width) throw new ImageFormatException("HDR scanline width mismatch.");

        for (int ch = 0; ch < 4; ch++)
        {
            int x = 0;
            while (x < width && p < src.Length)
            {
                byte n = src[p++];
                if (n > 128)
                {
                    int count = n & 0x7F;
                    if (p >= src.Length) break;
                    byte v = src[p++];
                    for (int k = 0; k < count && x < width; k++, x++) dst[x * 4 + ch] = v;
                }
                else
                {
                    int count = n;
                    if (p + count > src.Length) break;
                    for (int k = 0; k < count && x < width; k++, x++) dst[x * 4 + ch] = src[p++];
                }
            }
        }
    }

    private static int FindEol(byte[] b, int p)
    {
        while (p < b.Length && b[p] != (byte)'\n') p++;
        return p < b.Length ? p : -1;
    }

    private static bool Match(byte[] b, string s)
    {
        if (b.Length < s.Length) return false;
        for (int i = 0; i < s.Length; i++) if (b[i] != s[i]) return false;
        return true;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}
