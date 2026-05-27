using System.Buffers.Binary;
using System.Collections.Immutable;
using System.IO.Compression;

namespace Mediar.Imaging.Metafiles;

/// <summary>
/// Reader for the Windows Metafile family: EMF (Enhanced Metafile, MS-EMF),
/// WMF (legacy 16-bit Windows Metafile, MS-WMF), APM (Aldus Placeable
/// Metafile = 22-byte preamble + WMF), and the gzip-compressed wrappers
/// EMZ / WMZ. Exposes the record list, header metadata, and bounding box.
/// GDI playback (rasterization) is not implemented; <see cref="ReadFramesAsync"/>
/// throws.
/// </summary>
public sealed class MetafileReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format { get; }
    /// <inheritdoc/>
    public ImageInfo Info { get; }
    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;
    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>Decoded record list (record-type code + raw payload).</summary>
    public ImmutableArray<MetafileRecord> Records { get; }

    /// <summary>True if the source was gzip-compressed (.emz / .wmz).</summary>
    public bool WasCompressed { get; }

    /// <summary>True if the file is APM (Aldus Placeable Metafile) format.</summary>
    public bool IsPlaceable { get; }

    /// <summary>Bounding box in twips (1/1440 inch) for WMF/APM, device units for EMF.</summary>
    public (int Left, int Top, int Right, int Bottom) Bounds { get; }

    private MetafileReader(Stream s, bool owns, ImageFormat fmt, ImageInfo info,
                            ImmutableArray<MetafileRecord> records, bool compressed,
                            bool placeable, (int, int, int, int) bounds)
    {
        _stream = s; _ownsStream = owns;
        Format = fmt; Info = info;
        Records = records; WasCompressed = compressed; IsPlaceable = placeable; Bounds = bounds;
    }

    /// <summary>Open a metafile from a path.</summary>
    public static MetafileReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ImageFormatExtensions.FromExtension(path), ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a metafile from a stream.</summary>
    public static MetafileReader Open(Stream stream, ImageFormat expected = ImageFormat.Wmf, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] raw = ms.ToArray();

        bool compressed = raw.Length >= 2 && raw[0] == 0x1F && raw[1] == 0x8B;
        byte[] b;
        if (compressed)
        {
            using var src = new MemoryStream(raw);
            using var gz = new GZipStream(src, CompressionMode.Decompress);
            using var dst = new MemoryStream();
            gz.CopyTo(dst);
            b = dst.ToArray();
            // Refine format after unwrap.
            if (expected == ImageFormat.Emz) expected = ImageFormat.Emf;
            else if (expected == ImageFormat.Wmz) expected = ImageFormat.Wmf;
            else if (expected == ImageFormat.Svgz) throw new ImageFormatException("SVGZ is not a metafile; use SvgReader.");
        }
        else b = raw;

        return expected switch
        {
            ImageFormat.Emf => ParseEmf(stream, ownsStream, b, compressed, expected),
            ImageFormat.Apm => ParseApm(stream, ownsStream, b, compressed),
            _ => ParseWmf(stream, ownsStream, b, compressed, expected),
        };
    }

    private static MetafileReader ParseEmf(Stream s, bool owns, byte[] b, bool compressed, ImageFormat fmt)
    {
        if (b.Length < 88)
            throw new ImageFormatException("EMF file too short for EMR_HEADER.");
        uint recType = BinaryPrimitives.ReadUInt32LittleEndian(b);
        if (recType != 1)
            throw new ImageFormatException($"EMF first record must be EMR_HEADER (1), got {recType}.");
        int left = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(8));
        int top = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(12));
        int right = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(16));
        int bottom = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(20));

        var recs = ImmutableArray.CreateBuilder<MetafileRecord>();
        int p = 0;
        while (p + 8 <= b.Length)
        {
            uint type = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(p));
            uint size = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(p + 4));
            if (size < 8 || p + size > b.Length) break;
            recs.Add(new MetafileRecord((int)type, p, (int)size, b.AsSpan(p + 8, (int)size - 8).ToArray()));
            p += (int)size;
            if (type == 14 /* EMR_EOF */) break;
        }
        var info = new ImageInfo { Width = right - left, Height = bottom - top, Format = fmt, FrameCount = 1 };
        return new MetafileReader(s, owns, fmt, info, recs.ToImmutable(), compressed, false, (left, top, right, bottom));
    }

    private static MetafileReader ParseWmf(Stream s, bool owns, byte[] b, bool compressed, ImageFormat fmt)
    {
        if (b.Length < 18)
            throw new ImageFormatException("WMF file too short for META_HEADER.");
        ushort wmfType = BinaryPrimitives.ReadUInt16LittleEndian(b);
        if (wmfType != 1 && wmfType != 2)
            throw new ImageFormatException($"WMF Type field must be 1 or 2, got {wmfType}.");
        int headerSize = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(2)) * 2;
        var recs = ParseWmfRecords(b, headerSize);
        var info = new ImageInfo { Width = 0, Height = 0, Format = fmt, FrameCount = 1 };
        return new MetafileReader(s, owns, fmt, info, recs, compressed, false, (0, 0, 0, 0));
    }

    private static MetafileReader ParseApm(Stream s, bool owns, byte[] b, bool compressed)
    {
        if (b.Length < 22 || BinaryPrimitives.ReadUInt32LittleEndian(b) != 0x9AC6CDD7)
            throw new ImageFormatException("Not an Aldus Placeable Metafile (missing 0x9AC6CDD7 key).");
        // Placeable header: Key(4) + HWmf(2) + BoundingBox(8) + Inch(2) + Reserved(4) + Checksum(2)
        short left = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(6));
        short top = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(8));
        short right = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(10));
        short bottom = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(12));
        ushort inch = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(14));
        double dx = inch == 0 ? 0 : (double)(right - left) / inch;
        double dy = inch == 0 ? 0 : (double)(bottom - top) / inch;

        // The remaining bytes (offset 22 onwards) are a standard WMF.
        const int wmfStart = 22;
        if (b.Length < wmfStart + 18)
            throw new ImageFormatException("APM body too short for META_HEADER.");
        int headerSize = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(wmfStart + 2)) * 2;
        var recs = ParseWmfRecords(b.AsSpan(wmfStart).ToArray(), headerSize);

        var info = new ImageInfo
        {
            Width = right - left,
            Height = bottom - top,
            HorizontalDpi = dx == 0 ? 0 : dx * 96.0,  // px at 96 DPI default
            VerticalDpi = dy == 0 ? 0 : dy * 96.0,
            Format = ImageFormat.Apm,
            FrameCount = 1,
        };
        return new MetafileReader(s, owns, ImageFormat.Apm, info, recs, compressed, true, (left, top, right, bottom));
    }

    private static ImmutableArray<MetafileRecord> ParseWmfRecords(byte[] b, int startOffset)
    {
        var recs = ImmutableArray.CreateBuilder<MetafileRecord>();
        int p = startOffset;
        while (p + 6 <= b.Length)
        {
            uint sizeWords = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(p));
            ushort func = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(p + 4));
            long sizeBytes = (long)sizeWords * 2;
            if (sizeBytes < 6 || p + sizeBytes > b.Length) break;
            int payloadLen = (int)sizeBytes - 6;
            recs.Add(new MetafileRecord(func, p, (int)sizeBytes, b.AsSpan(p + 6, payloadLen).ToArray()));
            p += (int)sizeBytes;
            if (func == 0) break;  // META_EOF
        }
        return recs.ToImmutable();
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            "Metafile rasterization (GDI playback) is not implemented in this Mediar release. " +
            "Use the Records property to interpret or re-emit the metafile programmatically.");

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}

/// <summary>A single metafile record.</summary>
public sealed record MetafileRecord(int RecordType, int Offset, int Length, byte[] Payload);
