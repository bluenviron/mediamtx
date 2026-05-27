using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using Mediar.Imaging.Tiff;

namespace Mediar.Imaging.TiffRaw;

/// <summary>
/// Per-format configuration passed into <see cref="TiffRawReader.OpenStandard"/>.
/// Captures the small set of values that distinguish one TIFF-RAW dialect
/// from another (format identifier, brand display name, the
/// vendor-proprietary compression tag we currently surface as
/// "undecodable", and the Make-tag predicate the reader uses to confirm
/// that a TIFF file is actually a member of this format).
/// </summary>
public sealed record TiffRawConfig
{
    /// <summary>
    /// <see cref="ImageFormat"/> value this configuration corresponds to.
    /// Stamped onto <see cref="TiffRawReader.Info"/> and surfaced via
    /// <see cref="IImageReader.Format"/>.
    /// </summary>
    public required ImageFormat Format { get; init; }

    /// <summary>
    /// Short uppercase format label used in exception messages (e.g.
    /// "SRW", "NEF", "ARW", "ERF"). Should match the file extension.
    /// </summary>
    public required string FormatLabel { get; init; }

    /// <summary>
    /// Brand display name baked into exception messages and the
    /// proprietary-compression error (e.g. "Samsung", "Nikon", "Epson").
    /// </summary>
    public required string BrandName { get; init; }

    /// <summary>
    /// The vendor-proprietary TIFF compression tag for this format
    /// (Nikon 34713, Sony 32770, Mamiya 65000, Leaf 34713, Epson 65535).
    /// Sub-images using this compression tag are reported as
    /// <c>CanDecodePixels = false</c> with a brand-specific
    /// <see cref="NotSupportedException"/> message on
    /// <see cref="TiffRawReader.ReadFramesAsync"/>.
    /// </summary>
    public required int ProprietaryCompressionTag { get; init; }

    /// <summary>
    /// Predicate that returns <c>true</c> when the EXIF Make tag string
    /// matches this format's accepted manufacturer prefixes. Typically
    /// a small disjunction of <c>StringComparison.Ordinal</c>
    /// <c>StartsWith</c> checks.
    /// </summary>
    public required Func<string, bool> IsMatchingMake { get; init; }

    /// <summary>
    /// Optional vendor-specific rule that classifies an otherwise-decodable
    /// sub-image as undecodable. Receives (compression, photometric,
    /// bitsPerSample); return <c>true</c> to force
    /// <c>CanDecodePixels = false</c>. Used by 3FR to flag any CFA
    /// sub-image whose compression tag is not "uncompressed", since
    /// Hasselblad ships its proprietary 12/14-bit packing under
    /// generic deflate.
    /// </summary>
    public Func<int, int, int, bool>? IsVendorUndecodable { get; init; }
}

/// <summary>
/// Concrete TIFF-RAW reader that handles every standard TIFF-with-magic-42
/// camera-RAW dialect. Per-format projects (NEF, ARW, SRW, ERF, ...)
/// become thin static factories that hand a <see cref="TiffRawConfig"/>
/// to <see cref="OpenStandard"/>; the parse, IFD walk, SubIFD recursion,
/// Make-tag validation, metadata extraction and primary-sub-image
/// selection all happen here.
/// </summary>
public sealed class TiffRawReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly bool _littleEndian;
    private readonly TiffRawConfig _config;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _config.Format;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The EXIF / IFD 0 metadata block parsed from this TIFF-RAW file.</summary>
    public TiffRawMetadata Raw { get; }

    /// <summary>All sub-images discovered in this file (IFD 0 plus SubIFDs in walk order).</summary>
    public IReadOnlyList<TiffRawSubImageInfo> SubImages { get; }

    private TiffRawReader(Stream s, bool ownsStream, byte[] bytes, bool le, TiffRawConfig config,
                          ImageInfo info, ImageMetadata meta, TiffRawMetadata raw,
                          IReadOnlyList<TiffRawSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream;
        _bytes = bytes; _littleEndian = le; _config = config;
        Info = info; Metadata = meta; Raw = raw;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>
    /// Open a TIFF-RAW file by stream using a standard TIFF header
    /// (II/MM byte-order mark + magic 0x002A + IFD 0 offset).
    /// </summary>
    /// <param name="stream">Source stream; contents are buffered into memory.</param>
    /// <param name="config">Per-format configuration (see <see cref="TiffRawConfig"/>).</param>
    /// <param name="ownsStream">Whether the reader should dispose <paramref name="stream"/> on <see cref="Dispose"/>.</param>
    /// <exception cref="ImageFormatException">The header is malformed, magic is wrong, or the EXIF Make tag does not satisfy <see cref="TiffRawConfig.IsMatchingMake"/>.</exception>
    public static TiffRawReader OpenStandard(Stream stream, TiffRawConfig config, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        ArgumentNullException.ThrowIfNull(config);

        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        return OpenAlreadyBuffered(stream, ownsStream, bytes, config);
    }

    /// <summary>
    /// Open a TIFF-RAW from an already-buffered byte array using a
    /// standard TIFF header. Used by quirky readers (RW2, ORF) that
    /// need to patch the magic word before delegating the IFD walk to
    /// the shared infrastructure.
    /// </summary>
    public static TiffRawReader OpenAlreadyBuffered(Stream stream, bool ownsStream, byte[] bytes, TiffRawConfig config)
    {
        ArgumentNullException.ThrowIfNull(stream);
        ArgumentNullException.ThrowIfNull(bytes);
        ArgumentNullException.ThrowIfNull(config);

        if (bytes.Length < 8) throw new ImageFormatException($"Truncated {config.FormatLabel}.");

        bool le = bytes[0] == 'I' && bytes[1] == 'I';
        bool be = bytes[0] == 'M' && bytes[1] == 'M';
        if (!le && !be) throw new ImageFormatException($"Bad {config.FormatLabel} byte-order mark (expected II or MM).");
        int magic = TiffRawHelpers.ReadU16(bytes, 2, le);
        if (magic != 42) throw new ImageFormatException($"Unsupported {config.FormatLabel}/TIFF magic {magic}.");

        uint ifd0Offset = TiffRawHelpers.ReadU32(bytes, 4, le);
        if (ifd0Offset == 0) throw new ImageFormatException($"{config.FormatLabel} file has no IFDs.");

        var ifd0 = TiffRawHelpers.ParseIfd(bytes, le, (int)ifd0Offset);
        var rawMeta = ParseTiffRawMetadata(ifd0, bytes, le);

        if (string.IsNullOrEmpty(rawMeta.Make) || !config.IsMatchingMake(rawMeta.Make))
        {
            throw new ImageFormatException(
                $"Not a {config.FormatLabel} file (EXIF Make tag does not identify a {config.BrandName} camera).");
        }

        var subs = new List<TiffRawSubImageInfo>();
        var visited = new HashSet<uint>();
        WalkIfdsRecursive(bytes, le, ifd0Offset, parentSubIfdLevel: 0, subs, visited, config);

        var primary = SelectPrimary(subs, config.FormatLabel);
        var info = new ImageInfo
        {
            Width = primary.Width,
            Height = primary.Height,
            BitsPerPixel = primary.BitsPerSample * primary.SamplesPerPixel,
            ChannelCount = primary.SamplesPerPixel,
            PixelFormat = primary.PixelFormat,
            Format = config.Format,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(rawMeta);
        return new TiffRawReader(stream, ownsStream, bytes, le, config, info, meta, rawMeta,
                                  subs, primary.CanDecodePixels);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                $"This {_config.FormatLabel} file's primary image uses an unsupported compression scheme " +
                $"({_config.BrandName}-compressed RAW / tag {_config.ProprietaryCompressionTag} is not yet implemented).");
        }
        cancellationToken.ThrowIfCancellationRequested();

        using var ms = new MemoryStream(_bytes, writable: false);
        using var tiff = TiffReader.Open(ms, ownsStream: false);

        var primary = SelectPrimary(SubImages, _config.FormatLabel);
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
                $"{_config.FormatLabel} primary image was not produced by the underlying TIFF decoder.");
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    /// <summary>
    /// Pick the "best" sub-image to surface as the primary frame. We
    /// prefer the largest sub-image with <c>NewSubFileType == 0</c>
    /// (i.e. the full-resolution sensor mosaic); if none has that flag
    /// we fall back to the largest sub-image overall.
    /// </summary>
    public static TiffRawSubImageInfo SelectPrimary(IReadOnlyList<TiffRawSubImageInfo> subs, string formatLabel)
    {
        TiffRawSubImageInfo? best = null;
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
        return best ?? throw new ImageFormatException($"{formatLabel} file has no inspectable sub-images.");
    }

    /// <summary>
    /// Walk an IFD chain (root + nested SubIFDs reachable via tag
    /// 0x014A) recursively into <paramref name="sink"/>. The
    /// <paramref name="visited"/> set provides cycle protection against
    /// malformed files that loop back on themselves.
    /// </summary>
    public static void WalkIfdsRecursive(byte[] bytes, bool le, uint ifdOffset,
                                          int parentSubIfdLevel,
                                          List<TiffRawSubImageInfo> sink,
                                          HashSet<uint> visited,
                                          TiffRawConfig config)
    {
        while (ifdOffset != 0)
        {
            if (!visited.Add(ifdOffset)) return;
            if (ifdOffset + 2 > bytes.Length) return;
            var entries = TiffRawHelpers.ParseIfd(bytes, le, (int)ifdOffset);
            sink.Add(BuildSubImageInfo(entries, parentSubIfdLevel, bytes, le, config));

            foreach (var e in entries)
            {
                if (e.Tag != 0x014A) continue;
                var subOffsets = TiffRawHelpers.ReadLongArray(e, bytes, le);
                foreach (uint sub in subOffsets)
                {
                    WalkIfdsRecursive(bytes, le, sub, parentSubIfdLevel + 1, sink, visited, config);
                }
            }

            if (parentSubIfdLevel != 0) return;

            int n = entries.Length;
            int nextSlot = (int)ifdOffset + 2 + n * 12;
            if (nextSlot + 4 > bytes.Length) return;
            ifdOffset = TiffRawHelpers.ReadU32(bytes, nextSlot, le);
        }
    }

    /// <summary>
    /// Build a <see cref="TiffRawSubImageInfo"/> from a parsed IFD. Marks
    /// the sub-image as <c>CanDecodePixels = false</c> when the
    /// compression tag is the vendor-proprietary one
    /// (<see cref="TiffRawConfig.ProprietaryCompressionTag"/>), when the
    /// vendor's optional <see cref="TiffRawConfig.IsVendorUndecodable"/>
    /// rule fires, or when the compression / bit-depth combination is
    /// not yet supported by <see cref="TiffReader"/>.
    /// </summary>
    public static TiffRawSubImageInfo BuildSubImageInfo(IfdEntry[] entries, int subIfdLevel, byte[] bytes, bool le,
                                                         TiffRawConfig config)
    {
        int width = (int)TiffRawHelpers.GetScalar(entries, 0x0100, def: 0);
        int height = (int)TiffRawHelpers.GetScalar(entries, 0x0101, def: 0);
        ushort[] bps = TiffRawHelpers.GetShortArray(entries, 0x0102, bytes, le);
        int bitsPerSample = bps.Length == 0 ? 8 : bps[0];
        int samplesPerPixel = (int)TiffRawHelpers.GetScalar(entries, 0x0115, def: 1);
        int compression = (int)TiffRawHelpers.GetScalar(entries, 0x0103, def: 1);
        int photometric = (int)TiffRawHelpers.GetScalar(entries, 0x0106, def: 0);
        int newSubFileType = (int)TiffRawHelpers.GetScalar(entries, 0x00FE, def: 0);

        bool vendorBlocked = config.IsVendorUndecodable?.Invoke(compression, photometric, bitsPerSample) ?? false;
        bool canDecode = compression != config.ProprietaryCompressionTag
                         && !vendorBlocked
                         && compression is 1 or 5 or 7 or 8 or 32773 or 32946
                         && bitsPerSample is 1 or 8 or 16;

        var pf = (samplesPerPixel, bitsPerSample) switch
        {
            (1, 8) => PixelFormat.Gray8,
            (1, 16) => PixelFormat.Gray16,
            (3, _) => PixelFormat.Rgb24,
            _ => PixelFormat.Unknown,
        };

        return new TiffRawSubImageInfo
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

    /// <summary>
    /// Read the standard EXIF / IFD 0 metadata tags (Make/Model/Software/
    /// DateTime/Artist/Copyright/MakerNote-length) into a
    /// <see cref="TiffRawMetadata"/>.
    /// </summary>
    public static TiffRawMetadata ParseTiffRawMetadata(IfdEntry[] entries, byte[] bytes, bool le)
    {
        return new TiffRawMetadata
        {
            Make = TiffRawHelpers.ReadAsciiTag(entries, 0x010F, bytes, le),
            Model = TiffRawHelpers.ReadAsciiTag(entries, 0x0110, bytes, le),
            Software = TiffRawHelpers.ReadAsciiTag(entries, 0x0131, bytes, le),
            DateTime = TiffRawHelpers.ReadAsciiTag(entries, 0x0132, bytes, le),
            Artist = TiffRawHelpers.ReadAsciiTag(entries, 0x013B, bytes, le),
            Copyright = TiffRawHelpers.ReadAsciiTag(entries, 0x8298, bytes, le),
            MakerNoteLength = TiffRawHelpers.GetTagByteLength(entries, 0x927C),
        };
    }

    /// <summary>
    /// Project the EXIF block into the cross-format
    /// <see cref="ImageMetadata"/> surface that every <see cref="IImageReader"/>
    /// exposes. The MakerNote byte length is surfaced via the
    /// <c>Exif:MakerNoteLength</c> tag (vendor-agnostic key so the same
    /// downstream consumer code works regardless of camera brand).
    /// </summary>
    public static ImageMetadata BuildImageMetadata(TiffRawMetadata raw)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        if (raw.MakerNoteLength > 0)
        {
            tags["Exif:MakerNoteLength"] = raw.MakerNoteLength.ToString(System.Globalization.CultureInfo.InvariantCulture);
        }

        return new ImageMetadata
        {
            CameraMake = raw.Make,
            CameraModel = raw.Model,
            Software = raw.Software,
            CapturedAtRaw = raw.DateTime,
            Author = raw.Artist,
            Copyright = raw.Copyright,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }
}
