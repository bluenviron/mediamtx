namespace Mediar.Imaging.Rw2;

/// <summary>
/// Panasonic / Leica RW2-specific metadata parsed from the RAW IFD.
/// </summary>
public sealed record Rw2Metadata
{
    /// <summary>EXIF Make ("Panasonic" or "LEICA CAMERA AG" for genuine RW2 files).</summary>
    public required string? Make { get; init; }

    /// <summary>EXIF Model (e.g. "DC-S5M2" / "DMC-GH5").</summary>
    public required string? Model { get; init; }

    /// <summary>EXIF Software string (firmware version).</summary>
    public required string? Software { get; init; }

    /// <summary>EXIF DateTime (raw ASCII as stored).</summary>
    public required string? DateTime { get; init; }

    /// <summary>EXIF Artist.</summary>
    public required string? Artist { get; init; }

    /// <summary>EXIF Copyright.</summary>
    public required string? Copyright { get; init; }

    /// <summary>Tag 0x0001 - PanasonicRawVersion (ASCII, typically "0\0").</summary>
    public required string? PanasonicRawVersion { get; init; }

    /// <summary>Tag 0x0002 - SensorWidth.</summary>
    public required int SensorWidth { get; init; }

    /// <summary>Tag 0x0003 - SensorHeight.</summary>
    public required int SensorHeight { get; init; }

    /// <summary>Tag 0x0004 - SensorTopBorder.</summary>
    public required int SensorTopBorder { get; init; }

    /// <summary>Tag 0x0005 - SensorLeftBorder.</summary>
    public required int SensorLeftBorder { get; init; }

    /// <summary>Tag 0x0006 - SensorBottomBorder.</summary>
    public required int SensorBottomBorder { get; init; }

    /// <summary>Tag 0x0007 - SensorRightBorder.</summary>
    public required int SensorRightBorder { get; init; }

    /// <summary>Tag 0x0009 - CFAPattern (1 = [R,G,G,B], 2 = [G,B,R,G], 3 = [G,R,B,G], 4 = [B,G,G,R]).</summary>
    public required int CfaPattern { get; init; }

    /// <summary>Tag 0x000F - CropTop (in pixels relative to sensor).</summary>
    public required int CropTop { get; init; }

    /// <summary>Tag 0x0010 - CropLeft.</summary>
    public required int CropLeft { get; init; }

    /// <summary>Tag 0x0011 - CropBottom.</summary>
    public required int CropBottom { get; init; }

    /// <summary>Tag 0x0012 - CropRight.</summary>
    public required int CropRight { get; init; }

    /// <summary>Tag 0x0017 - ISO speed.</summary>
    public required int Iso { get; init; }
}

/// <summary>Public view of a single RW2 sub-image (IFD 0 plus SubIFDs).</summary>
public sealed record Rw2SubImageInfo
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
    /// 34316 = Panasonic proprietary RAW (not yet supported).
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
