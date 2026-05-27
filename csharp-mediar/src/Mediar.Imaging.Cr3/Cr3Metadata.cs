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
