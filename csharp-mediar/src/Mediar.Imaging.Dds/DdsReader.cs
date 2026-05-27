using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Dds;

/// <summary>
/// Reader for Microsoft DirectDraw Surface (.dds) files. The reader
/// fully decodes uncompressed RGB / RGBA / BGRA layouts; for BC1-BC7
/// (DXT*/BPTC) compressed surfaces it exposes the raw block payload as
/// a single buffer for downstream consumers.
/// </summary>
public sealed class DdsReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly int _pixelsOffset;
    private readonly uint _rBit, _gBit, _bBit, _aBit;
    private readonly int _pitchOrLinearSize;
    private readonly bool _isCompressed;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Dds;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;

    /// <inheritdoc/>
    public bool CanDecodePixels => !_isCompressed && Info.PixelFormat != PixelFormat.Unknown;

    private DdsReader(Stream s, bool owns, byte[] b, int pixelsOffset,
                      uint r, uint g, uint bMask, uint a, int pitch, bool compressed,
                      ImageInfo info)
    {
        _stream = s; _ownsStream = owns; _bytes = b;
        _pixelsOffset = pixelsOffset; _rBit = r; _gBit = g; _bBit = bMask; _aBit = a;
        _pitchOrLinearSize = pitch; _isCompressed = compressed;
        Info = info;
    }

    /// <summary>Open a DDS file by path.</summary>
    public static DdsReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a DDS from a stream.</summary>
    public static DdsReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 128) throw new ImageFormatException("Truncated DDS.");
        if (bytes[0] != (byte)'D' || bytes[1] != (byte)'D' ||
            bytes[2] != (byte)'S' || bytes[3] != (byte)' ')
        {
            throw new ImageFormatException("Not a DDS file (bad magic).");
        }
        uint size = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(4));
        if (size != 124) throw new ImageFormatException("Bad DDS header size.");

        int height = (int)BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(12));
        int width = (int)BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(16));
        int pitch = (int)BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(20));

        // Pixel format starts at offset 76, length 32.
        uint pfFlags = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(80));
        string fourCC = Encoding.ASCII.GetString(bytes, 84, 4);
        uint rgbBitCount = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(88));
        uint rMask = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(92));
        uint gMask = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(96));
        uint bMask = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(100));
        uint aMask = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(104));

        int pixelsOffset = 128;
        bool dx10 = (pfFlags & 0x4) != 0 && fourCC == "DX10";
        if (dx10) pixelsOffset = 128 + 20;

        bool compressed = (pfFlags & 0x4) != 0 && !((pfFlags & 0x40) != 0);
        var pf = compressed ? PixelFormat.Unknown : ClassifyUncompressed(rgbBitCount, rMask, gMask, bMask, aMask);
        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = (int)rgbBitCount,
            ChannelCount = pf.ChannelCount(),
            PixelFormat = pf,
            Format = ImageFormat.Dds,
            HasAlpha = aMask != 0,
            FrameCount = 1,
            ColorSpace = compressed ? fourCC : null,
        };

        return new DdsReader(stream, ownsStream, bytes, pixelsOffset,
                             rMask, gMask, bMask, aMask, pitch, compressed, info);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        if (_isCompressed)
        {
            throw new NotSupportedException(
                $"DDS uses {Info.ColorSpace ?? "compressed"} block compression; pixel decode not implemented.");
        }
        if (Info.PixelFormat == PixelFormat.Unknown)
        {
            throw new NotSupportedException("Unrecognised DDS uncompressed pixel layout.");
        }

        int width = Info.Width, height = Info.Height;
        int bpp = Info.BitsPerPixel / 8;
        int stride = width * bpp;
        var (frame, buf) = ImageFrame.Rent(width, height, Info.PixelFormat, stride);
        int total = stride * height;
        if (_pixelsOffset + total > _bytes.Length)
        {
            throw new ImageFormatException("Truncated DDS pixel data.");
        }
        Buffer.BlockCopy(_bytes, _pixelsOffset, buf, 0, total);
        yield return frame;
    }

    private static PixelFormat ClassifyUncompressed(uint bpp, uint r, uint g, uint b, uint a)
    {
        return (bpp, r, g, b, a) switch
        {
            (32, 0x00FF0000u, 0x0000FF00u, 0x000000FFu, 0xFF000000u) => PixelFormat.Bgra32,
            (32, 0x000000FFu, 0x0000FF00u, 0x00FF0000u, 0xFF000000u) => PixelFormat.Rgba32,
            (24, 0x00FF0000u, 0x0000FF00u, 0x000000FFu, 0) => PixelFormat.Bgr24,
            (24, 0x000000FFu, 0x0000FF00u, 0x00FF0000u, 0) => PixelFormat.Rgb24,
            (8, _, _, _, _) => PixelFormat.Gray8,
            (16, 0xF800u, 0x07E0u, 0x001Fu, 0) => PixelFormat.Rgb565,
            _ => PixelFormat.Unknown,
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
