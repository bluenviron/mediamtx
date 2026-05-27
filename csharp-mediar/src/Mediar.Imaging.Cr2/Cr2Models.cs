namespace Mediar.Imaging.Cr2;

/// <summary>
/// Canon Raw v2 (CR2) file header. Sits at bytes 8-15 of the file,
/// immediately after the TIFF byte-order-mark + magic + IFD0 offset.
/// </summary>
public sealed record Cr2Header
{
    /// <summary>CR2 major version (always 2 in practice; CR3 is a different ISO-BMFF-based format).</summary>
    public required int MajorVersion { get; init; }

    /// <summary>CR2 minor version (typically 0).</summary>
    public required int MinorVersion { get; init; }

    /// <summary>Absolute byte offset of the raw IFD (typically IFD 3, holding the lossless-JPEG raw sensor mosaic).</summary>
    public required uint RawIfdOffset { get; init; }
}

/// <summary>
/// Public view of a single Canon-Raw IFD discovered during the IFD-chain walk.
/// </summary>
public sealed record Cr2SubImageInfo
{
    /// <summary>
    /// Logical role of this IFD within the CR2 layout. <see cref="Cr2IfdRole.Unknown"/>
    /// for files that don't follow the canonical four-IFD pattern.
    /// </summary>
    public required Cr2IfdRole Role { get; init; }

    /// <summary>Width in pixels.</summary>
    public required int Width { get; init; }

    /// <summary>Height in pixels.</summary>
    public required int Height { get; init; }

    /// <summary>Bits per sample (typically 8 for thumbnails/previews, 14-16 for raw).</summary>
    public required int BitsPerSample { get; init; }

    /// <summary>Samples per pixel (3 for RGB previews, 1 for raw mosaic).</summary>
    public required int SamplesPerPixel { get; init; }

    /// <summary>TIFF compression tag (1 = uncompressed, 6 = old JPEG, 7 = JPEG/lossless SOF3).</summary>
    public required int CompressionTag { get; init; }

    /// <summary>TIFF photometric interpretation tag.</summary>
    public required int Photometric { get; init; }

    /// <summary>Pixel format Mediar will emit (<see cref="PixelFormat.Unknown"/> if not yet supported).</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>True if Mediar can decode this IFD's pixel data through the underlying TIFF reader.</summary>
    public required bool CanDecodePixels { get; init; }
}

/// <summary>Canonical CR2 IFD role mapping per the Canon-internal layout convention.</summary>
public enum Cr2IfdRole
{
    /// <summary>Catch-all for files that don't follow the four-IFD convention.</summary>
    Unknown = 0,

    /// <summary>IFD 0 - small RGB thumbnail (typically 160x120, uncompressed RGB or JPEG).</summary>
    Thumbnail = 1,

    /// <summary>IFD 1 - alternate or smaller thumbnail.</summary>
    AlternateThumbnail = 2,

    /// <summary>IFD 2 - full-size uncompressed RGB preview.</summary>
    FullPreview = 3,

    /// <summary>IFD 3 - raw sensor data, typically lossless JPEG SOF3.</summary>
    RawSensor = 4,
}
