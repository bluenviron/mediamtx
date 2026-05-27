namespace Mediar.Imaging.Dcr;

/// <summary>
/// Kodak-specific metadata parsed from the DCR file (mostly EXIF / IFD 0 tags).
/// </summary>
public sealed record DcrMetadata
{
    /// <summary>EXIF Make ("EASTMAN KODAK COMPANY" / "KODAK" / "Kodak" for genuine DCR files).</summary>
    public required string? Make { get; init; }

    /// <summary>EXIF Model (e.g. "DCS Pro 14n").</summary>
    public required string? Model { get; init; }

    /// <summary>EXIF Software string (firmware version).</summary>
    public required string? Software { get; init; }

    /// <summary>EXIF DateTime (raw ASCII as stored).</summary>
    public required string? DateTime { get; init; }

    /// <summary>EXIF Artist.</summary>
    public required string? Artist { get; init; }

    /// <summary>EXIF Copyright.</summary>
    public required string? Copyright { get; init; }

    /// <summary>Number of bytes occupied by the raw Kodak MakerNote (tag 0x927C), or 0 if absent.</summary>
    public required int MakerNoteLength { get; init; }
}

/// <summary>Public view of a single DCR sub-image (typically IFD 0 plus one SubIFD per pyramid level).</summary>
public sealed record DcrSubImageInfo
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
    /// TIFF compression tag. 1 = uncompressed, 7 = JPEG (standard JPEG-in-TIFF),
    /// 65000 = Kodak DCR compressed (proprietary, not yet supported).
    /// </summary>
    public required int CompressionTag { get; init; }

    /// <summary>TIFF photometric interpretation tag.</summary>
    public required int Photometric { get; init; }

    /// <summary>NewSubFileType (tag 0x00FE). 0 = primary, 1 = reduced-res preview, etc.</summary>
    public required int NewSubFileType { get; init; }

    /// <summary>Pixel format Mediar will emit (<see cref="PixelFormat.Unknown"/> if not yet supported).</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>0 for IFD 0, 1 for direct SubIFD children, etc.</summary>
    public required int SubIfdLevel { get; init; }

    /// <summary>True if Mediar can decode this sub-image through the underlying TIFF reader.</summary>
    public required bool CanDecodePixels { get; init; }
}
