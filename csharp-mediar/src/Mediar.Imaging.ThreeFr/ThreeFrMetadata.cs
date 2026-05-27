namespace Mediar.Imaging.ThreeFr;

/// <summary>
/// Hasselblad-specific metadata parsed from the 3FR file (mostly EXIF / IFD 0 tags).
/// </summary>
public sealed record ThreeFrMetadata
{
    /// <summary>EXIF Make ("Hasselblad" or "HASSELBLAD" for genuine 3FR files).</summary>
    public required string? Make { get; init; }

    /// <summary>EXIF Model (e.g. "H6D-100c", "X1D II 50C", "503CWD").</summary>
    public required string? Model { get; init; }

    /// <summary>EXIF Software string (firmware version).</summary>
    public required string? Software { get; init; }

    /// <summary>EXIF DateTime (raw ASCII as stored).</summary>
    public required string? DateTime { get; init; }

    /// <summary>EXIF Artist.</summary>
    public required string? Artist { get; init; }

    /// <summary>EXIF Copyright.</summary>
    public required string? Copyright { get; init; }

    /// <summary>Number of bytes occupied by the raw Hasselblad MakerNote (tag 0x927C), or 0 if absent.</summary>
    public required int MakerNoteLength { get; init; }
}

/// <summary>Public view of a single 3FR sub-image (typically IFD 0 plus one SubIFD per pyramid level).</summary>
public sealed record ThreeFrSubImageInfo
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
    /// 8 (with bayer-mosaic photometric) = Hasselblad-compressed RAW (not yet supported).
    /// </summary>
    public required int CompressionTag { get; init; }

    /// <summary>TIFF photometric interpretation tag. 32803 (CFA) is bayer mosaic.</summary>
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
