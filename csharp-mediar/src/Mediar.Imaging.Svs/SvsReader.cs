using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Svs;

/// <summary>
/// Reader for Aperio Whole-Slide Imaging (SVS) files. SVS is a multi-page
/// tiled TIFF used by Leica / Aperio whole-slide scanners; page 0 is the
/// baseline at full resolution, page 1 is the thumbnail, subsequent pages
/// are pyramid levels (typically 4× downsample steps) plus an optional
/// macro + label image.
/// </summary>
/// <remarks>
/// Because real SVS scans use compression code 7 (JPEG-in-TIFF) on tiled
/// layouts, full pixel decoding is gated on Mediar.Imaging.Tiff growing
/// tile + JPEG-in-TIFF support. Until then this reader parses every IFD,
/// extracts the Aperio vendor metadata string (AppMag, MPP, ScanScope ID,
/// time, user, region notes) and exposes the pyramid via
/// <see cref="Levels"/> so downstream code can pick a level.
/// </remarks>
public sealed class SvsReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Svs;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>
    /// Information for every pyramid / accessory page in the SVS, in file
    /// order. Index 0 is the baseline; the last entries are typically the
    /// macro and label snapshots.
    /// </summary>
    public IReadOnlyList<SvsLevel> Levels { get; }

    /// <summary>Aperio &quot;Image Library&quot; / vendor descriptor string for the baseline image.</summary>
    public string? VendorDescription { get; }

    private SvsReader(Stream s, bool owns, ImageInfo info, ImageMetadata meta,
                      IReadOnlyList<SvsLevel> levels, string? vendor)
    {
        _stream = s; _ownsStream = owns;
        Info = info; Metadata = meta;
        Levels = levels;
        VendorDescription = vendor;
    }

    /// <summary>Open an SVS file by path.</summary>
    public static SvsReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an SVS from a stream.</summary>
    public static SvsReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8) throw new ImageFormatException("Truncated SVS / TIFF.");

        bool le = bytes[0] == 'I' && bytes[1] == 'I';
        bool be = bytes[0] == 'M' && bytes[1] == 'M';
        if (!le && !be) throw new ImageFormatException("Bad TIFF byte-order mark.");
        int magic = ReadU16(bytes, 2, le);
        if (magic != 42) throw new ImageFormatException("BigTIFF SVS variants are not yet supported.");

        var levels = new List<SvsLevel>();
        string? firstDescription = null;

        uint ifdOffset = ReadU32(bytes, 4, le);
        while (ifdOffset != 0 && ifdOffset + 2 <= bytes.Length)
        {
            var entries = ParseIfd(bytes, le, (int)ifdOffset, out uint nextIfd);
            int width = (int)GetScalar(entries, 0x0100);
            int height = (int)GetScalar(entries, 0x0101);
            ushort[] bps = GetShortArray(entries, 0x0102, bytes, le);
            int bitsPerSample = bps.Length == 0 ? 1 : bps[0];
            int samplesPerPixel = (int)GetScalar(entries, 0x0115, def: 1);
            int compression = (int)GetScalar(entries, 0x0103, def: 1);
            string? description = GetAsciiTag(entries, 0x010E, bytes);
            bool isTiled = entries.Any(e => e.Tag == 0x0142);
            firstDescription ??= description;

            levels.Add(new SvsLevel
            {
                Width = width,
                Height = height,
                BitsPerPixel = bitsPerSample * samplesPerPixel,
                CompressionTag = compression,
                IsTiled = isTiled,
                Description = description,
            });
            ifdOffset = nextIfd;
        }

        if (levels.Count == 0) throw new ImageFormatException("No IFDs found in SVS.");

        var baseline = levels[0];
        var meta = BuildMetadata(firstDescription);
        var info = new ImageInfo
        {
            Width = baseline.Width,
            Height = baseline.Height,
            BitsPerPixel = baseline.BitsPerPixel,
            ChannelCount = baseline.BitsPerPixel / 8,
            PixelFormat = PixelFormat.Unknown,
            Format = ImageFormat.Svs,
            FrameCount = levels.Count,
            ColorSpace = "sRGB",
        };

        return new SvsReader(stream, ownsStream, info, meta, levels, firstDescription);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            "SVS pixel decoding requires tiled / JPEG-in-TIFF support, " +
            "which is not yet implemented. Use Info / Levels / Metadata for now.");

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static ImageMetadata BuildMetadata(string? description)
    {
        if (string.IsNullOrWhiteSpace(description))
            return ImageMetadata.Empty;

        var tags = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        string? title = null;
        string? capturedAtRaw = null;
        DateTimeOffset? captured = null;
        string? user = null;

        var parts = description.Split('|');
        if (parts.Length > 0) title = parts[0].Trim();
        for (int i = 1; i < parts.Length; i++)
        {
            var kv = parts[i].Split('=', 2);
            if (kv.Length != 2) continue;
            var k = kv[0].Trim();
            var v = kv[1].Trim();
            if (k.Length == 0) continue;
            tags[k] = v;
            if (k.Equals("User", StringComparison.OrdinalIgnoreCase)) user = v;
            else if (k.Equals("Date", StringComparison.OrdinalIgnoreCase)) capturedAtRaw = v + (capturedAtRaw is null ? string.Empty : " " + capturedAtRaw);
            else if (k.Equals("Time", StringComparison.OrdinalIgnoreCase)) capturedAtRaw = (capturedAtRaw ?? string.Empty) + " " + v;
        }

        if (capturedAtRaw is not null && DateTimeOffset.TryParse(capturedAtRaw.Trim(), out var dto)) captured = dto;

        return new ImageMetadata
        {
            Title = title,
            Author = user,
            Software = tags.TryGetValue("ScanScope ID", out var ssid) ? "Aperio ScanScope " + ssid : "Aperio ScanScope",
            CapturedAt = captured,
            CapturedAtRaw = capturedAtRaw?.Trim(),
            Tags = tags.ToFrozenDictionary(StringComparer.OrdinalIgnoreCase),
        };
    }

    private readonly record struct IfdEntry(int Tag, int Type, int Count, int ValueOrOffset);

    private static IfdEntry[] ParseIfd(byte[] bytes, bool le, int offset, out uint nextIfdOffset)
    {
        int n = ReadU16(bytes, offset, le);
        var arr = new IfdEntry[n];
        for (int i = 0; i < n; i++)
        {
            int p = offset + 2 + i * 12;
            arr[i] = new IfdEntry(
                ReadU16(bytes, p, le),
                ReadU16(bytes, p + 2, le),
                (int)ReadU32(bytes, p + 4, le),
                (int)ReadU32(bytes, p + 8, le));
        }
        nextIfdOffset = ReadU32(bytes, offset + 2 + n * 12, le);
        return arr;
    }

    private static long GetScalar(IfdEntry[] entries, int tag, long def = 0)
    {
        foreach (var e in entries)
        {
            if (e.Tag != tag) continue;
            return e.ValueOrOffset;
        }
        return def;
    }

    private static long GetScalar(IfdEntry[] entries, int tag) => GetScalar(entries, tag, 0);

    private static ushort[] GetShortArray(IfdEntry[] entries, int tag, byte[] bytes, bool le)
    {
        foreach (var e in entries)
        {
            if (e.Tag != tag) continue;
            if (e.Count <= 2 && e.Type == 3)
            {
                var inline = new ushort[e.Count];
                for (int i = 0; i < e.Count; i++)
                {
                    inline[i] = (ushort)ReadU16(bytes, (int)(((uint)e.ValueOrOffset) >> (16 - i * 16)) & 0xFFFF, le);
                }
                return inline;
            }
            var arr = new ushort[e.Count];
            for (int i = 0; i < e.Count; i++)
                arr[i] = (ushort)ReadU16(bytes, e.ValueOrOffset + i * 2, le);
            return arr;
        }
        return Array.Empty<ushort>();
    }

    private static string? GetAsciiTag(IfdEntry[] entries, int tag, byte[] bytes)
    {
        foreach (var e in entries)
        {
            if (e.Tag != tag || e.Type != 2) continue;
            int len = e.Count;
            if (len == 0) return null;
            int off = len <= 4 ? -1 : e.ValueOrOffset;
            if (off < 0)
            {
                Span<byte> inline = stackalloc byte[4];
                BinaryPrimitives.WriteInt32LittleEndian(inline, e.ValueOrOffset);
                int end = inline.IndexOf((byte)0);
                if (end < 0) end = Math.Min(len, 4);
                return Encoding.ASCII.GetString(inline[..end]);
            }
            int end2 = Math.Min(off + len, bytes.Length);
            int strEnd = Array.IndexOf(bytes, (byte)0, off, end2 - off);
            if (strEnd < 0) strEnd = end2;
            return Encoding.ASCII.GetString(bytes, off, strEnd - off);
        }
        return null;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int ReadU16(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o)) : BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o)) : BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));
}

/// <summary>
/// Describes a single pyramid / accessory image inside an SVS.
/// </summary>
public sealed record SvsLevel
{
    /// <summary>Width of this level, pixels.</summary>
    public int Width { get; init; }

    /// <summary>Height of this level, pixels.</summary>
    public int Height { get; init; }

    /// <summary>Bits per pixel.</summary>
    public int BitsPerPixel { get; init; }

    /// <summary>TIFF compression tag value: 7 = JPEG, 33003 = JPEG2000, 1 = uncompressed.</summary>
    public int CompressionTag { get; init; }

    /// <summary>True if the IFD declares TileWidth (tiled layout, common on baseline pages).</summary>
    public bool IsTiled { get; init; }

    /// <summary>The IFD's ImageDescription tag string, if present.</summary>
    public string? Description { get; init; }
}
