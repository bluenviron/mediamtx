using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Codecs.Bcn;

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
    private readonly BcnFormat _bcn;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Dds;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;

    /// <inheritdoc/>
    public bool CanDecodePixels =>
        (!_isCompressed && Info.PixelFormat != PixelFormat.Unknown) ||
        _bcn is BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3
             or BcnFormat.Bc4 or BcnFormat.Bc5
             or BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16
             or BcnFormat.Bc7;

    private DdsReader(Stream s, bool owns, byte[] b, int pixelsOffset,
                      uint r, uint g, uint bMask, uint a, int pitch, bool compressed,
                      BcnFormat bcn, ImageInfo info)
    {
        _stream = s; _ownsStream = owns; _bytes = b;
        _pixelsOffset = pixelsOffset; _rBit = r; _gBit = g; _bBit = bMask; _aBit = a;
        _pitchOrLinearSize = pitch; _isCompressed = compressed; _bcn = bcn;
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
        uint dxgiFormat = 0;
        if (dx10)
        {
            if (bytes.Length < 128 + 20) throw new ImageFormatException("Truncated DX10 DDS.");
            dxgiFormat = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(128));
            pixelsOffset = 128 + 20;
        }

        bool compressed = (pfFlags & 0x4) != 0 && !((pfFlags & 0x40) != 0);
        var bcn = compressed ? BcnDecoder.Identify(fourCC, dxgiFormat) : BcnFormat.None;
        var pf = compressed ? BcnToPixelFormat(bcn) : ClassifyUncompressed(rgbBitCount, rMask, gMask, bMask, aMask);
        bool hasAlpha = compressed
            ? bcn is BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc7
            : aMask != 0;
        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = compressed ? BcnBitsPerPixel(bcn) : (int)rgbBitCount,
            ChannelCount = pf.ChannelCount(),
            PixelFormat = pf,
            Format = ImageFormat.Dds,
            HasAlpha = hasAlpha,
            FrameCount = 1,
            ColorSpace = compressed ? (bcn != BcnFormat.None ? bcn.ToString() : fourCC) : null,
        };

        return new DdsReader(stream, ownsStream, bytes, pixelsOffset,
                             rMask, gMask, bMask, aMask, pitch, compressed, bcn, info);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();

        int width = Info.Width;
        int height = Info.Height;

        if (_isCompressed)
        {
            int blocksX = (width + 3) / 4;
            int blocksY = (height + 3) / 4;
            int bytesPerBlock = _bcn switch
            {
                BcnFormat.Bc1 or BcnFormat.Bc4 => 8,
                BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc5
                  or BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16 or BcnFormat.Bc7 => 16,
                _ => 0,
            };
            if (bytesPerBlock == 0)
            {
                throw new NotSupportedException(
                    $"DDS uses {Info.ColorSpace ?? "unknown"} block compression; pixel decode not implemented.");
            }
            int payloadLen = blocksX * blocksY * bytesPerBlock;
            if (_pixelsOffset + payloadLen > _bytes.Length)
            {
                throw new ImageFormatException("Truncated DDS pixel data.");
            }
            var payload = _bytes.AsSpan(_pixelsOffset, payloadLen);

            byte[] decoded;
            int stride;
            PixelFormat pf;
            switch (_bcn)
            {
                case BcnFormat.Bc1:
                    decoded = BcnDecoder.DecodeBc1(payload, width, height);
                    stride = width * 4;
                    pf = PixelFormat.Bgra32;
                    break;
                case BcnFormat.Bc2:
                    decoded = BcnDecoder.DecodeBc2(payload, width, height);
                    stride = width * 4;
                    pf = PixelFormat.Bgra32;
                    break;
                case BcnFormat.Bc3:
                    decoded = BcnDecoder.DecodeBc3(payload, width, height);
                    stride = width * 4;
                    pf = PixelFormat.Bgra32;
                    break;
                case BcnFormat.Bc4:
                    decoded = BcnDecoder.DecodeBc4(payload, width, height);
                    stride = width;
                    pf = PixelFormat.Gray8;
                    break;
                case BcnFormat.Bc5:
                    decoded = BcnDecoder.DecodeBc5(payload, width, height);
                    stride = width * 3;
                    pf = PixelFormat.Rgb24;
                    break;
                case BcnFormat.Bc7:
                    decoded = Bc7Decoder.DecodeBc7(payload, width, height);
                    stride = width * 4;
                    pf = PixelFormat.Bgra32;
                    break;
                case BcnFormat.Bc6hUf16:
                    decoded = Bc6hDecoder.DecodeBc6h(payload, width, height, isSigned: false);
                    stride = width * 12;
                    pf = PixelFormat.Rgb96Float;
                    break;
                case BcnFormat.Bc6hSf16:
                    decoded = Bc6hDecoder.DecodeBc6h(payload, width, height, isSigned: true);
                    stride = width * 12;
                    pf = PixelFormat.Rgb96Float;
                    break;
                default:
                    throw new NotSupportedException(
                        $"DDS uses {_bcn} block compression; pixel decode is not implemented in this Mediar release.");
            }
            yield return new ImageFrame(width, height, pf, stride, decoded);
            yield break;
        }

        if (Info.PixelFormat == PixelFormat.Unknown)
        {
            throw new NotSupportedException("Unrecognised DDS uncompressed pixel layout.");
        }

        int bpp = Info.BitsPerPixel / 8;
        int strideUn = width * bpp;
        var (frame, buf) = ImageFrame.Rent(width, height, Info.PixelFormat, strideUn);
        int total = strideUn * height;
        if (_pixelsOffset + total > _bytes.Length)
        {
            throw new ImageFormatException("Truncated DDS pixel data.");
        }
        Buffer.BlockCopy(_bytes, _pixelsOffset, buf, 0, total);
        yield return frame;
    }

    private static PixelFormat BcnToPixelFormat(BcnFormat f) => f switch
    {
        BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc7 => PixelFormat.Bgra32,
        BcnFormat.Bc4 => PixelFormat.Gray8,
        BcnFormat.Bc5 => PixelFormat.Rgb24,
        BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16 => PixelFormat.Rgb96Float,
        _ => PixelFormat.Unknown,
    };

    private static int BcnBitsPerPixel(BcnFormat f) => f switch
    {
        BcnFormat.Bc1 or BcnFormat.Bc4 => 4,
        BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc5
          or BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16 or BcnFormat.Bc7 => 8,
        _ => 0,
    };

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
