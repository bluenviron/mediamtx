namespace Mediar.Imaging.Mrw;

/// <summary>
/// Konica Minolta-specific metadata parsed from an MRW envelope. The values
/// come from the <c>\0PRD</c> (picture-raw-dimensions) sub-block and the
/// embedded <c>\0TTW</c> (TIFF tag wrapper) sub-block.
/// </summary>
public sealed record MrwMetadata
{
    /// <summary>8-byte ASCII version string from PRD bytes 0-7 (e.g. "27WB0002").</summary>
    public required string VersionNumber { get; init; }

    /// <summary>Sensor height in pixels (PRD bytes 8-9, big-endian).</summary>
    public required int SensorHeight { get; init; }

    /// <summary>Sensor width in pixels (PRD bytes 10-11, big-endian).</summary>
    public required int SensorWidth { get; init; }

    /// <summary>Image height in pixels (PRD bytes 12-13, big-endian). May differ from sensor for crop modes.</summary>
    public required int ImageHeight { get; init; }

    /// <summary>Image width in pixels (PRD bytes 14-15, big-endian).</summary>
    public required int ImageWidth { get; init; }

    /// <summary>Bits per pixel in storage (PRD byte 16). Typically 12 or 16.</summary>
    public required int DataSize { get; init; }

    /// <summary>Bits per pixel as captured by the sensor (PRD byte 17). Typically 12.</summary>
    public required int PixelSize { get; init; }

    /// <summary>
    /// Storage method (PRD byte 18). 0x52 = packed (12-bit values packed into bytes),
    /// 0x59 = unpacked (each 12-bit value stored as a 16-bit word).
    /// </summary>
    public required int StorageMethod { get; init; }

    /// <summary>Bayer mosaic pattern code (PRD byte 23 in newer firmware).</summary>
    public required int BayerPattern { get; init; }

    /// <summary>EXIF Make (from the embedded TTW TIFF IFD 0) - "MINOLTA CO.,LTD." / "KONICA MINOLTA CAMERA, INC." / "KONICA MINOLTA" for genuine MRW files.</summary>
    public required string? Make { get; init; }

    /// <summary>EXIF Model (e.g. "DiMAGE A2" / "DYNAX 7D" / "MAXXUM 5D").</summary>
    public required string? Model { get; init; }

    /// <summary>EXIF Software string (firmware version).</summary>
    public required string? Software { get; init; }

    /// <summary>EXIF DateTime (raw ASCII as stored in TTW).</summary>
    public required string? DateTime { get; init; }

    /// <summary>EXIF Artist (from TTW).</summary>
    public required string? Artist { get; init; }

    /// <summary>EXIF Copyright (from TTW).</summary>
    public required string? Copyright { get; init; }

    /// <summary>Length in bytes of the <c>\0WBG</c> sub-block payload (0 if absent).</summary>
    public required int WhiteBalanceGainsLength { get; init; }

    /// <summary>Length in bytes of the <c>\0RIF</c> sub-block payload (0 if absent).</summary>
    public required int RawInformationFileLength { get; init; }
}

/// <summary>
/// Public view of a single MRW sub-image. MRW exposes the embedded TTW TIFF
/// (carrying camera EXIF + sometimes a thumbnail strip) and the raw Bayer
/// mosaic CFA payload that follows the sub-block stream.
/// </summary>
public sealed record MrwSubImageInfo
{
    /// <summary>Logical role of this sub-image.</summary>
    public required MrwSubImageKind Kind { get; init; }

    /// <summary>Width in pixels (from TTW TIFF IFD 0 for TIFF; from PRD geometry for the CFA).</summary>
    public required int Width { get; init; }

    /// <summary>Height in pixels.</summary>
    public required int Height { get; init; }

    /// <summary>Byte offset of this sub-image's payload from the start of the MRW file.</summary>
    public required uint Offset { get; init; }

    /// <summary>Length in bytes of this sub-image's payload.</summary>
    public required uint Length { get; init; }

    /// <summary>Pixel format Mediar will emit when decoding this sub-image, or <see cref="PixelFormat.Unknown"/>.</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>True if Mediar can decode this sub-image with the bundled codecs.</summary>
    public required bool CanDecodePixels { get; init; }
}

/// <summary>The logical sub-image roles MRW exposes.</summary>
public enum MrwSubImageKind
{
    /// <summary>The TIFF Tag Wrapper (TTW) sub-block carrying EXIF and (optionally) a thumbnail strip.</summary>
    TiffTagWrapper = 0,
    /// <summary>The raw Bayer mosaic CFA payload that follows the sub-block stream.</summary>
    Cfa = 1,
}
