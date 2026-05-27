namespace Mediar.Imaging.Crw;

/// <summary>
/// Strongly-typed Canon CIFF metadata extracted from a CRW file. Captured
/// from the heap directory walk performed by <see cref="CrwReader.Open(System.IO.Stream,bool)"/>.
/// </summary>
public sealed record CrwMetadata
{
    /// <summary>The 4-byte ASCII byte-order mark at offset 0 ("II" or "MM").</summary>
    public required string ByteOrderMark { get; init; }

    /// <summary>CIFF header length in bytes (typically 26).</summary>
    public required uint HeaderLength { get; init; }

    /// <summary>The 8-byte ASCII type+subtype at offset 6 (typically "HEAPCCDR").</summary>
    public required string Type { get; init; }

    /// <summary>CIFF version as packed u32 (high u16 = major, low u16 = minor; e.g. 0x00010002 = 1.2).</summary>
    public required uint Version { get; init; }

    /// <summary>Camera-make string from tag 0x080A (e.g. "Canon").</summary>
    public string? Make { get; init; }

    /// <summary>Camera-model string from tag 0x080A (e.g. "Canon EOS-D30").</summary>
    public string? Model { get; init; }

    /// <summary>Firmware-version string from tag 0x080B.</summary>
    public string? FirmwareVersion { get; init; }

    /// <summary>Owner-name string from tag 0x0810.</summary>
    public string? OwnerName { get; init; }

    /// <summary>Capture-time epoch from tag 0x180E (seconds since 1970-01-01 UTC).</summary>
    public uint? CaptureTimeSeconds { get; init; }

    /// <summary>Sensor width in pixels from the ImageSpec entry (tag 0x1810 dword[0]).</summary>
    public uint? SensorWidth { get; init; }

    /// <summary>Sensor height in pixels from the ImageSpec entry (tag 0x1810 dword[1]).</summary>
    public uint? SensorHeight { get; init; }

    /// <summary>Pixel-aspect-ratio numerator from the ImageSpec entry (tag 0x1810 dword[2]).</summary>
    public uint? PixelAspectNumerator { get; init; }

    /// <summary>Pixel-aspect-ratio denominator from the ImageSpec entry (tag 0x1810 dword[3]).</summary>
    public uint? PixelAspectDenominator { get; init; }

    /// <summary>Component bit depth from the ImageSpec entry (tag 0x1810 dword[5]).</summary>
    public uint? ComponentBitDepth { get; init; }

    /// <summary>Number of entries discovered at the top-level heap directory.</summary>
    public required int TopLevelEntryCount { get; init; }

    /// <summary>Total number of entries discovered across all heap directories (recursive).</summary>
    public required int TotalEntryCount { get; init; }
}

/// <summary>
/// One CIFF heap entry surfaced by <see cref="CrwReader"/>. The CIFF format
/// is hierarchical and entries may live inside sub-heaps; each entry's
/// path is captured by <see cref="DirectoryDepth"/>.
/// </summary>
public sealed record CrwSubImageInfo
{
    /// <summary>Classification of the entry: image payload, thumbnail, sub-heap, metadata.</summary>
    public required CrwSubImageKind Kind { get; init; }

    /// <summary>The 16-bit CIFF tag number (e.g. 0x2005 = raw image, 0x2007 = embedded JPEG thumbnail).</summary>
    public required ushort Tag { get; init; }

    /// <summary>Byte length of the entry payload.</summary>
    public required uint Length { get; init; }

    /// <summary>Absolute file offset of the entry payload (after resolving the heap base).</summary>
    public required uint Offset { get; init; }

    /// <summary>Width in pixels (where the entry exposes one, e.g. decoded JPEG thumbnail).</summary>
    public required int Width { get; init; }

    /// <summary>Height in pixels (where the entry exposes one).</summary>
    public required int Height { get; init; }

    /// <summary>True when Mediar can decode pixels for this entry.</summary>
    public required bool CanDecodePixels { get; init; }

    /// <summary>Depth in the heap hierarchy (0 = top-level, 1+ = nested sub-heap).</summary>
    public required int DirectoryDepth { get; init; }
}

/// <summary>
/// Classification of a CIFF heap entry. CIFF distinguishes byte / ASCII /
/// word / dword / structured / sub-heap data types by the tag's high
/// nibble, but the reader exposes them through a smaller semantic enum.
/// </summary>
public enum CrwSubImageKind
{
    /// <summary>Unrecognised tag; payload bytes are surfaced raw.</summary>
    Unknown = 0,

    /// <summary>Embedded JPEG thumbnail / preview (tag 0x2007).</summary>
    JpegThumbnail,

    /// <summary>Raw CCD/CMOS sensor data (tag 0x2005). Decodable through
    /// the lossless-JPEG codec once the Canon predictor table is parsed
    /// — not yet supported.</summary>
    RawImageData,

    /// <summary>Sub-heap directory (high nibble 0x3xxx). The reader
    /// descends into the sub-heap automatically and surfaces every
    /// child entry individually.</summary>
    SubHeap,

    /// <summary>Camera-info / EXIF metadata structure.</summary>
    MetadataStructure,

    /// <summary>ASCII text payload (camera type, firmware string, owner name, ...).</summary>
    AsciiText,

    /// <summary>Numeric WORD or DWORD array payload (image spec, ISO, white balance, ...).</summary>
    NumericArray,

    /// <summary>Byte payload (custom function descriptors, AF info, etc.).</summary>
    ByteArray,
}
