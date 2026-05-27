namespace Mediar.Imaging.Raf;

/// <summary>
/// Fujifilm-specific metadata parsed from the RAF container header.
/// </summary>
public sealed record RafMetadata
{
    /// <summary>Format version string (4 ASCII bytes at offset 0x10), e.g. "0201".</summary>
    public required string FormatVersion { get; init; }

    /// <summary>
    /// Camera model string parsed from the 32-byte zero-terminated ASCII slot
    /// at offset 0x1C of the RAF header (e.g. "X-T4" / "FinePix S2Pro").
    /// </summary>
    public required string CameraModel { get; init; }

    /// <summary>Directory format version (4 ASCII bytes at offset 0x3C), e.g. "0100" or "0159".</summary>
    public required string DirectoryVersion { get; init; }

    /// <summary>Byte offset of the embedded EXIF/JFIF JPEG preview from the start of the RAF file.</summary>
    public required uint JpegOffset { get; init; }

    /// <summary>Length in bytes of the embedded JPEG preview.</summary>
    public required uint JpegLength { get; init; }

    /// <summary>Byte offset of the Fujifilm "Meta container" tag block.</summary>
    public required uint MetaOffset { get; init; }

    /// <summary>Length in bytes of the Meta container.</summary>
    public required uint MetaLength { get; init; }

    /// <summary>Byte offset of the CFA (Color Filter Array) TIFF block containing raw sensor data.</summary>
    public required uint CfaOffset { get; init; }

    /// <summary>Length in bytes of the CFA TIFF block.</summary>
    public required uint CfaLength { get; init; }
}

/// <summary>
/// Public view of one logical sub-image inside a RAF file. RAF exposes
/// two: the embedded JPEG preview (always present, always decodable) and
/// the CFA raw sensor block (a TIFF container whose pixel payload is
/// Fujifilm-proprietary on modern bodies).
/// </summary>
public sealed record RafSubImageInfo
{
    /// <summary>Logical role of this sub-image.</summary>
    public required RafSubImageKind Kind { get; init; }

    /// <summary>Width in pixels (parsed from the embedded JPEG / from the CFA TIFF IFD).</summary>
    public required int Width { get; init; }

    /// <summary>Height in pixels.</summary>
    public required int Height { get; init; }

    /// <summary>Byte offset of this sub-image's payload from the start of the RAF file.</summary>
    public required uint Offset { get; init; }

    /// <summary>Length in bytes of this sub-image's payload.</summary>
    public required uint Length { get; init; }

    /// <summary>Pixel format Mediar will emit when decoding this sub-image, or <see cref="PixelFormat.Unknown"/>.</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>True if Mediar can decode this sub-image with the bundled codecs.</summary>
    public required bool CanDecodePixels { get; init; }
}

/// <summary>The two logical sub-image roles RAF defines.</summary>
public enum RafSubImageKind
{
    /// <summary>The full EXIF/JFIF JPEG preview embedded in the RAF.</summary>
    JpegPreview = 0,
    /// <summary>The CFA TIFF block holding the raw sensor data.</summary>
    Cfa = 1,
}
