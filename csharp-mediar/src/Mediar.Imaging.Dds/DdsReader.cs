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
    private readonly uint _packedDxgi;
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
                      BcnFormat bcn, uint packedDxgi, ImageInfo info)
    {
        _stream = s; _ownsStream = owns; _bytes = b;
        _pixelsOffset = pixelsOffset; _rBit = r; _gBit = g; _bBit = bMask; _aBit = a;
        _pitchOrLinearSize = pitch; _isCompressed = compressed; _bcn = bcn;
        _packedDxgi = packedDxgi;
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

        bool fourCcFlag = (pfFlags & 0x4) != 0;
        bool rgbFlag = (pfFlags & 0x40) != 0;
        bool compressed = fourCcFlag && !rgbFlag;
        var bcn = compressed ? BcnDecoder.Identify(fourCC, dxgiFormat) : BcnFormat.None;

        // DX10 uncompressed surfaces also set the FOURCC flag but carry a
        // non-BCn DXGI code in the extended header. Re-classify those as
        // uncompressed so the byte-copy path runs and the pixel format is
        // populated from the DXGI table rather than the BCn table.
        PixelFormat dxgiUncompressed = PixelFormat.Unknown;
        bool dxgiAlpha = false;
        string? dxgiColorSpace = null;
        if (dx10 && bcn == BcnFormat.None)
        {
            dxgiUncompressed = DxgiToPixelFormat(dxgiFormat, out dxgiAlpha, out dxgiColorSpace);
            if (dxgiUncompressed != PixelFormat.Unknown)
            {
                compressed = false;
            }
        }

        var pf = compressed
            ? BcnToPixelFormat(bcn)
            : (dxgiUncompressed != PixelFormat.Unknown
                ? dxgiUncompressed
                : ClassifyUncompressed(rgbBitCount, rMask, gMask, bMask, aMask));
        bool hasAlpha = compressed
            ? bcn is BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc7
            : (dxgiUncompressed != PixelFormat.Unknown ? dxgiAlpha : aMask != 0);
        int bppFinal = compressed
            ? BcnBitsPerPixel(bcn)
            : (dxgiUncompressed != PixelFormat.Unknown ? pf.BitsPerPixel() : (int)rgbBitCount);
        uint packedDxgiCode = 0;
        if (!compressed && dxgiUncompressed != PixelFormat.Unknown && IsPackedDxgi(dxgiFormat))
        {
            packedDxgiCode = dxgiFormat;
        }
        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = bppFinal,
            ChannelCount = pf.ChannelCount(),
            PixelFormat = pf,
            Format = ImageFormat.Dds,
            HasAlpha = hasAlpha,
            FrameCount = 1,
            ColorSpace = compressed
                ? (bcn != BcnFormat.None ? bcn.ToString() : fourCC)
                : dxgiColorSpace,
        };

        return new DdsReader(stream, ownsStream, bytes, pixelsOffset,
                             rMask, gMask, bMask, aMask, pitch, compressed, bcn, packedDxgiCode, info);
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

        if (_packedDxgi != 0)
        {
            int srcBytes = width * height * 4;
            if (_pixelsOffset + srcBytes > _bytes.Length)
            {
                throw new ImageFormatException("Truncated DDS pixel data.");
            }
            var packedSrc = _bytes.AsSpan(_pixelsOffset, srcBytes);
            byte[] unpacked = _packedDxgi switch
            {
                24 => DdsPackedUnpacker.UnpackR10G10B10A2Unorm(packedSrc, width, height),
                26 => DdsPackedUnpacker.UnpackR11G11B10Float(packedSrc, width, height),
                67 => DdsPackedUnpacker.UnpackR9G9B9E5SharedExp(packedSrc, width, height),
                _ => throw new NotSupportedException(
                    $"DDS packed DXGI format {_packedDxgi} is not implemented."),
            };
            int packedStride = Info.PixelFormat == PixelFormat.Rgba32 ? width * 4 : width * 12;
            yield return new ImageFrame(width, height, Info.PixelFormat, packedStride, unpacked);
            yield break;
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

    /// <summary>
    /// Maps a DXGI_FORMAT value (from the DDS DX10 extended header) onto a
    /// Mediar <see cref="PixelFormat"/> for uncompressed surfaces. Returns
    /// <see cref="PixelFormat.Unknown"/> for compressed / unsupported codes,
    /// in which case the caller should fall back to the BCn dispatcher.
    /// </summary>
    /// <remarks>
    /// Values are sourced from <c>dxgiformat.h</c> in the Windows 10 SDK.
    /// The <c>hasAlpha</c> output captures whether the format has an alpha
    /// channel; the <c>colorSpace</c> output captures sRGB and float labels
    /// so the SRGB-suffixed DXGI variants surface meaningfully in
    /// <see cref="ImageInfo.ColorSpace"/>.
    /// </remarks>
    internal static PixelFormat DxgiToPixelFormat(uint dxgiFormat, out bool hasAlpha, out string? colorSpace)
    {
        hasAlpha = false;
        colorSpace = null;
        switch (dxgiFormat)
        {
            // 32-bit per channel float (HDR)
            case 2:  hasAlpha = true; colorSpace = "Linear"; return PixelFormat.Rgba128Float;  // R32G32B32A32_FLOAT
            case 6:  colorSpace = "Linear"; return PixelFormat.Rgb96Float;                     // R32G32B32_FLOAT
            // 16-bit per channel (RGBA64 is byte-identical to FP16 RGBA)
            case 10: hasAlpha = true; colorSpace = "Linear"; return PixelFormat.Rgba64;        // R16G16B16A16_FLOAT
            case 11: hasAlpha = true; return PixelFormat.Rgba64;                               // R16G16B16A16_UNORM
            case 13: hasAlpha = true; return PixelFormat.Rgba64;                               // R16G16B16A16_SNORM
            // 16-bit two-channel
            case 34: colorSpace = "Linear"; return PixelFormat.Rg32;                           // R16G16_FLOAT
            case 35: return PixelFormat.Rg32;                                                  // R16G16_UNORM
            case 37: return PixelFormat.Rg32;                                                  // R16G16_SNORM
            // 8-bit RGBA
            case 28: hasAlpha = true; return PixelFormat.Rgba32;                               // R8G8B8A8_UNORM
            case 29: hasAlpha = true; colorSpace = "sRGB"; return PixelFormat.Rgba32;          // R8G8B8A8_UNORM_SRGB
            case 31: hasAlpha = true; return PixelFormat.Rgba32;                               // R8G8B8A8_SNORM
            case 87: hasAlpha = true; return PixelFormat.Bgra32;                               // B8G8R8A8_UNORM
            case 88: return PixelFormat.Bgra32;                                                // B8G8R8X8_UNORM (X = ignore)
            case 91: hasAlpha = true; colorSpace = "sRGB"; return PixelFormat.Bgra32;          // B8G8R8A8_UNORM_SRGB
            case 93: colorSpace = "sRGB"; return PixelFormat.Bgra32;                           // B8G8R8X8_UNORM_SRGB
            // 8-bit two-channel (byte-identical to GrayAlpha16)
            case 49: return PixelFormat.GrayAlpha16;                                           // R8G8_UNORM
            case 51: return PixelFormat.GrayAlpha16;                                           // R8G8_SNORM
            // 16-bit single-channel
            case 56: return PixelFormat.Gray16;                                                // R16_UNORM
            case 58: return PixelFormat.Gray16;                                                // R16_SNORM
            // 8-bit single-channel
            case 61: return PixelFormat.Gray8;                                                 // R8_UNORM
            case 63: return PixelFormat.Gray8;                                                 // R8_SNORM
            case 65: return PixelFormat.Gray8;                                                 // A8_UNORM
            // 32-bit single-channel float (depth / luminance / scientific)
            case 41: colorSpace = "Linear"; return PixelFormat.Gray32Float;                    // R32_FLOAT
            // 16-bit packed
            case 85: return PixelFormat.Rgb565;                                                // B5G6R5_UNORM
            case 86: hasAlpha = true; return PixelFormat.Rgba5551;                             // B5G5R5A1_UNORM
            // 32-bit packed bit-fields (require unpacking at decode time)
            case 24: hasAlpha = true; return PixelFormat.Rgba32;                               // R10G10B10A2_UNORM
            case 26: colorSpace = "Linear"; return PixelFormat.Rgb96Float;                     // R11G11B10_FLOAT
            case 67: colorSpace = "Linear"; return PixelFormat.Rgb96Float;                     // R9G9B9E5_SHAREDEXP
        }
        return PixelFormat.Unknown;
    }

    /// <summary>
    /// Returns true for DXGI format codes whose bytes do not match the
    /// chosen <see cref="PixelFormat"/> 1:1 and therefore require runtime
    /// unpacking via <see cref="DdsPackedUnpacker"/>.
    /// </summary>
    internal static bool IsPackedDxgi(uint dxgiFormat) => dxgiFormat is 24u or 26u or 67u;

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}
