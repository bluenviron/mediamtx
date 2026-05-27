namespace Mediar.Imaging.Dng;

/// <summary>
/// DNG-specific tag set parsed from IFD 0 of an Adobe Digital Negative
/// file. Reflects the structure defined by the DNG 1.7 specification.
/// Values default to empty / null when the source file does not include
/// the corresponding tag.
/// </summary>
public sealed record DngMetadata
{
    /// <summary>DNGVersion (tag 0xC612). Four BYTEs: major.minor.0.0, e.g. <c>{1,7,0,0}</c> for DNG 1.7.</summary>
    public required ReadOnlyMemory<byte> DngVersion { get; init; }

    /// <summary>DNGBackwardVersion (tag 0xC613). Minimum DNG version a reader must support to handle this file.</summary>
    public required ReadOnlyMemory<byte> DngBackwardVersion { get; init; }

    /// <summary>UniqueCameraModel (tag 0xC614).</summary>
    public required string? UniqueCameraModel { get; init; }

    /// <summary>LocalizedCameraModel (tag 0xC615).</summary>
    public required string? LocalizedCameraModel { get; init; }

    /// <summary>Make (EXIF tag 0x010F).</summary>
    public required string? Make { get; init; }

    /// <summary>Model (EXIF tag 0x0110).</summary>
    public required string? Model { get; init; }

    /// <summary>Software (EXIF tag 0x0131).</summary>
    public required string? Software { get; init; }

    /// <summary>DateTime (EXIF tag 0x0132); raw ASCII as stored in the file.</summary>
    public required string? DateTime { get; init; }

    /// <summary>Artist (EXIF tag 0x013B).</summary>
    public required string? Artist { get; init; }

    /// <summary>Copyright (EXIF tag 0x8298).</summary>
    public required string? Copyright { get; init; }

    /// <summary>CFAPattern (tag 0x828E). 2×2 (or larger) Bayer mosaic pattern, one byte per cell (0=R, 1=G, 2=B, 3=C, 4=M, 5=Y, 6=W).</summary>
    public required ReadOnlyMemory<byte> CfaPattern { get; init; }

    /// <summary>CFARepeatPatternDim (tag 0x828D). Two SHORTs giving rows × cols of the repeat tile.</summary>
    public required ushort[] CfaRepeatPatternDim { get; init; }

    /// <summary>CFAPlaneColor (tag 0xC616). Per-plane colour codes (same encoding as CFAPattern).</summary>
    public required ReadOnlyMemory<byte> CfaPlaneColor { get; init; }

    /// <summary>CFALayout (tag 0xC617). 1=rectangular, 2..6=offset variants for staggered sensors.</summary>
    public required int CfaLayout { get; init; }

    /// <summary>BlackLevel (tag 0xC61A). Per-CFA-plane sensor floor.</summary>
    public required uint[] BlackLevel { get; init; }

    /// <summary>WhiteLevel (tag 0xC61D). Per-CFA-plane saturation level.</summary>
    public required uint[] WhiteLevel { get; init; }

    /// <summary>DefaultCropOrigin (tag 0xC61F). 2 RATIONALs (x, y) for the recommended default crop.</summary>
    public required double[] DefaultCropOrigin { get; init; }

    /// <summary>DefaultCropSize (tag 0xC620). 2 RATIONALs (width, height) for the recommended default crop.</summary>
    public required double[] DefaultCropSize { get; init; }

    /// <summary>ActiveArea (tag 0xC68D). 4 SHORTs/LONGs (top, left, bottom, right) for the active pixel area.</summary>
    public required double[] ActiveArea { get; init; }

    /// <summary>AsShotNeutral (tag 0xC628). Per-channel white balance multipliers (relative to a neutral reference).</summary>
    public required double[] AsShotNeutral { get; init; }

    /// <summary>AsShotWhiteXY (tag 0xC629). White-balance chromaticity in CIE xy.</summary>
    public required double[] AsShotWhiteXY { get; init; }

    /// <summary>ColorMatrix1 (tag 0xC621). 3×3 SRATIONAL matrix converting CIE XYZ-D50 to camera native, for illuminant 1.</summary>
    public required double[] ColorMatrix1 { get; init; }

    /// <summary>ColorMatrix2 (tag 0xC622). Same as ColorMatrix1 but for illuminant 2.</summary>
    public required double[] ColorMatrix2 { get; init; }
}

/// <summary>
/// Public view of a single sub-image discovered while walking the DNG IFD
/// tree (top-level IFDs + every SubIFD recursively).
/// </summary>
public sealed record DngSubImageInfo
{
    /// <summary>Width in pixels.</summary>
    public required int Width { get; init; }

    /// <summary>Height in pixels.</summary>
    public required int Height { get; init; }

    /// <summary>Bits per sample (typically 8, 12, 14, or 16 for raw sensor data).</summary>
    public required int BitsPerSample { get; init; }

    /// <summary>Samples per pixel (1 for raw Bayer, 3 for RGB previews).</summary>
    public required int SamplesPerPixel { get; init; }

    /// <summary>TIFF compression tag (1 = uncompressed, 7 = JPEG/SOF3 lossless, 8 = Deflate, etc.).</summary>
    public required int CompressionTag { get; init; }

    /// <summary>TIFF photometric interpretation tag.</summary>
    public required int Photometric { get; init; }

    /// <summary>
    /// NewSubFileType (tag 0x00FE). 0 = primary image, 1 = reduced-resolution copy,
    /// 2 = single page of a multi-page image, etc. Used to pick the
    /// full-resolution raw among multiple sub-images.
    /// </summary>
    public required int NewSubFileType { get; init; }

    /// <summary>True if the sub-image uses tiled layout.</summary>
    public required bool IsTiled { get; init; }

    /// <summary>Decoded pixel format Mediar will emit (or <see cref="PixelFormat.Unknown"/>).</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>0 for top-level IFDs, 1 for direct SubIFD children, 2 for SubIFD-of-SubIFD, etc.</summary>
    public required int SubIfdLevel { get; init; }

    /// <summary>True if Mediar can decode this sub-image's pixel data through the underlying TIFF reader.</summary>
    public required bool CanDecodePixels { get; init; }
}
