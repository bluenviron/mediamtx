using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Jpeg;

namespace Mediar.Imaging.Crw;

/// <summary>
/// Reader for Canon CIFF v1 RAW (CRW) files. CRW is the heap-based
/// Camera Image File Format that Canon used for the EOS-D30, D60, 10D,
/// 300D (Digital Rebel), 1D, 1Ds, and various PowerShot Pro1/G1-G6
/// bodies before the TIFF-based CR2 superseded it. The reader composes
/// <see cref="JpegReader"/> for the embedded JPEG thumbnail (always
/// decodable) and surfaces the raw CCD/CMOS data as an undecodable
/// sub-image — full raw decode requires the Canon-specific lossless
/// JPEG predictor table that is a separate codec engine.
/// </summary>
/// <remarks>
/// <para>
/// On-disk layout per the public Canon CIFF v1.0R specification:
/// </para>
/// <list type="table">
///   <listheader><term>Offset</term><description>Field</description></listheader>
///   <item><term>0x00</term><description>2-byte byte-order mark ("II" or "MM")</description></item>
///   <item><term>0x02</term><description>u32 header length (typically 26)</description></item>
///   <item><term>0x06</term><description>8-byte ASCII type+subtype ("HEAPCCDR")</description></item>
///   <item><term>0x0E</term><description>u32 CIFF version (e.g. 0x00010002 = v1.2)</description></item>
///   <item><term>0x12</term><description>8 reserved bytes</description></item>
///   <item><term>0x1A...EOF-4</term><description>Heap body (variable)</description></item>
///   <item><term>EOF-4</term><description>u32 heap-directory offset (relative to start of heap, i.e. byte after the header)</description></item>
/// </list>
/// <para>
/// At <c>HeapBase + DirectoryOffset</c> the directory is encoded as:
/// </para>
/// <list type="bullet">
///   <item><description>u16 entry count</description></item>
///   <item><description>N × 10-byte entries: u16 tag + u32 size + u32 payload-offset (offsets relative to heap base)</description></item>
///   <item><description>4 trailing bytes (heap-end marker)</description></item>
/// </list>
/// <para>
/// Tags are categorised by their high nibble:
/// 0x0xxx = byte data, 0x1xxx = ASCII/WORD data, 0x2xxx = DWORD data,
/// 0x3xxx = sub-heap (recursive directory). The reader walks every
/// sub-heap recursively with a depth-bounded cycle guard.
/// </para>
/// </remarks>
public sealed class CrwReader : IImageReader
{
    private const int HeaderSize = 26;
    private const int MaxRecursionDepth = 8;

    private static readonly byte[] s_typeHeapCcdr = "HEAPCCDR"u8.ToArray();

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly bool _littleEndian;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Crw;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>Canon-specific metadata parsed from the CIFF heap.</summary>
    public CrwMetadata Crw { get; }

    /// <summary>All entries discovered while walking the CIFF heap recursively.</summary>
    public IReadOnlyList<CrwSubImageInfo> SubImages { get; }

    private CrwReader(Stream s, bool ownsStream, byte[] bytes, bool littleEndian,
                     ImageInfo info, ImageMetadata meta, CrwMetadata crw,
                     IReadOnlyList<CrwSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream; _bytes = bytes;
        _littleEndian = littleEndian;
        Info = info; Metadata = meta; Crw = crw;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open a CRW file by path.</summary>
    public static CrwReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a CRW from a stream (the contents are buffered into memory).</summary>
    public static CrwReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < HeaderSize + 4)
        {
            throw new ImageFormatException("Truncated CRW (header + directory-offset trailer < 30 bytes).");
        }

        bool littleEndian = bytes[0] == (byte)'I' && bytes[1] == (byte)'I';
        bool bigEndian = bytes[0] == (byte)'M' && bytes[1] == (byte)'M';
        if (!littleEndian && !bigEndian)
        {
            throw new ImageFormatException("Not a CRW file (invalid byte-order mark; expected 'II' or 'MM').");
        }

        for (int i = 0; i < s_typeHeapCcdr.Length; i++)
        {
            if (bytes[6 + i] != s_typeHeapCcdr[i])
            {
                throw new ImageFormatException("Not a CRW file (missing 'HEAPCCDR' signature at offset 6).");
            }
        }

        uint headerLength = ReadU32(bytes, 2, littleEndian);
        if (headerLength < HeaderSize || headerLength > bytes.Length)
        {
            throw new ImageFormatException($"Invalid CRW header length {headerLength} (file size {bytes.Length}).");
        }

        uint version = ReadU32(bytes, 14, littleEndian);

        uint heapBase = headerLength;
        if (bytes.Length < heapBase + 4)
        {
            throw new ImageFormatException("CRW heap body missing or truncated.");
        }

        // Heap-directory offset lives in the LAST 4 bytes of the file and is
        // relative to the heap base (i.e. the first byte after the header).
        uint directoryRelOffset = ReadU32(bytes, bytes.Length - 4, littleEndian);
        long directoryAbsOffset = heapBase + (long)directoryRelOffset;
        if (directoryAbsOffset < heapBase || directoryAbsOffset + 2 > bytes.Length)
        {
            throw new ImageFormatException($"CRW heap-directory offset 0x{directoryAbsOffset:X} is out of bounds (file size {bytes.Length}).");
        }

        var subImages = new List<CrwSubImageInfo>();
        var parsed = new ParsedTags();

        int topLevelCount = WalkDirectory(
            bytes, heapBase, (uint)directoryAbsOffset, littleEndian,
            depth: 0, subImages, parsed);

        // Pick primary frame for ImageInfo: largest decodable JPEG thumbnail.
        CrwSubImageInfo? primary = null;
        foreach (var s in subImages)
        {
            if (s.CanDecodePixels && (primary is null || (long)s.Width * s.Height > (long)primary.Width * primary.Height))
            {
                primary = s;
            }
        }

        bool canDecode = primary is not null;
        int infoW = primary?.Width ?? (int)(parsed.SensorWidth ?? 0);
        int infoH = primary?.Height ?? (int)(parsed.SensorHeight ?? 0);
        var infoPf = canDecode ? PixelFormat.Rgb24 : PixelFormat.Unknown;
        int infoBpp = canDecode ? 24 : (int)(parsed.ComponentBitDepth ?? 0);

        var crw = new CrwMetadata
        {
            ByteOrderMark = littleEndian ? "II" : "MM",
            HeaderLength = headerLength,
            Type = "HEAPCCDR",
            Version = version,
            Make = parsed.Make,
            Model = parsed.Model,
            FirmwareVersion = parsed.FirmwareVersion,
            OwnerName = parsed.OwnerName,
            CaptureTimeSeconds = parsed.CaptureTimeSeconds,
            SensorWidth = parsed.SensorWidth,
            SensorHeight = parsed.SensorHeight,
            PixelAspectNumerator = parsed.PixelAspectNum,
            PixelAspectDenominator = parsed.PixelAspectDen,
            ComponentBitDepth = parsed.ComponentBitDepth,
            TopLevelEntryCount = topLevelCount,
            TotalEntryCount = subImages.Count,
        };

        var info = new ImageInfo
        {
            Width = infoW,
            Height = infoH,
            BitsPerPixel = infoBpp,
            ChannelCount = canDecode ? 3 : 1,
            PixelFormat = infoPf,
            Format = ImageFormat.Crw,
            HasAlpha = false,
            FrameCount = canDecode ? 1 : 0,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(crw);
        return new CrwReader(stream, ownsStream, bytes, littleEndian,
                             info, meta, crw, subImages, canDecode);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "CRW pixel decode requires a decodable embedded JPEG thumbnail (CIFF tag 0x2007). " +
                "None was found in this file. Raw CCD/CMOS data (tag 0x2005) decode requires the " +
                "Canon-specific lossless JPEG predictor table, which is a separate codec engine.");
        }

        // Yield from the largest decodable JPEG thumbnail.
        CrwSubImageInfo? primary = null;
        foreach (var s in SubImages)
        {
            if (s.CanDecodePixels && s.Kind == CrwSubImageKind.JpegThumbnail &&
                (primary is null || (long)s.Width * s.Height > (long)primary.Width * primary.Height))
            {
                primary = s;
            }
        }
        if (primary is null)
        {
            throw new InvalidOperationException("CrwReader.CanDecodePixels = true but no decodable JPEG sub-image was found.");
        }

        using var ms = new MemoryStream(_bytes, (int)primary.Offset, (int)primary.Length, writable: false);
        using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
        bool yielded = false;
        await foreach (var frame in jpeg.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
        {
            yielded = true;
            yield return frame;
            break;
        }
        if (!yielded)
        {
            throw new InvalidOperationException("Embedded CRW JPEG thumbnail produced no frames.");
        }
    }

    /// <inheritdoc/>
    public ValueTask DisposeAsync() { Dispose(); return ValueTask.CompletedTask; }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
        GC.SuppressFinalize(this);
    }

    private sealed class ParsedTags
    {
        public string? Make;
        public string? Model;
        public string? FirmwareVersion;
        public string? OwnerName;
        public uint? CaptureTimeSeconds;
        public uint? SensorWidth;
        public uint? SensorHeight;
        public uint? PixelAspectNum;
        public uint? PixelAspectDen;
        public uint? ComponentBitDepth;
    }

    private static int WalkDirectory(
        byte[] bytes, uint heapBase, uint dirAbsOffset, bool le,
        int depth, List<CrwSubImageInfo> sink, ParsedTags parsed)
    {
        if (depth > MaxRecursionDepth)
        {
            throw new ImageFormatException("CRW heap directory exceeds maximum recursion depth (possible cycle).");
        }
        if (dirAbsOffset + 2 > bytes.Length)
        {
            throw new ImageFormatException($"CRW heap directory at 0x{dirAbsOffset:X} is out of bounds.");
        }

        ushort entryCount = ReadU16(bytes, (int)dirAbsOffset, le);
        long entriesStart = dirAbsOffset + 2;
        if (entriesStart + (long)entryCount * 10 > bytes.Length)
        {
            throw new ImageFormatException($"CRW heap directory has {entryCount} entries but the file is truncated.");
        }

        for (int i = 0; i < entryCount; i++)
        {
            int entryOff = (int)entriesStart + i * 10;
            ushort tag = ReadU16(bytes, entryOff, le);
            uint size = ReadU32(bytes, entryOff + 2, le);
            uint payloadRel = ReadU32(bytes, entryOff + 6, le);

            // Tags with the 0x4000 bit set store the payload inline in the
            // entry (rare; called "InRecordEntry" in the spec). We treat the
            // payload offset as bytes-into-the-entry-record for those, but
            // none of the common Canon tags use it so we fall through to
            // the conventional out-of-line layout for now.
            long payloadAbs;
            bool inRecord = (tag & 0x4000) != 0;
            if (inRecord)
            {
                payloadAbs = entryOff + 2;
                size = Math.Min(size, 8u);
            }
            else
            {
                payloadAbs = heapBase + (long)payloadRel;
            }

            if (payloadAbs < 0 || payloadAbs + size > bytes.Length)
            {
                // Out-of-bounds entries are recorded as undecodable rather
                // than throwing; some legacy Canon firmware emits zero-size
                // placeholder entries.
                size = 0;
                payloadAbs = entryOff;
            }

            ushort baseTag = (ushort)(tag & 0x3FFF);
            byte category = (byte)((baseTag >> 12) & 0x0F);

            var kind = ClassifyTag(baseTag, category);

            int w = 0, h = 0;
            bool canDecode = false;
            if (kind == CrwSubImageKind.JpegThumbnail && size > 0)
            {
                (w, h, canDecode) = ProbeJpegDimensions(bytes, (int)payloadAbs, (int)size);
            }

            sink.Add(new CrwSubImageInfo
            {
                Kind = kind,
                Tag = tag,
                Length = size,
                Offset = (uint)payloadAbs,
                Width = w,
                Height = h,
                CanDecodePixels = canDecode,
                DirectoryDepth = depth,
            });

            // Extract typed metadata from well-known tags.
            ExtractTag(bytes, (int)payloadAbs, (int)size, baseTag, le, parsed);

            // Descend into sub-heaps (category 0x3, which terminate in a directory of their own).
            if (kind == CrwSubImageKind.SubHeap && size >= 6)
            {
                // The sub-heap is itself a CIFF heap: its trailing u32 (last 4
                // bytes of the sub-heap payload) points at the sub-directory,
                // relative to the start of the sub-heap payload.
                uint subDirRel = ReadU32(bytes, (int)(payloadAbs + size - 4), le);
                long subDirAbs = payloadAbs + (long)subDirRel;
                if (subDirAbs >= payloadAbs && subDirAbs + 2 <= payloadAbs + size)
                {
                    WalkDirectory(bytes, (uint)payloadAbs, (uint)subDirAbs, le,
                                  depth + 1, sink, parsed);
                }
            }
        }

        return entryCount;
    }

    private static CrwSubImageKind ClassifyTag(ushort baseTag, byte category)
    {
        // High nibble drives the category. Specific tags override the default.
        switch (baseTag)
        {
            case 0x2005: return CrwSubImageKind.RawImageData;
            case 0x2007: return CrwSubImageKind.JpegThumbnail;
        }
        return category switch
        {
            0x0 => CrwSubImageKind.ByteArray,
            0x1 => CrwSubImageKind.NumericArray,
            0x2 => CrwSubImageKind.NumericArray,
            0x3 => CrwSubImageKind.SubHeap,
            _ => CrwSubImageKind.Unknown,
        };
    }

    private static void ExtractTag(byte[] bytes, int offset, int size,
                                   ushort baseTag, bool le, ParsedTags parsed)
    {
        if (size <= 0) return;
        if (offset < 0 || offset + size > bytes.Length) return;
        var slice = bytes.AsSpan(offset, size);

        switch (baseTag)
        {
            case 0x080A:
            {
                // Camera type string: ASCII "Make\0Model\0..."
                int nul = slice.IndexOf((byte)0);
                if (nul > 0)
                {
                    parsed.Make ??= Encoding.ASCII.GetString(slice[..nul]);
                    var rest = slice[(nul + 1)..];
                    int nul2 = rest.IndexOf((byte)0);
                    if (nul2 < 0) nul2 = rest.Length;
                    if (nul2 > 0) parsed.Model ??= Encoding.ASCII.GetString(rest[..nul2]);
                }
                else if (slice.Length > 0)
                {
                    parsed.Make ??= Encoding.ASCII.GetString(slice).TrimEnd('\0');
                }
                break;
            }
            case 0x080B:
            {
                int nul = slice.IndexOf((byte)0);
                if (nul < 0) nul = slice.Length;
                if (nul > 0) parsed.FirmwareVersion ??= Encoding.ASCII.GetString(slice[..nul]);
                break;
            }
            case 0x0810:
            {
                int nul = slice.IndexOf((byte)0);
                if (nul < 0) nul = slice.Length;
                if (nul > 0) parsed.OwnerName ??= Encoding.ASCII.GetString(slice[..nul]);
                break;
            }
            case 0x180E:
            {
                if (size >= 4)
                {
                    parsed.CaptureTimeSeconds ??= ReadU32(bytes, offset, le);
                }
                break;
            }
            case 0x1810:
            {
                if (size >= 28)
                {
                    parsed.SensorWidth ??= ReadU32(bytes, offset + 0, le);
                    parsed.SensorHeight ??= ReadU32(bytes, offset + 4, le);
                    parsed.PixelAspectNum ??= ReadU32(bytes, offset + 8, le);
                    parsed.PixelAspectDen ??= ReadU32(bytes, offset + 12, le);
                    parsed.ComponentBitDepth ??= ReadU32(bytes, offset + 20, le);
                }
                break;
            }
        }
    }

    private static (int Width, int Height, bool CanDecode) ProbeJpegDimensions(byte[] bytes, int offset, int length)
    {
        if (length < 4) return (0, 0, false);
        if (bytes[offset] != 0xFF || bytes[offset + 1] != 0xD8) return (0, 0, false);
        try
        {
            using var ms = new MemoryStream(bytes, offset, length, writable: false);
            using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
            return (jpeg.Info.Width, jpeg.Info.Height, jpeg.CanDecodePixels);
        }
        catch (Exception ex) when (ex is ImageFormatException or InvalidOperationException or NotSupportedException)
        {
            return (0, 0, false);
        }
    }

    private static ImageMetadata BuildImageMetadata(CrwMetadata crw)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["CIFF:ByteOrder"] = crw.ByteOrderMark,
            ["CIFF:HeaderLength"] = crw.HeaderLength.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["CIFF:Type"] = crw.Type,
            ["CIFF:Version"] = $"0x{crw.Version:X8}",
            ["CIFF:TopLevelEntryCount"] = crw.TopLevelEntryCount.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["CIFF:TotalEntryCount"] = crw.TotalEntryCount.ToString(System.Globalization.CultureInfo.InvariantCulture),
        };
        if (crw.Make is not null) tags["EXIF:Make"] = crw.Make;
        if (crw.Model is not null) tags["EXIF:Model"] = crw.Model;
        if (crw.FirmwareVersion is not null) tags["EXIF:Software"] = crw.FirmwareVersion;
        if (crw.OwnerName is not null) tags["EXIF:Artist"] = crw.OwnerName;
        if (crw.CaptureTimeSeconds is uint cts)
        {
            var dto = DateTimeOffset.FromUnixTimeSeconds(cts);
            tags["EXIF:DateTime"] = dto.UtcDateTime.ToString("yyyy:MM:dd HH:mm:ss", System.Globalization.CultureInfo.InvariantCulture);
        }
        if (crw.SensorWidth is uint sw) tags["CIFF:SensorWidth"] = sw.ToString(System.Globalization.CultureInfo.InvariantCulture);
        if (crw.SensorHeight is uint sh) tags["CIFF:SensorHeight"] = sh.ToString(System.Globalization.CultureInfo.InvariantCulture);
        if (crw.ComponentBitDepth is uint cbd) tags["CIFF:BitsPerSample"] = cbd.ToString(System.Globalization.CultureInfo.InvariantCulture);

        var meta = new ImageMetadata
        {
            CameraMake = crw.Make,
            CameraModel = crw.Model,
            Software = crw.FirmwareVersion,
            Author = crw.OwnerName,
            CapturedAtRaw = tags.TryGetValue("EXIF:DateTime", out var dt) ? dt : null,
            CapturedAt = crw.CaptureTimeSeconds is uint cts2
                ? DateTimeOffset.FromUnixTimeSeconds(cts2)
                : null,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
        return meta;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16(byte[] bytes, int offset, bool le)
    {
        var s = bytes.AsSpan(offset, 2);
        return le ? BinaryPrimitives.ReadUInt16LittleEndian(s)
                  : BinaryPrimitives.ReadUInt16BigEndian(s);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] bytes, int offset, bool le)
    {
        var s = bytes.AsSpan(offset, 4);
        return le ? BinaryPrimitives.ReadUInt32LittleEndian(s)
                  : BinaryPrimitives.ReadUInt32BigEndian(s);
    }
}
