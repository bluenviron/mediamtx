using System.Buffers;
using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Bmp;

/// <summary>
/// Reader for Windows Bitmap (<c>.bmp</c>) and bare device-independent
/// bitmap (<c>.dib</c>) files. Supports BITMAPCOREHEADER (V2),
/// BITMAPINFOHEADER (V3), BITMAPV4HEADER (V4) and BITMAPV5HEADER (V5)
/// with compression methods <c>BI_RGB</c>, <c>BI_RLE8</c>, <c>BI_RLE4</c>
/// and <c>BI_BITFIELDS</c>.
/// </summary>
public sealed class BmpReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly bool _isDib;
    private readonly long _pixelDataOffset;
    private readonly BmpInfoHeader _hdr;
    private readonly uint[] _palette;
    private readonly uint _rMask, _gMask, _bMask, _aMask;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _isDib ? ImageFormat.Dib : ImageFormat.Bmp;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;

    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    private BmpReader(
        Stream stream, bool ownsStream, bool isDib,
        BmpInfoHeader hdr, uint[] palette,
        uint rMask, uint gMask, uint bMask, uint aMask,
        long pixelDataOffset, ImageInfo info)
    {
        _stream = stream;
        _ownsStream = ownsStream;
        _isDib = isDib;
        _hdr = hdr;
        _palette = palette;
        _rMask = rMask;
        _gMask = gMask;
        _bMask = bMask;
        _aMask = aMask;
        _pixelDataOffset = pixelDataOffset;
        Info = info;
    }

    /// <summary>Open a BMP / DIB file by path.</summary>
    public static BmpReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            bool isDib = !path.EndsWith(".bmp", StringComparison.OrdinalIgnoreCase);
            return Open(fs, isDib, ownsStream: true);
        }
        catch
        {
            fs.Dispose();
            throw;
        }
    }

    /// <summary>Open a BMP / DIB from <paramref name="stream"/>.</summary>
    public static BmpReader Open(Stream stream, bool isDib = false, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);

        long startPos = stream.CanSeek ? stream.Position : 0;
        Span<byte> fileHdr = stackalloc byte[14];
        long pixelOffset;
        bool hasFileHeader;
        if (!isDib)
        {
            ReadExactly(stream, fileHdr);
            if (fileHdr[0] != (byte)'B' || fileHdr[1] != (byte)'M')
            {
                throw new ImageFormatException("Not a BMP file: missing 'BM' magic.");
            }
            pixelOffset = BinaryPrimitives.ReadUInt32LittleEndian(fileHdr.Slice(10, 4));
            hasFileHeader = true;
        }
        else
        {
            pixelOffset = 0; // computed below
            hasFileHeader = false;
        }

        Span<byte> sizeBuf = stackalloc byte[4];
        ReadExactly(stream, sizeBuf);
        uint dibHeaderSize = BinaryPrimitives.ReadUInt32LittleEndian(sizeBuf);
        if (dibHeaderSize is < 12 or > 256)
        {
            throw new ImageFormatException($"Unsupported DIB header size: {dibHeaderSize}.");
        }
        Span<byte> dibBuf = dibHeaderSize <= 256 ? stackalloc byte[256] : new byte[dibHeaderSize];
        sizeBuf.CopyTo(dibBuf);
        ReadExactly(stream, dibBuf.Slice(4, (int)dibHeaderSize - 4));

        var hdr = BmpInfoHeader.Parse(dibBuf[..(int)dibHeaderSize]);
        uint rMask = hdr.RedMask, gMask = hdr.GreenMask, bMask = hdr.BlueMask, aMask = hdr.AlphaMask;

        // BI_BITFIELDS: masks follow header
        if (hdr.Compression == BmpCompression.BitFields && dibHeaderSize == 40)
        {
            Span<byte> maskBuf = stackalloc byte[12];
            ReadExactly(stream, maskBuf);
            rMask = BinaryPrimitives.ReadUInt32LittleEndian(maskBuf[..4]);
            gMask = BinaryPrimitives.ReadUInt32LittleEndian(maskBuf.Slice(4, 4));
            bMask = BinaryPrimitives.ReadUInt32LittleEndian(maskBuf.Slice(8, 4));
            if (hdr.BitsPerPixel == 32)
            {
                // optional 4th mask (BI_ALPHABITFIELDS) on Windows CE/V3 — not always present;
                // we try to be tolerant.
                if (stream.CanSeek)
                {
                    long here = stream.Position;
                    Span<byte> aBuf = stackalloc byte[4];
                    int n = stream.Read(aBuf);
                    if (n == 4)
                    {
                        aMask = BinaryPrimitives.ReadUInt32LittleEndian(aBuf);
                    }
                    else
                    {
                        stream.Position = here;
                    }
                }
            }
        }

        // palette for <= 8-bit
        int paletteEntries = (int)hdr.PaletteColors;
        if (paletteEntries == 0 && hdr.BitsPerPixel <= 8)
        {
            paletteEntries = 1 << hdr.BitsPerPixel;
        }
        uint[] palette = paletteEntries > 0 ? new uint[paletteEntries] : [];
        if (paletteEntries > 0)
        {
            int entrySize = dibHeaderSize == 12 ? 3 : 4;
            byte[] paletteBytes = new byte[paletteEntries * entrySize];
            ReadExactly(stream, paletteBytes);
            for (int i = 0; i < paletteEntries; i++)
            {
                byte b = paletteBytes[i * entrySize + 0];
                byte g = paletteBytes[i * entrySize + 1];
                byte r = paletteBytes[i * entrySize + 2];
                palette[i] = ((uint)0xFF << 24) | ((uint)r << 16) | ((uint)g << 8) | b;
            }
        }

        if (!hasFileHeader)
        {
            pixelOffset = stream.CanSeek ? stream.Position - startPos : -1;
        }

        var info = new ImageInfo
        {
            Width = hdr.Width,
            Height = Math.Abs(hdr.Height),
            BitsPerPixel = hdr.BitsPerPixel,
            ChannelCount = hdr.BitsPerPixel switch
            {
                1 or 4 or 8 => 1,
                16 or 24 => 3,
                32 => 4,
                _ => 0,
            },
            PixelFormat = hdr.BitsPerPixel switch
            {
                1 => PixelFormat.Indexed1,
                4 => PixelFormat.Indexed4,
                8 => PixelFormat.Indexed8,
                16 => PixelFormat.Rgb565,
                24 => PixelFormat.Bgr24,
                32 => PixelFormat.Bgra32,
                _ => PixelFormat.Unknown,
            },
            Format = isDib ? ImageFormat.Dib : ImageFormat.Bmp,
            HasAlpha = hdr.BitsPerPixel == 32,
            HorizontalDpi = hdr.XPelsPerMeter * 0.0254,
            VerticalDpi = hdr.YPelsPerMeter * 0.0254,
            FrameCount = 1,
        };

        // skip ahead to pixel data when given an absolute offset (file header present).
        if (hasFileHeader && stream.CanSeek)
        {
            stream.Position = startPos + pixelOffset;
        }

        return new BmpReader(stream, ownsStream, isDib, hdr, palette,
                             rMask, gMask, bMask, aMask, pixelOffset, info);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        int width = _hdr.Width;
        int height = Math.Abs(_hdr.Height);
        bool topDown = _hdr.Height < 0;
        PixelFormat pf = Info.PixelFormat;
        int outBpp = pf.BitsPerPixel();
        int outStride = ((width * outBpp + 31) / 32) * 4;
        if (pf is PixelFormat.Indexed1 or PixelFormat.Indexed4 or PixelFormat.Indexed8)
        {
            outStride = (width * outBpp + 7) / 8;
        }
        else
        {
            outStride = width * (outBpp / 8);
        }

        var (frame, pixels) = ImageFrame.Rent(width, height, pf, outStride, _palette);

        try
        {
            switch (_hdr.Compression)
            {
                case BmpCompression.Rgb:
                case BmpCompression.BitFields:
                    await DecodeUncompressedAsync(width, height, topDown, outStride, pixels,
                                                   cancellationToken).ConfigureAwait(false);
                    break;
                case BmpCompression.Rle8:
                    await DecodeRle8Async(width, height, outStride, pixels,
                                          cancellationToken).ConfigureAwait(false);
                    break;
                case BmpCompression.Rle4:
                    await DecodeRle4Async(width, height, outStride, pixels,
                                          cancellationToken).ConfigureAwait(false);
                    break;
                default:
                    throw new ImageFormatException($"Unsupported BMP compression {_hdr.Compression}.");
            }
            yield return frame;
        }
        finally
        {
        }
    }

    private async Task DecodeUncompressedAsync(
        int width, int height, bool topDown, int outStride, byte[] dest,
        CancellationToken cancellationToken)
    {
        int srcStride = ((width * _hdr.BitsPerPixel + 31) / 32) * 4;
        byte[] row = ArrayPool<byte>.Shared.Rent(srcStride);
        try
        {
            for (int y = 0; y < height; y++)
            {
                cancellationToken.ThrowIfCancellationRequested();
                await _stream.ReadExactlyAsync(row.AsMemory(0, srcStride), cancellationToken)
                             .ConfigureAwait(false);
                int dstRow = topDown ? y : (height - 1 - y);
                Span<byte> dstSpan = dest.AsSpan(dstRow * outStride, outStride);
                CopyRow(row.AsSpan(0, srcStride), dstSpan, width);
            }
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(row);
        }
    }

    private void CopyRow(ReadOnlySpan<byte> src, Span<byte> dst, int width)
    {
        switch (_hdr.BitsPerPixel)
        {
            case 1:
            case 4:
            case 8:
            case 24:
            case 32:
                src[..dst.Length].CopyTo(dst);
                break;
            case 16:
                // BMP stores 16bpp as 5/5/5 with optional bitfields; we store
                // as RGB565 — convert.
                for (int x = 0; x < width; x++)
                {
                    ushort v = BinaryPrimitives.ReadUInt16LittleEndian(src.Slice(x * 2, 2));
                    ushort r5, g, b5;
                    if (_hdr.Compression == BmpCompression.BitFields && _rMask != 0)
                    {
                        r5 = (ushort)((v & _rMask) >> BitOffset(_rMask));
                        g = (ushort)((v & _gMask) >> BitOffset(_gMask));
                        b5 = (ushort)((v & _bMask) >> BitOffset(_bMask));
                        ushort packed = (ushort)((Resize(r5, MaskBits(_rMask), 5) << 11) |
                                                 (Resize(g, MaskBits(_gMask), 6) << 5) |
                                                  Resize(b5, MaskBits(_bMask), 5));
                        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(x * 2, 2), packed);
                    }
                    else
                    {
                        // default 555 → 565
                        ushort r = (ushort)((v >> 10) & 0x1F);
                        ushort gg = (ushort)((v >> 5) & 0x1F);
                        ushort bb = (ushort)(v & 0x1F);
                        ushort packed = (ushort)((r << 11) | (gg << 6) | (gg & 1) << 5 | bb);
                        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(x * 2, 2), packed);
                    }
                }
                break;
            default:
                throw new ImageFormatException($"Unsupported BMP bit depth: {_hdr.BitsPerPixel}");
        }
    }

    private async Task DecodeRle8Async(int width, int height, int outStride, byte[] dst,
                                        CancellationToken cancellationToken)
    {
        // BMP RLE always bottom-up.
        int x = 0, y = height - 1;
        byte[] one = new byte[2];
        while (y >= 0)
        {
            cancellationToken.ThrowIfCancellationRequested();
            await _stream.ReadExactlyAsync(one.AsMemory(), cancellationToken).ConfigureAwait(false);
            byte n = one[0];
            byte v = one[1];
            if (n == 0)
            {
                if (v == 0) { x = 0; y--; }
                else if (v == 1) { return; }
                else if (v == 2)
                {
                    byte[] dxy = new byte[2];
                    await _stream.ReadExactlyAsync(dxy.AsMemory(), cancellationToken).ConfigureAwait(false);
                    x += dxy[0]; y -= dxy[1];
                }
                else
                {
                    int absLen = v;
                    byte[] abs = new byte[absLen + (absLen & 1)];
                    await _stream.ReadExactlyAsync(abs.AsMemory(), cancellationToken).ConfigureAwait(false);
                    for (int i = 0; i < absLen; i++)
                    {
                        if (x < width && y >= 0) dst[y * outStride + x++] = abs[i];
                    }
                }
            }
            else
            {
                for (int i = 0; i < n; i++)
                {
                    if (x < width && y >= 0) dst[y * outStride + x++] = v;
                }
            }
        }
    }

    private async Task DecodeRle4Async(int width, int height, int outStride, byte[] dst,
                                        CancellationToken cancellationToken)
    {
        // dst here is the *packed* 4bpp buffer.
        int x = 0, y = height - 1;
        byte[] one = new byte[2];
        while (y >= 0)
        {
            cancellationToken.ThrowIfCancellationRequested();
            await _stream.ReadExactlyAsync(one.AsMemory(), cancellationToken).ConfigureAwait(false);
            byte n = one[0];
            byte v = one[1];
            if (n == 0)
            {
                if (v == 0) { x = 0; y--; }
                else if (v == 1) { return; }
                else if (v == 2)
                {
                    byte[] dxy = new byte[2];
                    await _stream.ReadExactlyAsync(dxy.AsMemory(), cancellationToken).ConfigureAwait(false);
                    x += dxy[0]; y -= dxy[1];
                }
                else
                {
                    int absLen = v;
                    int bytesToRead = (absLen + 1) / 2;
                    if ((bytesToRead & 1) != 0) bytesToRead++;
                    byte[] abs = new byte[bytesToRead];
                    await _stream.ReadExactlyAsync(abs.AsMemory(), cancellationToken).ConfigureAwait(false);
                    for (int i = 0; i < absLen; i++)
                    {
                        byte nyb = (byte)((abs[i / 2] >> ((1 - (i & 1)) * 4)) & 0x0F);
                        if (x < width && y >= 0) PutNibble(dst, y * outStride, x++, nyb);
                    }
                }
            }
            else
            {
                byte hi = (byte)((v >> 4) & 0x0F);
                byte lo = (byte)(v & 0x0F);
                for (int i = 0; i < n; i++)
                {
                    byte nyb = (i & 1) == 0 ? hi : lo;
                    if (x < width && y >= 0) PutNibble(dst, y * outStride, x++, nyb);
                }
            }
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void PutNibble(byte[] dst, int rowStart, int x, byte value)
    {
        int idx = rowStart + (x / 2);
        if ((x & 1) == 0)
        {
            dst[idx] = (byte)((dst[idx] & 0x0F) | ((value & 0x0F) << 4));
        }
        else
        {
            dst[idx] = (byte)((dst[idx] & 0xF0) | (value & 0x0F));
        }
    }

    private static int BitOffset(uint mask)
    {
        if (mask == 0) return 0;
        int n = 0;
        while ((mask & 1) == 0) { mask >>= 1; n++; }
        return n;
    }

    private static int MaskBits(uint mask)
    {
        int bits = 0;
        while (mask != 0) { bits += (int)(mask & 1); mask >>= 1; }
        return bits;
    }

    private static ushort Resize(ushort value, int fromBits, int toBits)
    {
        if (fromBits == toBits) return value;
        if (fromBits == 0) return 0;
        return (ushort)((value * ((1 << toBits) - 1) + ((1 << fromBits) >> 1)) / ((1 << fromBits) - 1));
    }

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
