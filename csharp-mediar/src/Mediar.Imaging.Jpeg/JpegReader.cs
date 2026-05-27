using System.Collections.Frozen;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Reader for JPEG (JFIF / EXIF) files, including the multi-image MPO
/// container and digital-camera THM thumbnails (which are simply JPEGs).
/// </summary>
/// <remarks>
/// The reader parses every JPEG marker segment up to <c>SOS</c>, then
/// stops. Image dimensions, sampling factors and any embedded EXIF /
/// XMP metadata are exposed via <see cref="ImageInfo"/> and
/// <see cref="ImageMetadata"/>. Pixel decoding is performed by
/// <see cref="JpegBaselineDecoder"/>.
/// </remarks>
public sealed class JpegReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly JpegFrame _frame;
    private readonly byte[] _scanData;
    private readonly ImageMetadata _metadata;
    private readonly ImageFormat _format;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _format;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => _metadata;

    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>
    /// Returns the EXIF / TIFF tag dictionary verbatim (key prefixes:
    /// <c>IFD0:</c>, <c>Exif:</c>, <c>GPS:</c>).
    /// </summary>
    public IReadOnlyDictionary<string, string> ExifTags => _metadata.Tags;

    private JpegReader(
        Stream stream, bool ownsStream, JpegFrame frame, byte[] scanData,
        ImageMetadata metadata, ImageFormat format, ImageInfo info)
    {
        _stream = stream;
        _ownsStream = ownsStream;
        _frame = frame;
        _scanData = scanData;
        _metadata = metadata;
        _format = format;
        Info = info;
    }

    /// <summary>Open a JPEG file by path.</summary>
    public static JpegReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            var fmt = ImageFormat.Jpeg;
            if (path.EndsWith(".thm", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.Thm;
            else if (path.EndsWith(".mpo", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.Mpo;
            else if (path.EndsWith(".jfif", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.Jfif;
            else if (path.EndsWith(".jpg_large", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.JpgLarge;
            return Open(fs, fmt, ownsStream: true);
        }
        catch
        {
            fs.Dispose();
            throw;
        }
    }

    /// <summary>Open a JPEG from a stream.</summary>
    public static JpegReader Open(
        Stream stream, ImageFormat format = ImageFormat.Jpeg, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        Span<byte> two = stackalloc byte[2];
        ReadExactly(stream, two);
        if (two[0] != 0xFF || two[1] != 0xD8)
        {
            throw new ImageFormatException("Not a JPEG file (missing SOI marker).");
        }

        ImageMetadata metadata = ImageMetadata.Empty;
        var frame = new JpegFrame();
        byte[] scanBytes = [];
        Span<byte> lengthBuf = stackalloc byte[2];

        // Walk marker segments until SOS or EOI.
        while (true)
        {
            byte ff;
            do
            {
                int r = stream.ReadByte();
                if (r < 0) throw new ImageFormatException("Truncated JPEG (looking for marker).");
                ff = (byte)r;
            } while (ff != 0xFF);

            byte marker;
            do
            {
                int r = stream.ReadByte();
                if (r < 0) throw new ImageFormatException("Truncated JPEG (looking for marker).");
                marker = (byte)r;
            } while (marker == 0xFF);

            if (marker == 0xD9) break; // EOI
            if (marker == 0xD8) continue; // duplicate SOI
            if (marker is >= 0xD0 and <= 0xD7) continue; // restart

            Span<byte> _length = lengthBuf;
            ReadExactly(stream, _length);
            int segLen = (_length[0] << 8) | _length[1];
            if (segLen < 2) throw new ImageFormatException("Bad JPEG segment length.");
            byte[] segment = new byte[segLen - 2];
            if (segment.Length > 0) stream.ReadExactly(segment);

            switch (marker)
            {
                case 0xC0: // SOF0 baseline
                case 0xC1: // SOF1 extended sequential
                case 0xC2: // SOF2 progressive
                case 0xC3: // SOF3 lossless
                    {
                        ParseSof(segment, frame);
                        frame.IsBaseline = marker == 0xC0;
                        break;
                    }
                case 0xE0: // APP0 (JFIF)
                    break;
                case 0xE1: // APP1 (EXIF / XMP)
                    if (segment.Length > 6 &&
                        segment[0] == (byte)'E' && segment[1] == (byte)'x' &&
                        segment[2] == (byte)'i' && segment[3] == (byte)'f' &&
                        segment[4] == 0x00 && segment[5] == 0x00)
                    {
                        metadata = ExifParser.Parse(segment.AsSpan(6));
                    }
                    break;
                case 0xE2: // APP2 (MPO multi-image / ICC profile)
                    if (segment.Length > 4 &&
                        segment[0] == (byte)'M' && segment[1] == (byte)'P' &&
                        segment[2] == (byte)'F' && segment[3] == 0x00)
                    {
                        format = ImageFormat.Mpo;
                    }
                    break;
                case 0xDA: // SOS — scan begins; everything after this to EOI is entropy-coded.
                    {
                        // Skip the SOS payload bytes (already in `segment`) – the
                        // entropy-coded data follows immediately.
                        scanBytes = ReadRestOfStreamUntilEoi(stream);
                        goto done;
                    }
                default:
                    break;
            }
        }
    done:

        var pf = frame.NumberOfComponents == 1 ? PixelFormat.Gray8 : PixelFormat.Rgb24;
        var info = new ImageInfo
        {
            Width = frame.Width,
            Height = frame.Height,
            BitsPerPixel = frame.BitsPerSample * frame.NumberOfComponents,
            ChannelCount = frame.NumberOfComponents,
            PixelFormat = pf,
            Format = format,
            HasAlpha = false,
            HorizontalDpi = ParseDouble(metadata.Tags, "IFD0:XResolution"),
            VerticalDpi = ParseDouble(metadata.Tags, "IFD0:YResolution"),
            FrameCount = 1,
        };

        return new JpegReader(stream, ownsStream, frame, scanBytes, metadata, format, info);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        if (!_frame.IsBaseline)
        {
            throw new NotSupportedException(
                "JPEG progressive / lossless / arithmetic coded decoding is not implemented in this version of Mediar.");
        }
        var frame = JpegBaselineDecoder.Decode(_frame, _scanData);
        await Task.CompletedTask.ConfigureAwait(false);
        yield return frame;
    }

    private static void ParseSof(ReadOnlySpan<byte> seg, JpegFrame frame)
    {
        frame.BitsPerSample = seg[0];
        frame.Height = (seg[1] << 8) | seg[2];
        frame.Width = (seg[3] << 8) | seg[4];
        int nf = seg[5];
        frame.NumberOfComponents = nf;
        frame.Components = new JpegComponent[nf];
        int p = 6;
        for (int i = 0; i < nf; i++)
        {
            byte id = seg[p];
            byte sampling = seg[p + 1];
            byte qtab = seg[p + 2];
            frame.Components[i] = new JpegComponent
            {
                Id = id,
                HSampling = sampling >> 4,
                VSampling = sampling & 0x0F,
                QuantTableId = qtab,
            };
            p += 3;
        }
    }

    private static byte[] ReadRestOfStreamUntilEoi(Stream s)
    {
        using var ms = new MemoryStream();
        int b;
        while ((b = s.ReadByte()) >= 0)
        {
            ms.WriteByte((byte)b);
        }
        // Trim trailing FF D9 (EOI) if present.
        var arr = ms.ToArray();
        if (arr.Length >= 2 && arr[^2] == 0xFF && arr[^1] == 0xD9)
        {
            return arr[..^2];
        }
        return arr;
    }

    private static double ParseDouble(FrozenDictionary<string, string> tags, string key)
    {
        if (!tags.TryGetValue(key, out var s)) return 0;
        int slash = s.IndexOf('/');
        if (slash > 0 &&
            double.TryParse(s.AsSpan(0, slash), System.Globalization.CultureInfo.InvariantCulture, out var n) &&
            double.TryParse(s.AsSpan(slash + 1), System.Globalization.CultureInfo.InvariantCulture, out var d) &&
            d != 0)
        {
            return n / d;
        }
        return 0;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void ReadExactly(Stream s, Span<byte> dst)
    {
        int read = 0;
        while (read < dst.Length)
        {
            int n = s.Read(dst[read..]);
            if (n <= 0) throw new EndOfStreamException();
            read += n;
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}

/// <summary>JPEG SOFn frame parameters (internal).</summary>
internal sealed class JpegFrame
{
    public bool IsBaseline { get; set; }
    public int Width { get; set; }
    public int Height { get; set; }
    public int BitsPerSample { get; set; } = 8;
    public int NumberOfComponents { get; set; }
    public JpegComponent[] Components { get; set; } = [];
}

/// <summary>Per-component JPEG metadata (internal).</summary>
internal sealed class JpegComponent
{
    public byte Id { get; set; }
    public int HSampling { get; set; } = 1;
    public int VSampling { get; set; } = 1;
    public byte QuantTableId { get; set; }
}
