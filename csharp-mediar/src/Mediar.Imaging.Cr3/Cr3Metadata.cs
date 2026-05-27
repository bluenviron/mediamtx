namespace Mediar.Imaging.Cr3;

/// <summary>
/// Typed metadata extracted from a CR3 file: the major brand from
/// the <c>ftyp</c> box, the compatible brand list, and the EXIF tags
/// found inside the Canon UUID box's CMT1 TIFF IFD.
/// </summary>
public sealed record Cr3Metadata
{
    /// <summary>4-byte major brand from the <c>ftyp</c> box (always "crx " for CR3).</summary>
    public string MajorBrand { get; init; } = string.Empty;

    /// <summary>Minor version uint32 from the <c>ftyp</c> box.</summary>
    public uint MinorVersion { get; init; }

    /// <summary>List of compatible brand fourCCs from the <c>ftyp</c> box.</summary>
    public IReadOnlyList<string> CompatibleBrands { get; init; } = [];

    /// <summary>EXIF Make tag (typically "Canon").</summary>
    public string? Make { get; init; }

    /// <summary>EXIF Model tag (e.g. "Canon EOS R5").</summary>
    public string? Model { get; init; }

    /// <summary>EXIF Software tag.</summary>
    public string? Software { get; init; }

    /// <summary>EXIF DateTime tag (capture date in "YYYY:MM:DD HH:MM:SS" format).</summary>
    public string? DateTime { get; init; }

    /// <summary>EXIF Artist tag.</summary>
    public string? Artist { get; init; }

    /// <summary>EXIF Copyright tag.</summary>
    public string? Copyright { get; init; }

    /// <summary>True iff the Canon UUID box was successfully located.</summary>
    public bool HasCanonUuid { get; init; }

    /// <summary>True iff a CMT1 (main EXIF IFD) box was located inside the Canon UUID.</summary>
    public bool HasCmt1 { get; init; }

    /// <summary>True iff a CMT2 (EXIF sub-IFD) box was located inside the Canon UUID.</summary>
    public bool HasCmt2 { get; init; }

    /// <summary>True iff a CMT3 (Canon MakerNote) box was located inside the Canon UUID.</summary>
    public bool HasCmt3 { get; init; }

    /// <summary>True iff a CMT4 (GPS IFD) box was located inside the Canon UUID.</summary>
    public bool HasCmt4 { get; init; }

    /// <summary>Length in bytes of the raw CMT3 (Canon MakerNote) payload (not yet decoded).</summary>
    public int Cmt3ByteLength { get; init; }

    /// <summary>Typed EXIF sub-IFD metadata parsed from CMT2 (null when absent).</summary>
    public Cr3ExifMetadata? Exif { get; init; }

    /// <summary>Typed GPS IFD metadata parsed from CMT4 (null when absent).</summary>
    public Cr3GpsMetadata? Gps { get; init; }

    /// <summary>Typed Canon MakerNote metadata parsed from CMT3 (null when absent).</summary>
    public Cr3MakerNoteMetadata? MakerNote { get; init; }
}

/// <summary>
/// Canon MakerNote metadata (CR3 CMT3 box). Covers the well-documented
/// ASCII and integer top-level tags Canon writes in modern bodies; the
/// huge per-model proprietary arrays (CanonCameraSettings, CanonShotInfo,
/// CameraInfo, etc.) are intentionally skipped because their layout
/// differs per model and per firmware revision.
/// </summary>
/// <remarks>
/// Tag numbers follow Phil Harvey's ExifTool Canon MakerNote table at
/// <see href="https://exiftool.org/TagNames/Canon.html"/>. The IFD is
/// itself a standard TIFF stream so it reuses Mediar's CMT2 helpers.
/// </remarks>
public sealed record Cr3MakerNoteMetadata
{
    /// <summary>Canon MakerNote tag 0x0006 (ImageType), e.g. "Canon EOS R5".</summary>
    public string? ImageType { get; init; }

    /// <summary>Canon MakerNote tag 0x0007 (FirmwareRevision), e.g. "Firmware Version 1.6.0".</summary>
    public string? FirmwareRevision { get; init; }

    /// <summary>Canon MakerNote tag 0x0009 (OwnerName) - the camera owner-name field set in body menus.</summary>
    public string? OwnerName { get; init; }

    /// <summary>Canon MakerNote tag 0x000C (SerialNumber), camera body serial as a 32-bit unsigned integer.</summary>
    public uint? SerialNumber { get; init; }

    /// <summary>Canon MakerNote tag 0x0010 (ModelID), an internal Canon-assigned 32-bit identifier per body model.</summary>
    public uint? ModelId { get; init; }

    /// <summary>Canon MakerNote tag 0x0095 (LensModel), often more precise than the EXIF LensModel tag.</summary>
    public string? LensModel { get; init; }

    /// <summary>Canon MakerNote tag 0x0096 (InternalSerialNumber), an ASCII serial separate from the body SerialNumber.</summary>
    public string? InternalSerialNumber { get; init; }
}

/// <summary>
/// EXIF sub-IFD metadata (Canon CR3 CMT2 box). Mirrors the subset of
/// EXIF 2.32 tags Canon writes for every CR3 capture.
/// </summary>
public sealed record Cr3ExifMetadata
{
    /// <summary>EXIF tag 0x829A (ExposureTime), seconds.</summary>
    public double? ExposureTimeSeconds { get; init; }

    /// <summary>EXIF tag 0x829D (FNumber), e.g. 4.0 for f/4.0.</summary>
    public double? FNumber { get; init; }

    /// <summary>EXIF tag 0x8827 (ISOSpeedRatings).</summary>
    public ushort? IsoSpeedRatings { get; init; }

    /// <summary>EXIF tag 0x9003 (DateTimeOriginal), "YYYY:MM:DD HH:MM:SS".</summary>
    public string? DateTimeOriginal { get; init; }

    /// <summary>EXIF tag 0x9004 (DateTimeDigitized), "YYYY:MM:DD HH:MM:SS".</summary>
    public string? DateTimeDigitized { get; init; }

    /// <summary>EXIF tag 0x9204 (ExposureBiasValue), in stops (APEX).</summary>
    public double? ExposureBiasValue { get; init; }

    /// <summary>EXIF tag 0x920A (FocalLength), millimetres.</summary>
    public double? FocalLengthMm { get; init; }

    /// <summary>EXIF tag 0xA434 (LensModel).</summary>
    public string? LensModel { get; init; }

    /// <summary>EXIF tag 0xA433 (LensMake).</summary>
    public string? LensMake { get; init; }

    /// <summary>EXIF tag 0x9209 (Flash) status bits.</summary>
    public ushort? Flash { get; init; }

    /// <summary>EXIF tag 0x9207 (MeteringMode).</summary>
    public ushort? MeteringMode { get; init; }

    /// <summary>EXIF tag 0x8822 (ExposureProgram).</summary>
    public ushort? ExposureProgram { get; init; }

    /// <summary>EXIF tag 0xA403 (WhiteBalance): 0 = auto, 1 = manual.</summary>
    public ushort? WhiteBalance { get; init; }
}

/// <summary>
/// GPS IFD metadata (Canon CR3 CMT4 box). DMS triples are converted to
/// signed decimal degrees with reference letters (S/W) applied per EXIF
/// 2.32 § 4.6.6.
/// </summary>
public sealed record Cr3GpsMetadata
{
    /// <summary>Signed decimal latitude, degrees. Positive = north.</summary>
    public double? LatitudeDegrees { get; init; }

    /// <summary>Signed decimal longitude, degrees. Positive = east.</summary>
    public double? LongitudeDegrees { get; init; }

    /// <summary>Altitude in metres. Negative = below sea level.</summary>
    public double? AltitudeMeters { get; init; }

    /// <summary>Raw GPS latitude reference ("N" or "S").</summary>
    public string? LatitudeRef { get; init; }

    /// <summary>Raw GPS longitude reference ("E" or "W").</summary>
    public string? LongitudeRef { get; init; }

    /// <summary>UTC time-of-fix as "HH:MM:SS".</summary>
    public string? TimeStampUtc { get; init; }

    /// <summary>UTC date-of-fix as "YYYY:MM:DD".</summary>
    public string? DateStamp { get; init; }
}

/// <summary>
/// Kind of sub-image surfaced from a CR3 file.
/// </summary>
public enum Cr3SubImageKind
{
    /// <summary>The small embedded JPEG thumbnail (Canon UUID THMB box).</summary>
    Thumbnail = 0,

    /// <summary>The larger embedded JPEG preview (Canon PRVW UUID box).</summary>
    Preview = 1,

    /// <summary>The compressed raw sensor mosaic (CRAW track, not yet decodable).</summary>
    RawMosaic = 2,
}

/// <summary>
/// One sub-image discovered in a CR3 file. <see cref="Offset"/> is the
/// absolute file offset of the JPEG (or CRAW) byte slice.
/// </summary>
public sealed record Cr3SubImageInfo
{
    /// <summary>Kind of sub-image (thumbnail / preview / raw).</summary>
    public Cr3SubImageKind Kind { get; init; }

    /// <summary>Sub-image pixel width (probed via JpegReader for JPEG sub-images; 0 if unknown).</summary>
    public int Width { get; init; }

    /// <summary>Sub-image pixel height (probed via JpegReader for JPEG sub-images; 0 if unknown).</summary>
    public int Height { get; init; }

    /// <summary>Absolute file offset of the sub-image payload.</summary>
    public long Offset { get; init; }

    /// <summary>Sub-image payload length in bytes.</summary>
    public long Length { get; init; }

    /// <summary>True iff a Mediar reader exists for this sub-image's payload.</summary>
    public bool CanDecodePixels { get; init; }
}
