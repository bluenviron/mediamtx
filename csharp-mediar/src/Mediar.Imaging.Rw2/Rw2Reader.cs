using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Tiff;

namespace Mediar.Imaging.Rw2;

/// <summary>
/// Reader for Panasonic / Leica RW2 RAW files. RW2 uses a TIFF-derived
/// container that swaps the magic word: byte-order mark "II" is followed
/// by 0x0055 (85) instead of TIFF's 0x002A (42). The IFD layout itself
/// is otherwise identical to TIFF, so once the magic is patched the
/// existing <see cref="TiffReader"/> can be used unchanged to walk the
/// directory structure and decode strips that use a known compression.
/// </summary>
/// <remarks>
/// <para>
/// Panasonic places the raw sensor data with proprietary compression
/// (TIFF tag 34316) in IFD 0 plus optional Panasonic-specific tags
/// 0x0001 (PanasonicRawVersion), 0x0002 / 0x0003 (SensorWidth /
/// SensorHeight), 0x0004 - 0x0007 (SensorBorders), 0x0009 (CFAPattern),
/// 0x000F - 0x0012 (CropTop/Left/Bottom/Right), 0x0017 (ISO).
/// </para>
/// <para>
/// Panasonic-compressed RAW (TIFF tag 34316) uses a proprietary delta +
/// shift packing scheme that Mediar does not yet ship. Sub-images using
/// that compression are reported as <c>CanDecodePixels = false</c>.
/// Standard TIFF compressions (1 = uncompressed, 7 = JPEG-in-TIFF) decode
/// through the existing TIFF stack.
/// </para>
/// </remarks>
public sealed class Rw2Reader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Rw2;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The Panasonic-specific metadata block parsed from IFD 0.</summary>
    public Rw2Metadata Rw2 { get; }

    /// <summary>All sub-images discovered in this RW2 file (IFD 0 plus SubIFDs).</summary>
    public IReadOnlyList<Rw2SubImageInfo> SubImages { get; }

    private Rw2Reader(Stream s, bool ownsStream, byte[] bytes,
                     ImageInfo info, ImageMetadata meta, Rw2Metadata rw2,
                     IReadOnlyList<Rw2SubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream;
        _bytes = bytes;
        Info = info; Metadata = meta; Rw2 = rw2;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open an RW2 file by path.</summary>
    public static Rw2Reader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an RW2 from a stream (the contents are buffered into memory).</summary>
    public static Rw2Reader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8) throw new ImageFormatException("Truncated RW2.");

        // RW2 = II + 0x55 0x00 + first-IFD offset (always little-endian).
        if (bytes[0] != 'I' || bytes[1] != 'I')
        {
            throw new ImageFormatException("RW2 must use II byte-order mark.");
        }
        int magic = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(2));
        if (magic != 0x0055)
        {
            throw new ImageFormatException("Not an RW2 file (expected magic 0x0055, got 0x" + magic.ToString("X4", System.Globalization.CultureInfo.InvariantCulture) + ").");
        }

        uint ifd0Offset = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(4));
        if (ifd0Offset == 0) throw new ImageFormatException("RW2 file has no IFDs.");

        var ifd0 = ParseIfd(bytes, (int)ifd0Offset);
        var rw2 = ParseRw2Metadata(ifd0, bytes);

        var subs = new List<Rw2SubImageInfo>();
        var visited = new HashSet<uint>();
        WalkIfdsRecursive(bytes, ifd0Offset, parentSubIfdLevel: 0, subs, visited);

        var primary = SelectPrimary(subs);
        var info = new ImageInfo
        {
            Width = primary.Width,
            Height = primary.Height,
            BitsPerPixel = primary.BitsPerSample * primary.SamplesPerPixel,
            ChannelCount = primary.SamplesPerPixel,
            PixelFormat = primary.PixelFormat,
            Format = ImageFormat.Rw2,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(rw2);
        return new Rw2Reader(stream, ownsStream, bytes, info, meta, rw2,
                            subs, primary.CanDecodePixels);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "This RW2 file's primary image uses an unsupported compression scheme " +
                "(Panasonic proprietary RAW / tag 34316 is not yet implemented).");
        }
        cancellationToken.ThrowIfCancellationRequested();

        // Patch the magic from 0x0055 to 0x002A so the existing TiffReader accepts it.
        byte[] patched = (byte[])_bytes.Clone();
        BinaryPrimitives.WriteUInt16LittleEndian(patched.AsSpan(2), 0x002A);

        using var ms = new MemoryStream(patched, writable: false);
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
                "RW2 primary image was not produced by the underlying TIFF decoder.");
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static Rw2SubImageInfo SelectPrimary(IReadOnlyList<Rw2SubImageInfo> subs)
    {
        Rw2SubImageInfo? best = null;
        long bestPixels = 0;
        foreach (var s in subs)
        {
            if (s.NewSubFileType == 0)
            {
                long px = (long)s.Width * s.Height;
                if (best is null || px > bestPixels) { best = s; bestPixels = px; }
            }
        }
        if (best is not null) return best;

        foreach (var s in subs)
        {
            long px = (long)s.Width * s.Height;
            if (best is null || px > bestPixels) { best = s; bestPixels = px; }
        }
        return best ?? throw new ImageFormatException("RW2 file has no inspectable sub-images.");
    }

    private static void WalkIfdsRecursive(byte[] bytes, uint ifdOffset,
                                          int parentSubIfdLevel,
                                          List<Rw2SubImageInfo> sink,
                                          HashSet<uint> visited)
    {
        while (ifdOffset != 0)
        {
            if (!visited.Add(ifdOffset)) return;
            if (ifdOffset + 2 > bytes.Length) return;
            var entries = ParseIfd(bytes, (int)ifdOffset);
            sink.Add(BuildSubImageInfo(entries, parentSubIfdLevel, bytes));

            foreach (var e in entries)
            {
                if (e.Tag != 0x014A) continue;
                var subOffsets = ReadLongArray(e, bytes);
                foreach (uint sub in subOffsets)
                {
                    WalkIfdsRecursive(bytes, sub, parentSubIfdLevel + 1, sink, visited);
                }
            }

            if (parentSubIfdLevel != 0) return;

            int n = entries.Length;
            int nextSlot = (int)ifdOffset + 2 + n * 12;
            if (nextSlot + 4 > bytes.Length) return;
            ifdOffset = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(nextSlot));
        }
    }

    private static Rw2SubImageInfo BuildSubImageInfo(IfdEntry[] entries, int subIfdLevel, byte[] bytes)
    {
        // RW2 IFD 0 stores the raw image geometry in Panasonic-specific tags 0x0002 (SensorWidth)
        // and 0x0003 (SensorHeight); the standard TIFF 0x0100 / 0x0101 tags may be absent for
        // the raw sub-image. Fall back to the Panasonic tags when the TIFF ones are missing.
        int width = (int)GetScalar(entries, 0x0100, def: 0);
        int height = (int)GetScalar(entries, 0x0101, def: 0);
        if (width == 0) width = (int)GetScalar(entries, 0x0002, def: 0);
        if (height == 0) height = (int)GetScalar(entries, 0x0003, def: 0);
        ushort[] bps = GetShortArray(entries, 0x0102, bytes);
        int bitsPerSample = bps.Length == 0 ? 12 : bps[0];
        int samplesPerPixel = (int)GetScalar(entries, 0x0115, def: 1);
        int compression = (int)GetScalar(entries, 0x0103, def: 1);
        int photometric = (int)GetScalar(entries, 0x0106, def: 0);
        int newSubFileType = (int)GetScalar(entries, 0x00FE, def: 0);

        // Panasonic-compressed RAW = 34316; not yet supported.
        bool canDecode = compression is 1 or 5 or 7 or 8 or 32773 or 32946
                         && bitsPerSample is 1 or 8 or 16;

        var pf = (samplesPerPixel, bitsPerSample) switch
        {
            (1, 8) => PixelFormat.Gray8,
            (1, 16) => PixelFormat.Gray16,
            (3, _) => PixelFormat.Rgb24,
            _ => PixelFormat.Unknown,
        };

        return new Rw2SubImageInfo
        {
            Width = width,
            Height = height,
            BitsPerSample = bitsPerSample,
            SamplesPerPixel = samplesPerPixel,
            CompressionTag = compression,
            Photometric = photometric,
            NewSubFileType = newSubFileType,
            PixelFormat = pf,
            SubIfdLevel = subIfdLevel,
            CanDecodePixels = canDecode,
        };
    }

    private static Rw2Metadata ParseRw2Metadata(IfdEntry[] entries, byte[] bytes)
    {
        var make = ReadAsciiTag(entries, 0x010F, bytes);
        var model = ReadAsciiTag(entries, 0x0110, bytes);
        var software = ReadAsciiTag(entries, 0x0131, bytes);
        var dateTime = ReadAsciiTag(entries, 0x0132, bytes);
        var artist = ReadAsciiTag(entries, 0x013B, bytes);
        var copyright = ReadAsciiTag(entries, 0x8298, bytes);
        var rawVersion = ReadAsciiTag(entries, 0x0001, bytes);

        int sensorWidth = (int)GetScalar(entries, 0x0002, def: 0);
        int sensorHeight = (int)GetScalar(entries, 0x0003, def: 0);
        int sensorTop = (int)GetScalar(entries, 0x0004, def: 0);
        int sensorLeft = (int)GetScalar(entries, 0x0005, def: 0);
        int sensorBottom = (int)GetScalar(entries, 0x0006, def: 0);
        int sensorRight = (int)GetScalar(entries, 0x0007, def: 0);
        int cfaPattern = (int)GetScalar(entries, 0x0009, def: 0);
        int cropTop = (int)GetScalar(entries, 0x000F, def: 0);
        int cropLeft = (int)GetScalar(entries, 0x0010, def: 0);
        int cropBottom = (int)GetScalar(entries, 0x0011, def: 0);
        int cropRight = (int)GetScalar(entries, 0x0012, def: 0);
        int iso = (int)GetScalar(entries, 0x0017, def: 0);

        return new Rw2Metadata
        {
            Make = make,
            Model = model,
            Software = software,
            DateTime = dateTime,
            Artist = artist,
            Copyright = copyright,
            PanasonicRawVersion = rawVersion,
            SensorWidth = sensorWidth,
            SensorHeight = sensorHeight,
            SensorTopBorder = sensorTop,
            SensorLeftBorder = sensorLeft,
            SensorBottomBorder = sensorBottom,
            SensorRightBorder = sensorRight,
            CfaPattern = cfaPattern,
            CropTop = cropTop,
            CropLeft = cropLeft,
            CropBottom = cropBottom,
            CropRight = cropRight,
            Iso = iso,
        };
    }

    private static ImageMetadata BuildImageMetadata(Rw2Metadata rw2)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        if (rw2.SensorWidth > 0)
        {
            tags["RW2:SensorWidth"] = rw2.SensorWidth.ToString(System.Globalization.CultureInfo.InvariantCulture);
        }
        if (rw2.SensorHeight > 0)
        {
            tags["RW2:SensorHeight"] = rw2.SensorHeight.ToString(System.Globalization.CultureInfo.InvariantCulture);
        }
        if (rw2.Iso > 0)
        {
            tags["RW2:ISO"] = rw2.Iso.ToString(System.Globalization.CultureInfo.InvariantCulture);
        }
        if (!string.IsNullOrEmpty(rw2.PanasonicRawVersion))
        {
            tags["RW2:Version"] = rw2.PanasonicRawVersion;
        }

        return new ImageMetadata
        {
            CameraMake = rw2.Make,
            CameraModel = rw2.Model,
            Software = rw2.Software,
            CapturedAtRaw = rw2.DateTime,
            Author = rw2.Artist,
            Copyright = rw2.Copyright,
            IsoSpeed = rw2.Iso > 0 ? rw2.Iso : null,
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

    private static uint[] ReadLongArray(IfdEntry e, byte[] b)
    {
        int n = (int)e.Count;
        if (n == 0) return [];
        if (n == 1) return [e.ValueOffset];
        var arr = new uint[n];
        for (int k = 0; k < n; k++)
        {
            arr[k] = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan((int)e.ValueOffset + k * 4));
        }
        return arr;
    }

    private static ushort[] GetShortArray(IfdEntry[] ifd, int tag, byte[] b)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            var arr = new ushort[n];
            if (n == 0) return arr;
            if (n * 2 <= 4)
            {
                Span<byte> tmp = stackalloc byte[4];
                BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
                for (int k = 0; k < n; k++)
                {
                    arr[k] = BinaryPrimitives.ReadUInt16LittleEndian(tmp[(k * 2)..]);
                }
            }
            else
            {
                for (int k = 0; k < n; k++)
                {
                    arr[k] = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan((int)e.ValueOffset + k * 2));
                }
            }
            return arr;
        }
        return [];
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
