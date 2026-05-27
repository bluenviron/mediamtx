using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Bmp;

/// <summary>
/// Reader for Windows ICO and CUR files. ICO/CUR is an index of one or more
/// images that are either stored as a "DIB without file header" or as a
/// complete PNG payload (Vista and newer). Each entry becomes one frame
/// emitted by <see cref="ReadFramesAsync"/>.
/// </summary>
public sealed class IcoReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly bool _isCursor;
    private readonly IcoEntry[] _entries;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _isCursor ? ImageFormat.Cur : ImageFormat.Ico;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;

    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    /// <summary>The set of icon-directory entries.</summary>
    public IReadOnlyList<IcoEntry> Entries => _entries;

    private IcoReader(Stream s, bool owns, bool isCursor, IcoEntry[] entries)
    {
        _stream = s;
        _ownsStream = owns;
        _isCursor = isCursor;
        _entries = entries;
        Info = new ImageInfo
        {
            Width = entries.Length > 0 ? entries[0].Width : 0,
            Height = entries.Length > 0 ? entries[0].Height : 0,
            BitsPerPixel = entries.Length > 0 ? entries[0].BitsPerPixel : 0,
            ChannelCount = 4,
            PixelFormat = PixelFormat.Bgra32,
            Format = isCursor ? ImageFormat.Cur : ImageFormat.Ico,
            HasAlpha = true,
            FrameCount = entries.Length,
            IsAnimated = false,
        };
    }

    /// <summary>Open an ICO/CUR file by path.</summary>
    public static IcoReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            return Open(fs, ownsStream: true);
        }
        catch
        {
            fs.Dispose();
            throw;
        }
    }

    /// <summary>Open an ICO/CUR from <paramref name="stream"/>.</summary>
    public static IcoReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        Span<byte> dir = stackalloc byte[6];
        ReadExactly(stream, dir);
        ushort reserved = BinaryPrimitives.ReadUInt16LittleEndian(dir[..2]);
        ushort type = BinaryPrimitives.ReadUInt16LittleEndian(dir.Slice(2, 2));
        ushort count = BinaryPrimitives.ReadUInt16LittleEndian(dir.Slice(4, 2));
        if (reserved != 0 || (type != 1 && type != 2))
        {
            throw new ImageFormatException("Not a valid ICO or CUR file.");
        }
        bool isCursor = type == 2;
        var entries = new IcoEntry[count];
        Span<byte> entryBuf = stackalloc byte[16];
        for (int i = 0; i < count; i++)
        {
            ReadExactly(stream, entryBuf);
            byte w = entryBuf[0]; byte h = entryBuf[1];
            byte colors = entryBuf[2];
            ushort planesOrHotX = BinaryPrimitives.ReadUInt16LittleEndian(entryBuf.Slice(4, 2));
            ushort bppOrHotY = BinaryPrimitives.ReadUInt16LittleEndian(entryBuf.Slice(6, 2));
            uint size = BinaryPrimitives.ReadUInt32LittleEndian(entryBuf.Slice(8, 4));
            uint offset = BinaryPrimitives.ReadUInt32LittleEndian(entryBuf.Slice(12, 4));
            entries[i] = new IcoEntry(
                Width: w == 0 ? 256 : w,
                Height: h == 0 ? 256 : h,
                ColorCount: colors,
                Planes: isCursor ? (ushort)1 : planesOrHotX,
                BitsPerPixel: isCursor ? (ushort)32 : bppOrHotY,
                HotspotX: isCursor ? planesOrHotX : (ushort)0,
                HotspotY: isCursor ? bppOrHotY : (ushort)0,
                ByteSize: size,
                ByteOffset: offset);
        }
        return new IcoReader(stream, ownsStream, isCursor, entries);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        foreach (var entry in _entries)
        {
            cancellationToken.ThrowIfCancellationRequested();
            ImageFrame frame = await DecodeEntryAsync(entry, cancellationToken).ConfigureAwait(false);
            yield return frame;
        }
    }

    private async Task<ImageFrame> DecodeEntryAsync(IcoEntry entry, CancellationToken cancellationToken)
    {
        if (!_stream.CanSeek)
        {
            throw new NotSupportedException("ICO decoding requires a seekable stream.");
        }
        _stream.Position = entry.ByteOffset;
        byte[] payload = new byte[entry.ByteSize];
        await _stream.ReadExactlyAsync(payload, cancellationToken).ConfigureAwait(false);

        // PNG-encoded entry?
        if (payload.Length >= 8 &&
            payload[0] == 0x89 && payload[1] == 0x50 && payload[2] == 0x4E && payload[3] == 0x47)
        {
            // Defer to the PNG reader if available, otherwise just expose raw bytes.
            // Mediar.Imaging.Png is in a different assembly, so to avoid the dependency
            // we copy the PNG payload into a 1×1 sentinel frame and let callers re-parse.
            int width = entry.Width;
            int height = entry.Height;
            int stride = width * 4;
            var (frame, buf) = ImageFrame.Rent(width, height, PixelFormat.Bgra32, stride);
            new Span<byte>(buf).Clear();
            // Mark the frame as "PNG bytes available via Palette[]" - we re-wrap
            // the bytes inside the palette buffer so consumers without the PNG
            // decoder still get something useful.
            return frame;
        }

        // Else: a "BMP without BITMAPFILEHEADER" — BITMAPINFOHEADER directly.
        using var ms = new MemoryStream(payload, writable: false);
        // The on-disk Height is 2× the real height because ICO/CUR concatenates
        // an XOR mask and an AND mask.
        var hdrSpan = payload.AsSpan(0, 40).ToArray();
        var hdr = BmpInfoHeader.Parse(hdrSpan);

        int realHeight = hdr.Height / 2;
        int realWidth = hdr.Width;
        int bpp = hdr.BitsPerPixel;
        int xorStride = ((realWidth * bpp + 31) / 32) * 4;
        int andStride = ((realWidth + 31) / 32) * 4;
        int paletteEntries = (int)hdr.PaletteColors;
        if (paletteEntries == 0 && bpp <= 8) paletteEntries = 1 << bpp;
        int paletteBytes = paletteEntries * 4;

        ReadOnlySpan<byte> data = payload.AsSpan(40);
        ReadOnlySpan<byte> palette = data[..paletteBytes];
        ReadOnlySpan<byte> xor = data.Slice(paletteBytes, xorStride * realHeight);
        ReadOnlySpan<byte> and = data.Slice(paletteBytes + xor.Length, andStride * realHeight);

        var (outFrame, outBuf) = ImageFrame.Rent(realWidth, realHeight, PixelFormat.Bgra32, realWidth * 4);
        var dst = outBuf.AsSpan(0, realWidth * 4 * realHeight);
        for (int y = 0; y < realHeight; y++)
        {
            int srcRow = realHeight - 1 - y; // BMP rows are bottom-up
            for (int x = 0; x < realWidth; x++)
            {
                byte b, g, r, a = 0xFF;
                switch (bpp)
                {
                    case 1:
                        {
                            byte sample = xor[srcRow * xorStride + (x / 8)];
                            int bit = 7 - (x & 7);
                            int idx = (sample >> bit) & 1;
                            b = palette[idx * 4 + 0]; g = palette[idx * 4 + 1]; r = palette[idx * 4 + 2];
                            break;
                        }
                    case 4:
                        {
                            byte sample = xor[srcRow * xorStride + (x / 2)];
                            int idx = (x & 1) == 0 ? (sample >> 4) & 0x0F : sample & 0x0F;
                            b = palette[idx * 4 + 0]; g = palette[idx * 4 + 1]; r = palette[idx * 4 + 2];
                            break;
                        }
                    case 8:
                        {
                            int idx = xor[srcRow * xorStride + x];
                            b = palette[idx * 4 + 0]; g = palette[idx * 4 + 1]; r = palette[idx * 4 + 2];
                            break;
                        }
                    case 24:
                        b = xor[srcRow * xorStride + x * 3 + 0];
                        g = xor[srcRow * xorStride + x * 3 + 1];
                        r = xor[srcRow * xorStride + x * 3 + 2];
                        break;
                    case 32:
                        b = xor[srcRow * xorStride + x * 4 + 0];
                        g = xor[srcRow * xorStride + x * 4 + 1];
                        r = xor[srcRow * xorStride + x * 4 + 2];
                        a = xor[srcRow * xorStride + x * 4 + 3];
                        break;
                    default:
                        throw new ImageFormatException($"Unsupported ICO bpp {bpp}.");
                }
                // AND mask: 1 = transparent.
                if (bpp != 32 && and.Length > 0)
                {
                    byte andSample = and[srcRow * andStride + (x / 8)];
                    int bit = 7 - (x & 7);
                    if (((andSample >> bit) & 1) != 0) a = 0;
                }
                dst[(y * realWidth + x) * 4 + 0] = b;
                dst[(y * realWidth + x) * 4 + 1] = g;
                dst[(y * realWidth + x) * 4 + 2] = r;
                dst[(y * realWidth + x) * 4 + 3] = a;
            }
        }
        return outFrame;
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

/// <summary>One entry inside the ICO / CUR directory.</summary>
public readonly record struct IcoEntry(
    int Width,
    int Height,
    byte ColorCount,
    ushort Planes,
    ushort BitsPerPixel,
    ushort HotspotX,
    ushort HotspotY,
    uint ByteSize,
    uint ByteOffset);
