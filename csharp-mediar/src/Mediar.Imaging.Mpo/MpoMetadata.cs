namespace Mediar.Imaging.Mpo;

/// <summary>
/// Decoded values from an MPO file's MP Index IFD (the
/// Multi-Picture Format APP2 segment defined by CIPA DC-007).
/// </summary>
public sealed record MpoMetadata
{
    /// <summary>4-byte ASCII MPF version (e.g. "0100").</summary>
    public string Version { get; init; } = string.Empty;

    /// <summary>The number of sub-images declared by tag 0xB001.</summary>
    public uint NumberOfImages { get; init; }

    /// <summary>
    /// Byte-order of the MPF TIFF stream: "II" = little-endian (the
    /// CIPA reference and all consumer cameras), "MM" = big-endian.
    /// </summary>
    public string ByteOrder { get; init; } = "II";

    /// <summary>
    /// 33-character ASCII image unique ID strings emitted by tag
    /// 0xB003 (ImageUIDList), if present. Empty when the tag is absent.
    /// </summary>
    public IReadOnlyList<string> ImageUids { get; init; } = [];
}

/// <summary>
/// Image-type classification of an MPO sub-image, derived from the
/// MPType field (lower 24 bits) of the Individual Image Attribute
/// uint32 stored in the MP Entry record. Values are per CIPA DC-007.
/// </summary>
public enum MpoImageKind : uint
{
    /// <summary>Sub-image whose type code is not one of the values defined by CIPA DC-007.</summary>
    Unknown = 0,

    /// <summary>Baseline MP primary image (the first JPEG in the file).</summary>
    BaselineMpPrimary = 0x030000,

    /// <summary>Large thumbnail (VGA-equivalent, 16:9 or 4:3 frame).</summary>
    LargeThumbnailClass1 = 0x010001,

    /// <summary>Large thumbnail (full HD class).</summary>
    LargeThumbnailClass2 = 0x010002,

    /// <summary>Multi-frame panorama sub-image.</summary>
    MultiFramePanorama = 0x020001,

    /// <summary>Multi-frame disparity sub-image (stereoscopic 3D).</summary>
    MultiFrameDisparity = 0x020002,

    /// <summary>Multi-frame multi-angle sub-image.</summary>
    MultiAngle = 0x020003,
}

/// <summary>
/// One sub-image entry decoded from the MP Index IFD's MPEntry table
/// (tag 0xB002, 16 bytes per entry). All offsets are absolute file
/// positions (the raw value is relative to the MP Endian header inside
/// the MPF segment; <see cref="MpoReader"/> resolves that to a file-
/// relative position).
/// </summary>
public sealed record MpoSubImageInfo
{
    /// <summary>Zero-based index of this entry in the MP Entry table.</summary>
    public int Index { get; init; }

    /// <summary>
    /// Decoded image-type classification (lower 24 bits of the
    /// Individual Image Attribute uint32).
    /// </summary>
    public MpoImageKind Kind { get; init; }

    /// <summary>True if this sub-image is flagged as a representative image.</summary>
    public bool IsRepresentative { get; init; }

    /// <summary>True if this sub-image is flagged as the dependent parent image.</summary>
    public bool IsDependentParent { get; init; }

    /// <summary>True if this sub-image is flagged as a dependent child image.</summary>
    public bool IsDependentChild { get; init; }

    /// <summary>Raw 32-bit Individual Image Attribute word.</summary>
    public uint RawAttribute { get; init; }

    /// <summary>Sub-image size in bytes (the JPEG slice including SOI..EOI).</summary>
    public uint Length { get; init; }

    /// <summary>Absolute file offset of the JPEG slice.</summary>
    public long Offset { get; init; }

    /// <summary>1-based MPEntry number of the first dependent image (0 = none).</summary>
    public ushort DependentImage1 { get; init; }

    /// <summary>1-based MPEntry number of the second dependent image (0 = none).</summary>
    public ushort DependentImage2 { get; init; }

    /// <summary>Sub-image pixel width (probed via <see cref="Mediar.Imaging.Jpeg.JpegReader"/>; 0 if probing fails).</summary>
    public int Width { get; init; }

    /// <summary>Sub-image pixel height (probed via <see cref="Mediar.Imaging.Jpeg.JpegReader"/>; 0 if probing fails).</summary>
    public int Height { get; init; }

    /// <summary>True when <see cref="Mediar.Imaging.Jpeg.JpegReader"/> could parse the sub-image's SOF header at <c>Open</c> time.</summary>
    public bool CanDecodePixels { get; init; }
}
