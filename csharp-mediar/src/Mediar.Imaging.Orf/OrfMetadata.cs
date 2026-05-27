namespace Mediar.Imaging.Orf;

/// <summary>
/// Olympus-specific metadata parsed from the ORF root IFD.
/// </summary>
public sealed record OrfMetadata
{
    /// <summary>EXIF Make ("OLYMPUS IMAGING CORP.", "OLYMPUS CORPORATION",
    /// "OLYMPUS OPTICAL CO.,LTD", "OM Digital Solutions" for genuine ORF files).</summary>
    public required string? Make { get; init; }

    /// <summary>EXIF Model (e.g. "OM-1", "E-M1", "PEN-F").</summary>
    public required string? Model { get; init; }

    /// <summary>EXIF Software string (firmware version).</summary>
    public required string? Software { get; init; }

    /// <summary>EXIF DateTime (raw ASCII as stored).</summary>
    public required string? DateTime { get; init; }

    /// <summary>EXIF Artist.</summary>
    public required string? Artist { get; init; }

    /// <summary>EXIF Copyright.</summary>
    public required string? Copyright { get; init; }

    /// <summary>
    /// The Olympus magic word found at file offset 2, as read with the
    /// file's byte order. The raw on-disk bytes are always 'R'/'O' (or
    /// 'R'/'S' for legacy E-System) at offsets 2-3, but their numerical
    /// interpretation depends on endianness:
    /// <list type="bullet">
    ///   <item><c>0x002A</c> - standard TIFF magic (Olympus often uses this).</item>
    ///   <item><c>0x4F52</c> - little-endian "RO" or big-endian "OR" (current Olympus).</item>
    ///   <item><c>0x5352</c> - little-endian "RS" (older E-System).</item>
    /// </list>
    /// </summary>
    public required int OlympusMagic { get; init; }

    /// <summary>Number of bytes occupied by the raw Olympus MakerNote (tag 0x927C), or 0 if absent.</summary>
    public required int MakerNoteLength { get; init; }
}

/// <summary>Public view of a single ORF sub-image (typically IFD 0 plus SubIFDs).</summary>
public sealed record OrfSubImageInfo
{
    /// <summary>Width in pixels.</summary>
    public required int Width { get; init; }

    /// <summary>Height in pixels.</summary>
    public required int Height { get; init; }

    /// <summary>Bits per sample.</summary>
    public required int BitsPerSample { get; init; }

    /// <summary>Samples per pixel.</summary>
    public required int SamplesPerPixel { get; init; }

    /// <summary>
    /// TIFF compression tag. 1 = uncompressed, 7 = JPEG-in-TIFF (embedded preview),
    /// 32773 = PackBits, custom Olympus packed RAW uses standard tag 1 with a
    /// 12-bit-into-16-bit packing scheme that requires the Olympus MakerNote
    /// LUT (tag 0x100) to decode.
    /// </summary>
    public required int CompressionTag { get; init; }

    /// <summary>TIFF photometric interpretation tag.</summary>
    public required int Photometric { get; init; }

    /// <summary>NewSubFileType (tag 0x00FE). 0 = primary, 1 = reduced-res preview.</summary>
    public required int NewSubFileType { get; init; }

    /// <summary>Pixel format Mediar will emit (<see cref="PixelFormat.Unknown"/> if not yet supported).</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>0 for IFD 0, 1 for direct SubIFD children, etc.</summary>
    public required int SubIfdLevel { get; init; }

    /// <summary>True if Mediar can decode this sub-image through the underlying TIFF reader.</summary>
    public required bool CanDecodePixels { get; init; }
}
