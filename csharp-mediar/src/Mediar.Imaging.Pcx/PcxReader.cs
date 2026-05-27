using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Pcx;

/// <summary>
/// Reader for ZSoft PCX raster images. Supports 1 / 4 / 8 bpp with
/// RLE compression and 1, 3, or 4 planes (mono, 16-color, 256-color
/// palette, 24-bit RGB).
/// </summary>
public sealed class PcxReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Pcx;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    private PcxReader(Stream s, bool owns, byte[] b, ImageInfo info, bool canDecode)
    {
        _stream = s; _ownsStream = owns; _bytes = b;
        Info = info; CanDecodePixels = canDecode;
    }

    /// <summary>Open a PCX file by path.</summary>
    public static PcxReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a PCX from a stream.</summary>
    public static PcxReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 128 || bytes[0] != 0x0A)
        {
            throw new ImageFormatException("Not a PCX file.");
        }

        byte bitsPerPlane = bytes[3];
        int xMin = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(4));
        int yMin = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(6));
        int xMax = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(8));
        int yMax = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(10));
        byte nPlanes = bytes[65];
        int width = xMax - xMin + 1;
        int height = yMax - yMin + 1;

        bool supported = (bitsPerPlane == 1 && nPlanes == 1)
                      || (bitsPerPlane == 8 && nPlanes == 1)
                      || (bitsPerPlane == 8 && nPlanes == 3);

        var pf = (nPlanes, bitsPerPlane) switch
        {
            (1, 8) => PixelFormat.Indexed8,
            (3, 8) => PixelFormat.Rgb24,
            (1, 1) => PixelFormat.Indexed1,
            _ => PixelFormat.Unknown,
        };

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = nPlanes * bitsPerPlane,
            ChannelCount = nPlanes,
            PixelFormat = pf,
            Format = ImageFormat.Pcx,
            FrameCount = 1,
        };

        return new PcxReader(stream, ownsStream, bytes, info, supported);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        if (!CanDecodePixels) throw new NotSupportedException("Unsupported PCX layout.");

        byte bitsPerPlane = _bytes[3];
        byte nPlanes = _bytes[65];
        int bytesPerLine = BinaryPrimitives.ReadUInt16LittleEndian(_bytes.AsSpan(66));
        int width = Info.Width;
        int height = Info.Height;
        int p = 128;

        int totalLine = bytesPerLine * nPlanes;
        var line = new byte[totalLine];
        var pf = Info.PixelFormat;

        if (pf == PixelFormat.Indexed8)
        {
            // 256-color palette is the last 769 bytes of the file: 0x0C + 768.
            uint[] palette = new uint[256];
            if (_bytes.Length >= 769 && _bytes[^769] == 0x0C)
            {
                int pp = _bytes.Length - 768;
                for (int i = 0; i < 256; i++)
                {
                    byte r = _bytes[pp + i * 3];
                    byte g = _bytes[pp + i * 3 + 1];
                    byte b = _bytes[pp + i * 3 + 2];
                    palette[i] = 0xFF000000u | ((uint)b << 16) | ((uint)g << 8) | r;
                }
            }
            int stride = width;
            var (frame, buf) = ImageFrame.Rent(width, height, PixelFormat.Indexed8, stride, palette);
            for (int y = 0; y < height; y++)
            {
                DecodeRle(_bytes, ref p, line);
                Buffer.BlockCopy(line, 0, buf, y * stride, width);
            }
            yield return frame;
            yield break;
        }

        if (pf == PixelFormat.Rgb24)
        {
            int stride = width * 3;
            var (frame, buf) = ImageFrame.Rent(width, height, PixelFormat.Rgb24, stride);
            for (int y = 0; y < height; y++)
            {
                DecodeRle(_bytes, ref p, line);
                int row = y * stride;
                for (int x = 0; x < width; x++)
                {
                    buf[row + x * 3 + 0] = line[x];
                    buf[row + x * 3 + 1] = line[bytesPerLine + x];
                    buf[row + x * 3 + 2] = line[2 * bytesPerLine + x];
                }
            }
            yield return frame;
            yield break;
        }

        // Indexed1
        {
            int stride = (width + 7) / 8;
            var (frame, buf) = ImageFrame.Rent(width, height, PixelFormat.Indexed1, stride);
            for (int y = 0; y < height; y++)
            {
                DecodeRle(_bytes, ref p, line);
                Buffer.BlockCopy(line, 0, buf, y * stride, Math.Min(stride, line.Length));
            }
            yield return frame;
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void DecodeRle(byte[] src, ref int p, byte[] line)
    {
        int o = 0;
        while (o < line.Length && p < src.Length)
        {
            byte b = src[p++];
            if ((b & 0xC0) == 0xC0)
            {
                int run = b & 0x3F;
                if (p >= src.Length) break;
                byte val = src[p++];
                while (run-- > 0 && o < line.Length) line[o++] = val;
            }
            else
            {
                line[o++] = b;
            }
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
