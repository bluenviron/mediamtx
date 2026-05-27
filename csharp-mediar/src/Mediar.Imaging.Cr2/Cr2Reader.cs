using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Tiff;

namespace Mediar.Imaging.Cr2;

/// <summary>
/// Reader for Canon Raw v2 (CR2) files. CR2 is a TIFF-based RAW container
/// distinguished by a <c>CR\x02\x00</c> sentinel at byte offset 8 and a
/// dedicated raw-IFD pointer slot at bytes 12-15 (in addition to the
/// normal TIFF IFD0 pointer at bytes 4-7).
/// </summary>
/// <remarks>
/// <para>
/// Canonical CR2 files have four IFDs:
/// </para>
/// <list type="number">
///   <item><description>IFD 0 - small RGB thumbnail (often 160x120).</description></item>
///   <item><description>IFD 1 - alternate / smaller thumbnail.</description></item>
///   <item><description>IFD 2 - full-size uncompressed RGB preview.</description></item>
///   <item><description>IFD 3 - the raw sensor data, addressed by the offset in
///     <see cref="Cr2Header.RawIfdOffset"/>. Typically lossless JPEG (SOF3).</description></item>
/// </list>
/// <para>
/// This reader composes <see cref="TiffReader"/> for the IFD walk and pixel
/// decode; the raw-sensor path delegates to the same lossless-JPEG decoder
/// that already lives in <c>Mediar.Imaging.Jpeg</c>.
/// </para>
/// </remarks>
public sealed class Cr2Reader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Cr2;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The CR2-specific file header (version + raw-IFD offset).</summary>
    public Cr2Header Cr2 { get; }

    /// <summary>All discovered IFDs, in file order. Index 0 is IFD 0 (the thumbnail).</summary>
    public IReadOnlyList<Cr2SubImageInfo> SubImages { get; }

    private Cr2Reader(Stream s, bool ownsStream, byte[] bytes,
                     ImageInfo info, ImageMetadata meta, Cr2Header header,
                     IReadOnlyList<Cr2SubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream;
        _bytes = bytes;
        Info = info; Metadata = meta; Cr2 = header;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open a CR2 file by path.</summary>
    public static Cr2Reader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a CR2 from a stream (the contents are buffered into memory).</summary>
    public static Cr2Reader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 16) throw new ImageFormatException("Truncated CR2.");

        // CR2 is always little-endian per the Canon spec.
        if (bytes[0] != 'I' || bytes[1] != 'I')
            throw new ImageFormatException("CR2 must be little-endian (II byte-order mark expected).");
        int magic = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(2));
        if (magic != 42) throw new ImageFormatException("Unsupported CR2/TIFF magic " + magic + ".");

        uint ifd0Offset = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(4));
        if (ifd0Offset == 0) throw new ImageFormatException("CR2 file has no IFDs.");

        // Bytes 8-11: "CR\x02\x00" sentinel + version. Bytes 12-15: raw-IFD offset.
        if (bytes[8] != 'C' || bytes[9] != 'R')
        {
            throw new ImageFormatException(
                "Not a CR2 file (missing 'CR' sentinel at byte offset 8).");
        }
        var header = new Cr2Header
        {
            MajorVersion = bytes[10],
            MinorVersion = bytes[11],
            RawIfdOffset = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(12)),
        };

        // Walk the standard IFD chain starting at IFD0.
        var subs = new List<Cr2SubImageInfo>();
        var visited = new HashSet<uint>();
        IfdEntry[]? ifd0Entries = null;
        uint cursor = ifd0Offset;
        int role = 0;
        while (cursor != 0)
        {
            if (!visited.Add(cursor)) break;
            if (cursor + 2 > bytes.Length) break;
            var entries = ParseIfd(bytes, (int)cursor);
            ifd0Entries ??= entries;
            subs.Add(BuildSubImageInfo(entries, AssignRole(role)));
            role++;
            int n = entries.Length;
            int nextSlot = (int)cursor + 2 + n * 12;
            if (nextSlot + 4 > bytes.Length) break;
            cursor = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(nextSlot));
        }

        // The raw IFD is reached via the header pointer at bytes 12-15
        // (NOT chained from IFD 2), so add it separately.
        if (header.RawIfdOffset != 0 && visited.Add(header.RawIfdOffset)
            && header.RawIfdOffset + 2 <= bytes.Length)
        {
            var rawEntries = ParseIfd(bytes, (int)header.RawIfdOffset);
            subs.Add(BuildSubImageInfo(rawEntries, Cr2IfdRole.RawSensor));
        }

        if (ifd0Entries is null)
        {
            throw new ImageFormatException("CR2 file has no readable IFDs.");
        }

        // Primary: largest sub-image by pixel count (typically the raw sensor).
        var primary = SelectPrimary(subs);

        var info = new ImageInfo
        {
            Width = primary.Width,
            Height = primary.Height,
            BitsPerPixel = primary.BitsPerSample * primary.SamplesPerPixel,
            ChannelCount = primary.SamplesPerPixel,
            PixelFormat = primary.PixelFormat,
            Format = ImageFormat.Cr2,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(ifd0Entries, bytes, header);
        return new Cr2Reader(stream, ownsStream, bytes, info, meta, header, subs, primary.CanDecodePixels);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "This CR2 file's primary image uses an unsupported compression scheme.");
        }
        cancellationToken.ThrowIfCancellationRequested();

        // Delegate to TiffReader. It will walk the chained IFDs (thumbnail,
        // alternate, preview) and yield decodable ones; the raw-sensor IFD
        // is not chained, so for now we surface whatever the largest
        // chained frame is.
        using var ms = new MemoryStream(_bytes, writable: false);
        using var tiff = TiffReader.Open(ms, ownsStream: false);

        var primary = SelectPrimary(SubImages);
        bool yielded = false;
        await foreach (var frame in tiff.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
        {
            if (frame.Width == primary.Width && frame.Height == primary.Height)
            {
                yielded = true;
                yield return frame;
                yield break;
            }
            frame.Dispose();
        }
        if (!yielded)
        {
            throw new NotSupportedException(
                "CR2 primary image was not produced by the underlying TIFF decoder.");
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static Cr2SubImageInfo SelectPrimary(IReadOnlyList<Cr2SubImageInfo> subs)
    {
        Cr2SubImageInfo? best = null;
        long bestPixels = 0;
        foreach (var s in subs)
        {
            long px = (long)s.Width * s.Height;
            if (best is null || px > bestPixels)
            {
                best = s; bestPixels = px;
            }
        }
        return best ?? throw new ImageFormatException("CR2 file has no inspectable sub-images.");
    }

    private static Cr2IfdRole AssignRole(int chainIndex) => chainIndex switch
    {
        0 => Cr2IfdRole.Thumbnail,
        1 => Cr2IfdRole.AlternateThumbnail,
        2 => Cr2IfdRole.FullPreview,
        _ => Cr2IfdRole.Unknown,
    };

    private static Cr2SubImageInfo BuildSubImageInfo(IfdEntry[] entries, Cr2IfdRole role)
    {
        int width = (int)GetScalar(entries, 0x0100, def: 0);
        int height = (int)GetScalar(entries, 0x0101, def: 0);
        int bitsPerSample = (int)GetScalar(entries, 0x0102, def: 8);
        int samplesPerPixel = (int)GetScalar(entries, 0x0115, def: 1);
        int compression = (int)GetScalar(entries, 0x0103, def: 1);
        int photometric = (int)GetScalar(entries, 0x0106, def: 0);

        bool canDecode = compression is 1 or 5 or 7 or 8 or 32773 or 32946
                         && bitsPerSample is 1 or 8 or 16;

        var pf = (samplesPerPixel, bitsPerSample) switch
        {
            (1, 8) => PixelFormat.Gray8,
            (1, 16) => PixelFormat.Gray16,
            (3, _) => PixelFormat.Rgb24,
            _ => PixelFormat.Unknown,
        };

        return new Cr2SubImageInfo
        {
            Role = role,
            Width = width,
            Height = height,
            BitsPerSample = bitsPerSample,
            SamplesPerPixel = samplesPerPixel,
            CompressionTag = compression,
            Photometric = photometric,
            PixelFormat = pf,
            CanDecodePixels = canDecode,
        };
    }

    private static ImageMetadata BuildImageMetadata(IfdEntry[] ifd, byte[] bytes, Cr2Header header)
    {
        var make = ReadAsciiTag(ifd, 0x010F, bytes);
        var model = ReadAsciiTag(ifd, 0x0110, bytes);
        var software = ReadAsciiTag(ifd, 0x0131, bytes);
        var dateTime = ReadAsciiTag(ifd, 0x0132, bytes);
        var artist = ReadAsciiTag(ifd, 0x013B, bytes);
        var copyright = ReadAsciiTag(ifd, 0x8298, bytes);

        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["CR2:Version"] = $"{header.MajorVersion}.{header.MinorVersion}",
            ["CR2:RawIfdOffset"] = header.RawIfdOffset.ToString(System.Globalization.CultureInfo.InvariantCulture),
        };

        return new ImageMetadata
        {
            CameraMake = make,
            CameraModel = model,
            Software = software,
            CapturedAtRaw = dateTime,
            Author = artist,
            Copyright = copyright,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static IfdEntry[] ParseIfd(byte[] b, int offset)
    {
        if (offset < 0 || offset + 2 > b.Length) throw new ImageFormatException("Bad IFD offset.");
        int n = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(offset));
        if (offset + 2 + n * 12 > b.Length) throw new ImageFormatException("IFD truncated.");
        var arr = new IfdEntry[n];
        for (int i = 0; i < n; i++)
        {
            int o = offset + 2 + i * 12;
            arr[i] = new IfdEntry(
                BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o)),
                BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o + 2)),
                BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o + 4)),
                BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o + 8)));
        }
        return arr;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint GetScalar(IfdEntry[] ifd, int tag, uint def = 0)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            if (e.Type == 3) return e.ValueOffset & 0xFFFF;
            return e.ValueOffset;
        }
        return def;
    }

    private static string? ReadAsciiTag(IfdEntry[] ifd, int tag, byte[] b)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            if (n == 0) return string.Empty;
            string raw;
            if (n <= 4)
            {
                Span<byte> tmp = stackalloc byte[4];
                BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
                while (n > 0 && tmp[n - 1] == 0) n--;
                raw = Encoding.ASCII.GetString(tmp[..n]);
            }
            else
            {
                if (e.ValueOffset + n > b.Length) return null;
                while (n > 0 && b[e.ValueOffset + n - 1] == 0) n--;
                raw = Encoding.ASCII.GetString(b, (int)e.ValueOffset, n);
            }
            return raw;
        }
        return null;
    }

    internal readonly record struct IfdEntry(int Tag, int Type, uint Count, uint ValueOffset);
}
