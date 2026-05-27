using System.Globalization;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Pnm;

/// <summary>
/// Reader for Netpbm portable bitmap / graymap / pixmap files:
/// P1 (PBM-ASCII), P2 (PGM-ASCII), P3 (PPM-ASCII),
/// P4 (PBM-raw), P5 (PGM-raw), P6 (PPM-raw).
/// </summary>
public sealed class PnmReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly int _pixelsStart;
    private readonly int _magic;
    private readonly int _maxVal;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format { get; }

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;

    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    private PnmReader(Stream s, bool owns, byte[] b, int start, int magic, int maxVal,
                      ImageInfo info, ImageFormat fmt)
    {
        _stream = s; _ownsStream = owns; _bytes = b;
        _pixelsStart = start; _magic = magic; _maxVal = maxVal;
        Info = info; Format = fmt;
    }

    /// <summary>Open a Netpbm file by path.</summary>
    public static PnmReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            var format = path.EndsWith(".pbm", StringComparison.OrdinalIgnoreCase) ? ImageFormat.Pbm
                : path.EndsWith(".pgm", StringComparison.OrdinalIgnoreCase) ? ImageFormat.Pgm
                : path.EndsWith(".ppm", StringComparison.OrdinalIgnoreCase) ? ImageFormat.Ppm
                : ImageFormat.Pnm;
            return Open(fs, format, ownsStream: true);
        }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a Netpbm stream.</summary>
    public static PnmReader Open(Stream stream, ImageFormat format = ImageFormat.Pnm, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 4 || bytes[0] != (byte)'P') throw new ImageFormatException("Not a Netpbm file.");
        int magic = bytes[1] - '0';
        if (magic is < 1 or > 6) throw new ImageFormatException("Unsupported Netpbm magic P" + magic);
        int p = 2;
        int width = ReadInt(bytes, ref p);
        int height = ReadInt(bytes, ref p);
        int maxVal = magic is 1 or 4 ? 1 : ReadInt(bytes, ref p);
        // Single whitespace after maxVal before binary data.
        if (magic >= 4 && p < bytes.Length && IsWhitespace(bytes[p])) p++;

        var pf = magic is 1 or 4 ? PixelFormat.Indexed1
            : magic is 2 or 5 ? (maxVal > 255 ? PixelFormat.Gray16 : PixelFormat.Gray8)
            : (maxVal > 255 ? PixelFormat.Rgb48 : PixelFormat.Rgb24);
        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = pf.BitsPerPixel(),
            ChannelCount = pf.ChannelCount(),
            PixelFormat = pf,
            Format = format == ImageFormat.Pnm ? magic switch
            {
                1 or 4 => ImageFormat.Pbm,
                2 or 5 => ImageFormat.Pgm,
                _ => ImageFormat.Ppm,
            } : format,
            FrameCount = 1,
        };
        return new PnmReader(stream, ownsStream, bytes, p, magic, maxVal, info, info.Format);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        int width = Info.Width, height = Info.Height;
        var pf = Info.PixelFormat;
        int stride = pf switch
        {
            PixelFormat.Indexed1 => (width + 7) / 8,
            PixelFormat.Gray8 => width,
            PixelFormat.Gray16 => width * 2,
            PixelFormat.Rgb24 => width * 3,
            PixelFormat.Rgb48 => width * 6,
            _ => 0,
        };
        var (frame, buf) = ImageFrame.Rent(width, height, pf, stride);

        int p = _pixelsStart;
        switch (_magic)
        {
            case 1: // PBM ASCII
                for (int y = 0; y < height; y++)
                {
                    int rowStart = y * stride;
                    for (int x = 0; x < width; x++)
                    {
                        int v = ReadInt(_bytes, ref p);
                        if (v != 0) buf[rowStart + x / 8] |= (byte)(0x80 >> (x % 8));
                    }
                }
                break;
            case 4: // PBM raw – stored MSB-first per row, padded to byte.
                {
                    int rowBytes = (width + 7) / 8;
                    for (int y = 0; y < height; y++)
                    {
                        if (p + rowBytes > _bytes.Length) throw new ImageFormatException("Truncated PBM.");
                        Buffer.BlockCopy(_bytes, p, buf, y * stride, rowBytes);
                        p += rowBytes;
                    }
                }
                break;
            case 2: // PGM ASCII
                if (pf == PixelFormat.Gray8)
                {
                    for (int i = 0; i < width * height; i++)
                    {
                        int v = ReadInt(_bytes, ref p);
                        buf[i] = (byte)Math.Clamp(v * 255 / _maxVal, 0, 255);
                    }
                }
                else
                {
                    for (int i = 0; i < width * height; i++)
                    {
                        int v = ReadInt(_bytes, ref p);
                        ushort u = (ushort)Math.Clamp((long)v * 65535 / _maxVal, 0, 65535);
                        buf[i * 2 + 0] = (byte)(u & 0xFF);
                        buf[i * 2 + 1] = (byte)(u >> 8);
                    }
                }
                break;
            case 5: // PGM raw
                if (pf == PixelFormat.Gray8)
                {
                    if (_maxVal == 255 && p + width * height <= _bytes.Length)
                    {
                        Buffer.BlockCopy(_bytes, p, buf, 0, width * height);
                    }
                    else
                    {
                        for (int i = 0; i < width * height; i++)
                        {
                            int v = _bytes[p++];
                            buf[i] = (byte)Math.Clamp(v * 255 / _maxVal, 0, 255);
                        }
                    }
                }
                else
                {
                    for (int i = 0; i < width * height; i++)
                    {
                        int hi = _bytes[p++];
                        int lo = _bytes[p++];
                        ushort u = (ushort)((hi << 8) | lo);
                        ushort scaled = (ushort)Math.Clamp((long)u * 65535 / _maxVal, 0, 65535);
                        buf[i * 2 + 0] = (byte)(scaled & 0xFF);
                        buf[i * 2 + 1] = (byte)(scaled >> 8);
                    }
                }
                break;
            case 3: // PPM ASCII
                if (pf == PixelFormat.Rgb24)
                {
                    for (int i = 0; i < width * height * 3; i++)
                    {
                        int v = ReadInt(_bytes, ref p);
                        buf[i] = (byte)Math.Clamp(v * 255 / _maxVal, 0, 255);
                    }
                }
                else
                {
                    for (int i = 0; i < width * height * 3; i++)
                    {
                        int v = ReadInt(_bytes, ref p);
                        ushort u = (ushort)Math.Clamp((long)v * 65535 / _maxVal, 0, 65535);
                        buf[i * 2 + 0] = (byte)(u & 0xFF);
                        buf[i * 2 + 1] = (byte)(u >> 8);
                    }
                }
                break;
            case 6: // PPM raw
                if (pf == PixelFormat.Rgb24)
                {
                    int nb = width * height * 3;
                    if (_maxVal == 255 && p + nb <= _bytes.Length)
                    {
                        Buffer.BlockCopy(_bytes, p, buf, 0, nb);
                    }
                    else
                    {
                        for (int i = 0; i < nb; i++)
                        {
                            int v = _bytes[p++];
                            buf[i] = (byte)Math.Clamp(v * 255 / _maxVal, 0, 255);
                        }
                    }
                }
                else
                {
                    int nb = width * height * 3;
                    for (int i = 0; i < nb; i++)
                    {
                        int hi = _bytes[p++];
                        int lo = _bytes[p++];
                        ushort u = (ushort)((hi << 8) | lo);
                        ushort scaled = (ushort)Math.Clamp((long)u * 65535 / _maxVal, 0, 65535);
                        buf[i * 2 + 0] = (byte)(scaled & 0xFF);
                        buf[i * 2 + 1] = (byte)(scaled >> 8);
                    }
                }
                break;
        }
        yield return frame;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static bool IsWhitespace(byte b) => b is (byte)' ' or (byte)'\t' or (byte)'\n' or (byte)'\r';

    private static int ReadInt(byte[] b, ref int p)
    {
        // Skip whitespace + comments.
        while (p < b.Length)
        {
            if (b[p] == (byte)'#')
            {
                while (p < b.Length && b[p] != (byte)'\n') p++;
            }
            else if (IsWhitespace(b[p])) p++;
            else break;
        }
        int start = p;
        while (p < b.Length && !IsWhitespace(b[p])) p++;
        if (start == p) throw new ImageFormatException("Unexpected EOF in PNM header.");
        return int.Parse(Encoding.ASCII.GetString(b, start, p - start), CultureInfo.InvariantCulture);
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}
