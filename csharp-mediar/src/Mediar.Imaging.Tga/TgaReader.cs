using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Tga;

/// <summary>
/// Reader for Truevision TGA / TARGA files. Supports image types 1, 2,
/// 3, 9, 10, and 11 (uncompressed and RLE-compressed indexed, RGB and
/// grayscale). Also parses the TGA 2.0 footer ("TRUEVISION-XFILE.") and
/// extension area for author / comments.
/// </summary>
public sealed class TgaReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Tga;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    private TgaReader(Stream s, bool owns, byte[] b, ImageInfo info, ImageMetadata meta, bool canDecode)
    {
        _stream = s; _ownsStream = owns; _bytes = b;
        Info = info; Metadata = meta; CanDecodePixels = canDecode;
    }

    /// <summary>Open a TGA file by path.</summary>
    public static TgaReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a TGA from a stream.</summary>
    public static TgaReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 18) throw new ImageFormatException("Truncated TGA.");

        byte idLength = bytes[0];
        byte colorMapType = bytes[1];
        byte imageType = bytes[2];
        int firstEntryIndex = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(3));
        int colorMapLength = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(5));
        byte colorMapDepth = bytes[7];
        int width = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(12));
        int height = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(14));
        byte pixelDepth = bytes[16];
        _ = firstEntryIndex; _ = colorMapDepth;

        if (width <= 0 || height <= 0) throw new ImageFormatException("Bad TGA dimensions.");

        bool indexed = imageType is 1 or 9;
        bool grayscale = imageType is 3 or 11;
        bool rgb = imageType is 2 or 10;
        bool supported = indexed || grayscale || rgb;

        var pf = grayscale ? PixelFormat.Gray8
            : indexed ? PixelFormat.Indexed8
            : pixelDepth switch
            {
                24 => PixelFormat.Bgr24,
                32 => PixelFormat.Bgra32,
                _ => PixelFormat.Unknown,
            };

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = pixelDepth,
            ChannelCount = pf.ChannelCount(),
            PixelFormat = pf,
            Format = ImageFormat.Tga,
            HasAlpha = pixelDepth == 32 && !indexed && !grayscale,
            FrameCount = 1,
        };

        var meta = TryParseFooter(bytes);
        return new TgaReader(stream, ownsStream, bytes, info, meta, supported);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        if (!CanDecodePixels) throw new NotSupportedException("Unsupported TGA image type.");

        byte idLength = _bytes[0];
        byte imageType = _bytes[2];
        int colorMapLength = BinaryPrimitives.ReadUInt16LittleEndian(_bytes.AsSpan(5));
        byte colorMapDepth = _bytes[7];
        int width = Info.Width;
        int height = Info.Height;
        byte pixelDepth = _bytes[16];
        byte descriptor = _bytes[17];
        bool topDown = (descriptor & 0x20) != 0;

        int p = 18 + idLength;
        ReadOnlyMemory<uint> palette = default;
        if (colorMapLength > 0)
        {
            var pal = new uint[colorMapLength];
            int bytesPerEntry = colorMapDepth / 8;
            for (int i = 0; i < colorMapLength; i++)
            {
                byte b = _bytes[p];
                byte g = _bytes[p + 1];
                byte r = _bytes[p + 2];
                byte a = (byte)(bytesPerEntry == 4 ? _bytes[p + 3] : 255);
                pal[i] = (uint)((a << 24) | (r << 16) | (g << 8) | b);
                p += bytesPerEntry;
            }
            palette = pal;
        }

        var pf = Info.PixelFormat;
        int bpp = pixelDepth / 8;
        int stride = width * (pf == PixelFormat.Indexed8 ? 1 : pf == PixelFormat.Gray8 ? 1 : bpp);
        var (frame, buf) = ImageFrame.Rent(width, height, pf, stride, palette);

        bool rle = imageType is 9 or 10 or 11;
        if (!rle)
        {
            int len = stride * height;
            if (p + len > _bytes.Length) throw new ImageFormatException("Truncated TGA pixel block.");
            for (int y = 0; y < height; y++)
            {
                int dstY = topDown ? y : height - 1 - y;
                Buffer.BlockCopy(_bytes, p + y * stride, buf, dstY * stride, stride);
            }
            yield return frame;
            yield break;
        }

        // RLE decode.
        var output = new byte[stride * height];
        int outPos = 0;
        while (outPos < output.Length && p < _bytes.Length)
        {
            byte hdr = _bytes[p++];
            int count = (hdr & 0x7F) + 1;
            if ((hdr & 0x80) != 0)
            {
                // run packet
                if (p + bpp > _bytes.Length) break;
                for (int i = 0; i < count && outPos + bpp <= output.Length; i++, outPos += bpp)
                {
                    Buffer.BlockCopy(_bytes, p, output, outPos, bpp);
                }
                p += bpp;
            }
            else
            {
                int need = count * bpp;
                if (p + need > _bytes.Length) break;
                Buffer.BlockCopy(_bytes, p, output, outPos, Math.Min(need, output.Length - outPos));
                outPos += need;
                p += need;
            }
        }
        for (int y = 0; y < height; y++)
        {
            int dstY = topDown ? y : height - 1 - y;
            Buffer.BlockCopy(output, y * stride, buf, dstY * stride, stride);
        }
        yield return frame;
    }

    private static ImageMetadata TryParseFooter(byte[] b)
    {
        if (b.Length < 26) return ImageMetadata.Empty;
        var footer = b.AsSpan(b.Length - 26);
        if (footer.Length >= 18 &&
            Encoding.ASCII.GetString(footer[8..25]) == "TRUEVISION-XFILE.")
        {
            uint extOff = BinaryPrimitives.ReadUInt32LittleEndian(footer);
            if (extOff > 0 && extOff + 495 <= b.Length)
            {
                var ext = b.AsSpan((int)extOff);
                string author = Trim(Encoding.ASCII.GetString(ext.Slice(2, 41)));
                string comments = Trim(Encoding.ASCII.GetString(ext.Slice(43, 324)));
                string software = Trim(Encoding.ASCII.GetString(ext.Slice(426, 41)));
                var tags = new Dictionary<string, string>(StringComparer.Ordinal);
                if (author.Length > 0) tags["TGA:Author"] = author;
                if (comments.Length > 0) tags["TGA:Comments"] = comments;
                if (software.Length > 0) tags["TGA:Software"] = software;
                return new ImageMetadata
                {
                    Author = author.Length > 0 ? author : null,
                    Description = comments.Length > 0 ? comments : null,
                    Software = software.Length > 0 ? software : null,
                    Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
                };
            }
        }
        return ImageMetadata.Empty;
    }

    private static string Trim(string s)
    {
        int i = s.IndexOf('\0');
        if (i >= 0) s = s[..i];
        return s.TrimEnd();
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}
