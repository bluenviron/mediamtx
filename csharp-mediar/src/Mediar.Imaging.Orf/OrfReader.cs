using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using Mediar.Imaging.Tiff;
using Mediar.Imaging.TiffRaw;
using static Mediar.Imaging.TiffRaw.TiffRawHelpers;

namespace Mediar.Imaging.Orf;

/// <summary>
/// Reader for Olympus RAW (ORF) files. ORF is TIFF-based and identified
/// by either an Olympus-specific magic word (0x4F52 / 0x5253 / big-endian
/// 0x524F) or the standard TIFF magic 0x002A combined with an EXIF
/// <c>Make</c> tag matching "OLYMPUS" or "OM Digital Solutions". The
/// reader composes <see cref="TiffReader"/> for pixel decode and exposes
/// the parsed Olympus-specific metadata block.
/// </summary>
/// <remarks>
/// <para>
/// Like NEF / ARW / PEF, ORF places small previews / thumbnails in IFD 0
/// and the full-resolution raw sensor data in either a SubIFD pointed at
/// by tag 0x014A or directly in IFD 1. SubIFDs are walked recursively to
/// populate <see cref="SubImages"/>.
/// </para>
/// <para>
/// Olympus packed RAW uses a custom 12-bit-into-16-bit scheme with a
/// look-up table stored in the Olympus MakerNote (tag 0x100 of the
/// "Equipment" sub-IFD). Mediar does not yet ship this codec - sub-
/// images that use it are reported as <c>CanDecodePixels = false</c>.
/// Uncompressed (tag 1) and standard JPEG-in-TIFF (tag 7) thumbnails /
/// previews decode through the existing TIFF stack.
/// </para>
/// </remarks>
public sealed class OrfReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly bool _littleEndian;
    private readonly int _olympusMagic;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Orf;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The Olympus-specific metadata block parsed from IFD 0.</summary>
    public OrfMetadata Orf { get; }

    /// <summary>All sub-images discovered in this ORF file (IFD 0 plus SubIFDs in walk order).</summary>
    public IReadOnlyList<OrfSubImageInfo> SubImages { get; }

    private OrfReader(Stream s, bool ownsStream, byte[] bytes, bool le, int olyMagic,
                     ImageInfo info, ImageMetadata meta, OrfMetadata orf,
                     IReadOnlyList<OrfSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream;
        _bytes = bytes; _littleEndian = le; _olympusMagic = olyMagic;
        Info = info; Metadata = meta; Orf = orf;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open an ORF file by path.</summary>
    public static OrfReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an ORF from a stream (the contents are buffered into memory).</summary>
    public static OrfReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8) throw new ImageFormatException("Truncated ORF.");

        bool le = bytes[0] == 'I' && bytes[1] == 'I';
        bool be = bytes[0] == 'M' && bytes[1] == 'M';
        if (!le && !be) throw new ImageFormatException("Bad ORF byte-order mark (expected II or MM).");

        int magic = ReadU16(bytes, 2, le);
        // Accept standard TIFF (0x002A), Olympus current 'RO' (LE 0x4F52, BE 0x524F = "OR"),
        // Olympus legacy 'RS' (LE 0x5352). Note these are the *LE/BE u16 readings* of the
        // on-disk byte sequence "RO" / "RS" / "OR"; the raw bytes themselves are always
        // 'R'/'S'/'O' at offsets 2-3 regardless of byte order.
        bool standardTiff = magic == 0x002A;
        bool olympusMagic = (le && magic is 0x4F52 or 0x5352) || (!le && magic is 0x4F52);
        if (!standardTiff && !olympusMagic)
        {
            throw new ImageFormatException(
                "Unsupported ORF magic 0x" + magic.ToString("X4", System.Globalization.CultureInfo.InvariantCulture) + ".");
        }

        // If non-standard, patch it to 0x002A in-place so the rest of the
        // reader (and TiffReader on delegation) can use the standard layout.
        if (olympusMagic)
        {
            if (le) BinaryPrimitives.WriteUInt16LittleEndian(bytes.AsSpan(2), 0x002A);
            else BinaryPrimitives.WriteUInt16BigEndian(bytes.AsSpan(2), 0x002A);
        }

        uint ifd0Offset = ReadU32(bytes, 4, le);
        if (ifd0Offset == 0) throw new ImageFormatException("ORF file has no IFDs.");

        var ifd0 = ParseIfd(bytes, le, (int)ifd0Offset);
        var orf = ParseOrfMetadata(ifd0, bytes, le, magic);

        // If the file used standard TIFF magic, require the Make tag to identify Olympus
        // (otherwise we can't distinguish from any other TIFF).
        if (standardTiff && (string.IsNullOrEmpty(orf.Make) || !IsOlympusMake(orf.Make)))
        {
            throw new ImageFormatException(
                "Not an ORF file (EXIF Make tag does not identify Olympus / OM Digital Solutions and magic is standard TIFF).");
        }

        var subs = new List<OrfSubImageInfo>();
        var visited = new HashSet<uint>();
        TiffRawHelpers.WalkIfdsRecursive<OrfSubImageInfo>(
            bytes, le, ifd0Offset, parentSubIfdLevel: 0, subs, visited,
            (entries, b, lo, lvl) => BuildSubImageInfo(entries, lvl, b, lo));

        var primary = SelectPrimary(subs);
        var info = new ImageInfo
        {
            Width = primary.Width,
            Height = primary.Height,
            BitsPerPixel = primary.BitsPerSample * primary.SamplesPerPixel,
            ChannelCount = primary.SamplesPerPixel,
            PixelFormat = primary.PixelFormat,
            Format = ImageFormat.Orf,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(orf);
        return new OrfReader(stream, ownsStream, bytes, le, magic, info, meta, orf,
                            subs, primary.CanDecodePixels);
    }

    private static bool IsOlympusMake(string make) =>
        make.StartsWith("OLYMPUS", StringComparison.Ordinal) ||
        make.StartsWith("OM Digital", StringComparison.OrdinalIgnoreCase) ||
        make.StartsWith("OM-Digital", StringComparison.OrdinalIgnoreCase);

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "This ORF file's primary image uses an unsupported compression scheme " +
                "(Olympus packed RAW requires the MakerNote 0x100 LUT, not yet implemented).");
        }
        cancellationToken.ThrowIfCancellationRequested();

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
                "ORF primary image was not produced by the underlying TIFF decoder.");
        }
        _ = _olympusMagic;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static OrfSubImageInfo SelectPrimary(IReadOnlyList<OrfSubImageInfo> subs)
    {
        OrfSubImageInfo? best = null;
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
        return best ?? throw new ImageFormatException("ORF file has no inspectable sub-images.");
    }

    private static OrfSubImageInfo BuildSubImageInfo(IfdEntry[] entries, int subIfdLevel, byte[] bytes, bool le)
    {
        int width = (int)GetScalar(entries, 0x0100, def: 0);
        int height = (int)GetScalar(entries, 0x0101, def: 0);
        ushort[] bps = GetShortArray(entries, 0x0102, bytes, le);
        int bitsPerSample = bps.Length == 0 ? 12 : bps[0];
        int samplesPerPixel = (int)GetScalar(entries, 0x0115, def: 1);
        int compression = (int)GetScalar(entries, 0x0103, def: 1);
        int photometric = (int)GetScalar(entries, 0x0106, def: 0);
        int newSubFileType = (int)GetScalar(entries, 0x00FE, def: 0);

        // Olympus packed RAW uses compression tag 1 but 12-bit-into-16-bit packing
        // with a MakerNote LUT - we can only safely decode when SamplesPerPixel == 3
        // (full RGB preview) or when BitsPerSample is a standard width.
        bool canDecode = compression is 1 or 5 or 7 or 8 or 32773 or 32946
                         && bitsPerSample is 1 or 8 or 16
                         && (samplesPerPixel >= 3 || photometric == 1 || photometric == 0);

        var pf = (samplesPerPixel, bitsPerSample) switch
        {
            (1, 8) => PixelFormat.Gray8,
            (1, 16) => PixelFormat.Gray16,
            (3, _) => PixelFormat.Rgb24,
            _ => PixelFormat.Unknown,
        };

        return new OrfSubImageInfo
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

    private static OrfMetadata ParseOrfMetadata(IfdEntry[] entries, byte[] bytes, bool le, int olyMagic)
    {
        var make = ReadAsciiTag(entries, 0x010F, bytes, le);
        var model = ReadAsciiTag(entries, 0x0110, bytes, le);
        var software = ReadAsciiTag(entries, 0x0131, bytes, le);
        var dateTime = ReadAsciiTag(entries, 0x0132, bytes, le);
        var artist = ReadAsciiTag(entries, 0x013B, bytes, le);
        var copyright = ReadAsciiTag(entries, 0x8298, bytes, le);
        int makerNoteLen = GetTagByteLength(entries, 0x927C);

        return new OrfMetadata
        {
            Make = make,
            Model = model,
            Software = software,
            DateTime = dateTime,
            Artist = artist,
            Copyright = copyright,
            OlympusMagic = olyMagic,
            MakerNoteLength = makerNoteLen,
        };
    }

    private static ImageMetadata BuildImageMetadata(OrfMetadata orf)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        if (orf.MakerNoteLength > 0)
        {
            tags["ORF:MakerNoteLength"] = orf.MakerNoteLength.ToString(System.Globalization.CultureInfo.InvariantCulture);
        }
        tags["ORF:Magic"] = "0x" + orf.OlympusMagic.ToString("X4", System.Globalization.CultureInfo.InvariantCulture);

        return new ImageMetadata
        {
            CameraMake = orf.Make,
            CameraModel = orf.Model,
            Software = orf.Software,
            CapturedAtRaw = orf.DateTime,
            Author = orf.Artist,
            Copyright = orf.Copyright,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

}
