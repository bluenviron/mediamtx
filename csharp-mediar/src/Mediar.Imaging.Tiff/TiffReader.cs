using System.Buffers.Binary;
using System.Collections.Frozen;
using System.IO.Compression;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Codecs.Lzw;
using Mediar.Codecs.PackBits;
using Mediar.Imaging.Jpeg;

namespace Mediar.Imaging.Tiff;

/// <summary>
/// Reader for TIFF 6.0 (and the common subset of BigTIFF) files. Supports
/// uncompressed, PackBits, Deflate (Adobe), LZW, and baseline JPEG
/// (compression 7) strips and tiles. Images are emitted as 8 bpc
/// <see cref="PixelFormat.Rgb24"/> / <see cref="PixelFormat.Rgba32"/> /
/// <see cref="PixelFormat.Gray8"/> frames; CMYK is left as
/// <see cref="PixelFormat.Cmyk32"/>.
/// </summary>
/// <remarks>
/// This reader is optimised for inspection + simple round-tripping; CCITT
/// G3 / G4 and the deprecated "old-style" JPEG (compression 6) throw
/// <see cref="NotSupportedException"/>.
/// </remarks>
public sealed class TiffReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly bool _littleEndian;
    private readonly IfdEntry[] _ifd;
    private readonly TiffPageInfo[] _pages;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Tiff;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>
    /// All pages contained in this TIFF, in IFD-chain order. Index 0 is the
    /// baseline (the page referenced by the file header offset). Useful for
    /// multi-page faxes, scanned documents and Aperio SVS pyramids.
    /// </summary>
    public IReadOnlyList<TiffPage> Pages { get; }

    private TiffReader(Stream s, bool ownsStream, byte[] bytes, bool le,
                       TiffPageInfo[] pages, ImageInfo info, ImageMetadata meta, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream;
        _bytes = bytes; _littleEndian = le; _pages = pages;
        _ifd = pages[0].Ifd;
        Info = info; Metadata = meta; CanDecodePixels = canDecode;
        var pub = new TiffPage[pages.Length];
        for (int i = 0; i < pages.Length; i++) pub[i] = pages[i].ToPublic();
        Pages = pub;
    }

    /// <summary>Open a TIFF file by path.</summary>
    public static TiffReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a TIFF from a stream.</summary>
    public static TiffReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8) throw new ImageFormatException("Truncated TIFF.");
        bool le = bytes[0] == 'I' && bytes[1] == 'I';
        bool be = bytes[0] == 'M' && bytes[1] == 'M';
        if (!le && !be) throw new ImageFormatException("Bad TIFF byte-order mark.");
        int magic = ReadU16(bytes, 2, le);
        if (magic != 42) throw new ImageFormatException("Unsupported TIFF magic " + magic + " (BigTIFF=43 not implemented).");

        // Walk the entire IFD chain. The header points at IFD0; each IFD's
        // trailing uint32 either points at the next IFD or is zero (end).
        var pages = new List<TiffPageInfo>();
        uint ifdOffset = ReadU32(bytes, 4, le);
        var visited = new HashSet<uint>();
        while (ifdOffset != 0)
        {
            if (!visited.Add(ifdOffset))
            {
                // Defensive: refuse cyclic IFD chains.
                throw new ImageFormatException("Cyclic TIFF IFD chain detected.");
            }
            if (ifdOffset + 2 > bytes.Length)
                throw new ImageFormatException("IFD offset beyond end of file.");
            var entries = ParseIfd(bytes, le, (int)ifdOffset);
            pages.Add(BuildPageInfo(entries, bytes, le));

            // Locate the next-IFD pointer (immediately after the entries array).
            int n = entries.Length;
            int nextSlot = (int)ifdOffset + 2 + n * 12;
            if (nextSlot + 4 > bytes.Length) break;
            ifdOffset = ReadU32(bytes, nextSlot, le);
        }
        if (pages.Count == 0) throw new ImageFormatException("TIFF file has no IFDs.");

        var baseline = pages[0];
        var info = new ImageInfo
        {
            Width = baseline.Width,
            Height = baseline.Height,
            BitsPerPixel = baseline.BitsPerSample * baseline.SamplesPerPixel,
            ChannelCount = baseline.SamplesPerPixel,
            PixelFormat = baseline.PixelFormat,
            Format = ImageFormat.Tiff,
            HasAlpha = baseline.SamplesPerPixel == 4 && baseline.Photometric != 5,
            FrameCount = pages.Count,
        };

        var meta = BuildMetadata(baseline.Ifd, bytes, le);
        bool canDecode = false;
        foreach (var p in pages) if (p.Supported) { canDecode = true; break; }
        return new TiffReader(stream, ownsStream, bytes, le, pages.ToArray(), info, meta, canDecode);
    }

    private static TiffPageInfo BuildPageInfo(IfdEntry[] entries, byte[] bytes, bool le)
    {
        int width = (int)GetScalar(entries, 0x0100, bytes, le);
        int height = (int)GetScalar(entries, 0x0101, bytes, le);
        ushort[] bps = GetShortArray(entries, 0x0102, bytes, le);
        int bitsPerSample = bps.Length == 0 ? 1 : bps[0];
        int samplesPerPixel = (int)GetScalar(entries, 0x0115, bytes, le, def: 1);
        int compression = (int)GetScalar(entries, 0x0103, bytes, le, def: 1);
        int photometric = (int)GetScalar(entries, 0x0106, bytes, le, def: 0);

        bool supported = compression is 1 or 5 or 7 or 8 or 32773 or 32946 && bitsPerSample is 1 or 8 or 16;
        var pf = samplesPerPixel switch
        {
            1 when bitsPerSample == 8 => PixelFormat.Gray8,
            1 when bitsPerSample == 16 => PixelFormat.Gray16,
            1 when bitsPerSample == 1 => PixelFormat.Indexed1,
            3 => PixelFormat.Rgb24,
            4 => photometric == 5 ? PixelFormat.Cmyk32 : PixelFormat.Rgba32,
            _ => PixelFormat.Unknown,
        };

        return new TiffPageInfo
        {
            Ifd = entries,
            Width = width,
            Height = height,
            BitsPerSample = bitsPerSample,
            SamplesPerPixel = samplesPerPixel,
            Compression = compression,
            Photometric = photometric,
            PixelFormat = pf,
            Supported = supported,
            IsTiled = HasTag(entries, 0x0142) && HasTag(entries, 0x0143),
        };
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "This TIFF uses an unsupported compression scheme or pixel layout.");
        }

        for (int i = 0; i < _pages.Length; i++)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var page = _pages[i];
            if (!page.Supported) continue;

            int dstStride = page.Width * (page.BitsPerSample == 16 ? 2 : 1) * page.SamplesPerPixel;
            if (page.PixelFormat == PixelFormat.Indexed1) dstStride = (page.Width + 7) / 8;

            if (page.IsTiled)
            {
                yield return await DecodeTiled(page.Ifd, page.Width, page.Height,
                    page.SamplesPerPixel, page.BitsPerSample, page.Compression,
                    page.Photometric, page.PixelFormat, dstStride);
            }
            else
            {
                yield return DecodeStripped(page.Ifd, page.Width, page.Height,
                    page.SamplesPerPixel, page.BitsPerSample, page.Compression,
                    page.Photometric, page.PixelFormat, dstStride);
            }
        }
    }

    private ImageFrame DecodeStripped(IfdEntry[] ifd, int width, int height, int spp, int bps,
                                       int compression, int photometric, PixelFormat pf, int dstStride)
    {
        int rowsPerStrip = (int)GetScalar(ifd, 0x0116, _bytes, _littleEndian, def: (uint)height);
        uint[] stripOffsets = GetLongArray(ifd, 0x0111, _bytes, _littleEndian);
        uint[] stripByteCounts = GetLongArray(ifd, 0x0117, _bytes, _littleEndian);

        var (frame, buf) = ImageFrame.Rent(width, height, pf, dstStride);
        int dstRow = 0;
        for (int s = 0; s < stripOffsets.Length; s++)
        {
            byte[] strip = ReadStrip(_bytes, (int)stripOffsets[s], (int)stripByteCounts[s], compression);
            if (compression == 7)
            {
                // JPEG strip — decode the whole strip into RGB / Gray pixels at once.
                CopyJpegStrip(strip, buf, dstRow, width, dstStride, pf);
                int decodedRows = Math.Min(rowsPerStrip, height - dstRow);
                dstRow += decodedRows;
                continue;
            }
            int expectedRows = Math.Min(rowsPerStrip, height - dstRow);
            int srcStride = width * spp * (bps == 16 ? 2 : 1);
            if (pf == PixelFormat.Indexed1) srcStride = (width + 7) / 8;
            for (int r = 0; r < expectedRows; r++)
            {
                int srcOff = r * srcStride;
                int dstOff = (dstRow + r) * dstStride;
                if (srcOff + srcStride > strip.Length) break;
                if (photometric == 0 && pf == PixelFormat.Gray8)
                {
                    for (int i = 0; i < srcStride; i++)
                    {
                        buf[dstOff + i] = (byte)(255 - strip[srcOff + i]);
                    }
                }
                else
                {
                    Buffer.BlockCopy(strip, srcOff, buf, dstOff, srcStride);
                }
            }
            dstRow += expectedRows;
        }
        return frame;
    }

    private async Task<ImageFrame> DecodeTiled(IfdEntry[] ifd, int width, int height, int spp, int bps,
                                                int compression, int photometric, PixelFormat pf, int dstStride)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        int tileW = (int)GetScalar(ifd, 0x0142, _bytes, _littleEndian);
        int tileH = (int)GetScalar(ifd, 0x0143, _bytes, _littleEndian);
        uint[] tileOffsets = GetLongArray(ifd, 0x0144, _bytes, _littleEndian);
        uint[] tileByteCounts = GetLongArray(ifd, 0x0145, _bytes, _littleEndian);
        if (tileW <= 0 || tileH <= 0)
            throw new ImageFormatException("Invalid TIFF tile dimensions.");

        int tilesAcross = (width + tileW - 1) / tileW;
        int tilesDown = (height + tileH - 1) / tileH;
        if (tileOffsets.Length < tilesAcross * tilesDown)
            throw new ImageFormatException("Tile offset table shorter than tile grid.");

        var (frame, buf) = ImageFrame.Rent(width, height, pf, dstStride);
        int bytesPerPixel = (bps == 16 ? 2 : 1) * spp;
        int tileSrcStride = tileW * bytesPerPixel;

        for (int ty = 0; ty < tilesDown; ty++)
        {
            for (int tx = 0; tx < tilesAcross; tx++)
            {
                int t = ty * tilesAcross + tx;
                byte[] tileBytes = ReadStrip(
                    _bytes, (int)tileOffsets[t], (int)tileByteCounts[t], compression);

                int copyW = Math.Min(tileW, width - tx * tileW);
                int copyH = Math.Min(tileH, height - ty * tileH);

                if (compression == 7)
                {
                    // The tile is a complete JPEG; decode and BLT into the framebuffer.
                    CopyJpegTile(tileBytes, buf, tx * tileW, ty * tileH, copyW, copyH, width, dstStride, pf);
                    continue;
                }
                int copyBytes = copyW * bytesPerPixel;
                for (int r = 0; r < copyH; r++)
                {
                    int srcOff = r * tileSrcStride;
                    int dstOff = (ty * tileH + r) * dstStride + tx * tileW * bytesPerPixel;
                    if (srcOff + copyBytes > tileBytes.Length) break;
                    if (photometric == 0 && pf == PixelFormat.Gray8)
                    {
                        for (int i = 0; i < copyBytes; i++)
                            buf[dstOff + i] = (byte)(255 - tileBytes[srcOff + i]);
                    }
                    else
                    {
                        Buffer.BlockCopy(tileBytes, srcOff, buf, dstOff, copyBytes);
                    }
                }
            }
        }
        return frame;
    }

    private static void CopyJpegStrip(byte[] jpegBytes, byte[] dst, int dstRow,
                                       int imgWidth, int dstStride, PixelFormat pf)
    {
        using var ms = new MemoryStream(jpegBytes);
        using var reader = JpegReader.Open(ms, ownsStream: false);
        // Synchronously pull the (sole) frame; JpegReader is sync-friendly.
        var enumerator = reader.ReadFramesAsync().GetAsyncEnumerator();
        try
        {
            if (!enumerator.MoveNextAsync().AsTask().GetAwaiter().GetResult()) return;
            var frame = enumerator.Current;
            int rowsAvailable = Math.Min(frame.Height, (dst.Length / dstStride) - dstRow);
            int copyBytes = Math.Min(frame.Stride, imgWidth * BytesPerPixel(pf));
            for (int r = 0; r < rowsAvailable; r++)
            {
                int srcOff = r * frame.Stride;
                int dstOff = (dstRow + r) * dstStride;
                frame.Pixels.Span.Slice(srcOff, copyBytes).CopyTo(dst.AsSpan(dstOff));
            }
            frame.Dispose();
        }
        finally
        {
            enumerator.DisposeAsync().AsTask().GetAwaiter().GetResult();
        }
    }

    private static void CopyJpegTile(byte[] jpegBytes, byte[] dst, int dstX, int dstY,
                                      int copyW, int copyH, int imgWidth, int dstStride, PixelFormat pf)
    {
        using var ms = new MemoryStream(jpegBytes);
        using var reader = JpegReader.Open(ms, ownsStream: false);
        var enumerator = reader.ReadFramesAsync().GetAsyncEnumerator();
        try
        {
            if (!enumerator.MoveNextAsync().AsTask().GetAwaiter().GetResult()) return;
            var frame = enumerator.Current;
            int bpp = BytesPerPixel(pf);
            int actualW = Math.Min(copyW, frame.Width);
            int actualH = Math.Min(copyH, frame.Height);
            var srcSpan = frame.Pixels.Span;
            for (int r = 0; r < actualH; r++)
            {
                int srcOff = r * frame.Stride;
                int dstOff = (dstY + r) * dstStride + dstX * bpp;
                if (dstOff + actualW * bpp > dst.Length) break;
                srcSpan.Slice(srcOff, actualW * bpp).CopyTo(dst.AsSpan(dstOff));
            }
            frame.Dispose();
        }
        finally
        {
            enumerator.DisposeAsync().AsTask().GetAwaiter().GetResult();
        }
    }

    private static int BytesPerPixel(PixelFormat pf) => pf switch
    {
        PixelFormat.Gray8 => 1,
        PixelFormat.Gray16 => 2,
        PixelFormat.Rgb24 => 3,
        PixelFormat.Rgba32 or PixelFormat.Bgra32 or PixelFormat.Argb32 or PixelFormat.Cmyk32 => 4,
        PixelFormat.Indexed1 => 1,
        _ => 1,
    };

    private static bool HasTag(IfdEntry[] ifd, int tag)
    {
        for (int i = 0; i < ifd.Length; i++)
            if (ifd[i].Tag == tag) return true;
        return false;
    }

    private static byte[] ReadStrip(byte[] src, int offset, int length, int compression)
    {
        if (offset < 0 || length < 0 || offset + length > src.Length)
        {
            throw new ImageFormatException("Strip out of range.");
        }
        var stripBytes = new ReadOnlySpan<byte>(src, offset, length);
        return compression switch
        {
            1 => stripBytes.ToArray(),
            7 => stripBytes.ToArray(),  // raw JPEG payload, decoded by CopyJpegStrip / CopyJpegTile.
            8 or 32946 => DeflateDecode(stripBytes),
            32773 => PackBitsCodec.Decode(stripBytes),
            5 => LzwDecoder.DecodeTiff(stripBytes),
            _ => throw new NotSupportedException($"TIFF compression {compression} not implemented."),
        };
    }

    private static byte[] DeflateDecode(ReadOnlySpan<byte> input)
    {
        using var ms = new MemoryStream(input.ToArray());
        using var z = new ZLibStream(ms, CompressionMode.Decompress);
        using var output = new MemoryStream();
        z.CopyTo(output);
        return output.ToArray();
    }

    private static IfdEntry[] ParseIfd(byte[] b, bool le, int offset)
    {
        if (offset < 0 || offset + 2 > b.Length) throw new ImageFormatException("Bad IFD offset.");
        int n = ReadU16(b, offset, le);
        var arr = new IfdEntry[n];
        for (int i = 0; i < n; i++)
        {
            int o = offset + 2 + i * 12;
            arr[i] = new IfdEntry(
                ReadU16(b, o, le),
                ReadU16(b, o + 2, le),
                ReadU32(b, o + 4, le),
                ReadU32(b, o + 8, le));
        }
        return arr;
    }

    private static ImageMetadata BuildMetadata(IfdEntry[] entries, byte[] b, bool le)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        foreach (var e in entries)
        {
            string? name = TagName(e.Tag);
            if (name is null) continue;
            tags["TIFF:" + name] = FormatTagValue(e, b, le);
        }
        return new ImageMetadata
        {
            Software = tags.TryGetValue("TIFF:Software", out var sw) ? sw : null,
            Description = tags.TryGetValue("TIFF:ImageDescription", out var d) ? d : null,
            Copyright = tags.TryGetValue("TIFF:Copyright", out var c) ? c : null,
            Author = tags.TryGetValue("TIFF:Artist", out var a) ? a : null,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static string? TagName(int tag) => tag switch
    {
        0x0100 => "ImageWidth", 0x0101 => "ImageLength",
        0x0102 => "BitsPerSample", 0x0103 => "Compression",
        0x0106 => "Photometric", 0x010D => "DocumentName",
        0x010E => "ImageDescription", 0x010F => "Make",
        0x0110 => "Model", 0x0112 => "Orientation",
        0x0115 => "SamplesPerPixel", 0x0116 => "RowsPerStrip",
        0x011A => "XResolution", 0x011B => "YResolution",
        0x0128 => "ResolutionUnit", 0x0131 => "Software",
        0x0132 => "DateTime", 0x013B => "Artist",
        0x8298 => "Copyright",
        _ => null,
    };

    private static string FormatTagValue(IfdEntry e, byte[] b, bool le)
    {
        return e.Type switch
        {
            2 => ReadAscii(b, e.Count, e.ValueOffset, le),
            3 => e.Count == 1
                ? ((ushort)(le ? e.ValueOffset & 0xFFFF : (e.ValueOffset >> 16) & 0xFFFF))
                    .ToString(System.Globalization.CultureInfo.InvariantCulture)
                : "<short[]>",
            4 => e.Count == 1 ? e.ValueOffset.ToString(System.Globalization.CultureInfo.InvariantCulture) : "<long[]>",
            5 => ReadRational(b, e.Count, e.ValueOffset, le),
            _ => "<type-" + e.Type + ">",
        };
    }

    private static string ReadAscii(byte[] b, uint count, uint offset, bool le)
    {
        if (count <= 4)
        {
            Span<byte> tmp = stackalloc byte[4];
            if (le)
            {
                BinaryPrimitives.WriteUInt32LittleEndian(tmp, offset);
            }
            else
            {
                BinaryPrimitives.WriteUInt32BigEndian(tmp, offset);
            }
            int n = (int)count;
            while (n > 0 && tmp[n - 1] == 0) n--;
            return Encoding.ASCII.GetString(tmp[..n]);
        }
        if (offset + count > b.Length) return string.Empty;
        int len = (int)count;
        while (len > 0 && b[offset + len - 1] == 0) len--;
        return Encoding.ASCII.GetString(b, (int)offset, len);
    }

    private static string ReadRational(byte[] b, uint count, uint offset, bool le)
    {
        if (count == 1 && offset + 8 <= b.Length)
        {
            uint n = ReadU32(b, (int)offset, le);
            uint d = ReadU32(b, (int)offset + 4, le);
            return $"{n}/{d}";
        }
        return "<rational[]>";
    }

    private static uint GetScalar(IfdEntry[] ifd, int tag, byte[] b, bool le, uint def = 0)
    {
        for (int i = 0; i < ifd.Length; i++)
        {
            if (ifd[i].Tag != tag) continue;
            var e = ifd[i];
            if (e.Type == 3)
            {
                return (uint)(le ? e.ValueOffset & 0xFFFF : (e.ValueOffset >> 16) & 0xFFFF);
            }
            return e.ValueOffset;
        }
        return def;
    }

    private static ushort[] GetShortArray(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        for (int i = 0; i < ifd.Length; i++)
        {
            if (ifd[i].Tag != tag) continue;
            var e = ifd[i];
            int n = (int)e.Count;
            var arr = new ushort[n];
            if (n == 0) return arr;
            if (n * 2 <= 4)
            {
                Span<byte> tmp = stackalloc byte[4];
                if (le) BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
                else BinaryPrimitives.WriteUInt32BigEndian(tmp, e.ValueOffset);
                for (int k = 0; k < n; k++)
                {
                    arr[k] = le
                        ? BinaryPrimitives.ReadUInt16LittleEndian(tmp[(k * 2)..])
                        : BinaryPrimitives.ReadUInt16BigEndian(tmp[(k * 2)..]);
                }
            }
            else
            {
                for (int k = 0; k < n; k++)
                {
                    arr[k] = ReadU16(b, (int)e.ValueOffset + k * 2, le);
                }
            }
            return arr;
        }
        return [];
    }

    private static uint[] GetLongArray(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        for (int i = 0; i < ifd.Length; i++)
        {
            if (ifd[i].Tag != tag) continue;
            var e = ifd[i];
            int n = (int)e.Count;
            if (n == 0) return [];
            if (e.Type == 3)
            {
                var src = GetShortArray(ifd, tag, b, le);
                var dst = new uint[src.Length];
                for (int k = 0; k < src.Length; k++) dst[k] = src[k];
                return dst;
            }
            if (n == 1) return [e.ValueOffset];
            var arr = new uint[n];
            for (int k = 0; k < n; k++)
            {
                arr[k] = ReadU32(b, (int)e.ValueOffset + k * 4, le);
            }
            return arr;
        }
        return [];
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o))
           : BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o))
           : BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private readonly record struct IfdEntry(int Tag, int Type, uint Count, uint ValueOffset);

    private sealed class TiffPageInfo
    {
        public required IfdEntry[] Ifd { get; init; }
        public required int Width { get; init; }
        public required int Height { get; init; }
        public required int BitsPerSample { get; init; }
        public required int SamplesPerPixel { get; init; }
        public required int Compression { get; init; }
        public required int Photometric { get; init; }
        public required PixelFormat PixelFormat { get; init; }
        public required bool Supported { get; init; }
        public required bool IsTiled { get; init; }

        public TiffPage ToPublic() => new()
        {
            Width = Width,
            Height = Height,
            BitsPerSample = BitsPerSample,
            SamplesPerPixel = SamplesPerPixel,
            CompressionTag = Compression,
            Photometric = Photometric,
            PixelFormat = PixelFormat,
            IsTiled = IsTiled,
            CanDecodePixels = Supported,
        };
    }
}

/// <summary>
/// Public view of a single page (IFD) inside a multi-page TIFF.
/// </summary>
public sealed record TiffPage
{
    /// <summary>Width of this page, pixels.</summary>
    public required int Width { get; init; }

    /// <summary>Height of this page, pixels.</summary>
    public required int Height { get; init; }

    /// <summary>Bits per sample (typically 1, 8 or 16).</summary>
    public required int BitsPerSample { get; init; }

    /// <summary>Samples per pixel (1 = gray/indexed, 3 = RGB, 4 = RGBA/CMYK).</summary>
    public required int SamplesPerPixel { get; init; }

    /// <summary>TIFF compression tag value (1 = uncompressed, 5 = LZW, 7 = JPEG, 8/32946 = Deflate, 32773 = PackBits).</summary>
    public required int CompressionTag { get; init; }

    /// <summary>TIFF photometric-interpretation tag value (0 = WhiteIsZero, 1 = BlackIsZero, 2 = RGB, 5 = CMYK, …).</summary>
    public required int Photometric { get; init; }

    /// <summary>Decoded pixel format Mediar will emit for this page.</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>True if the page uses tiled layout (TileWidth/TileLength tags present).</summary>
    public required bool IsTiled { get; init; }

    /// <summary>True if Mediar can decode this page's pixel data.</summary>
    public required bool CanDecodePixels { get; init; }
}
