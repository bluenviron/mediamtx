namespace Mediar.Imaging.X3f;

/// <summary>
/// Strongly-typed metadata parsed from a Sigma Foveon X3F file. Populated
/// from the "FOVb" header at file start, the trailing "SECd" directory,
/// and the discovered "SECp" property section(s).
/// </summary>
public sealed record X3fMetadata
{
    /// <summary>Major header version (e.g. 2 for v2.x).</summary>
    public required ushort VersionMajor { get; init; }

    /// <summary>Minor header version (e.g. 3 for v2.3).</summary>
    public required ushort VersionMinor { get; init; }

    /// <summary>16-byte file unique identifier, lower-case hex without separators.</summary>
    public required string FileIdHex { get; init; }

    /// <summary>32-bit file mark from header bytes 24..27.</summary>
    public required uint FileMark { get; init; }

    /// <summary>Image rotation (0/90/180/270) from extended header (v >= 2.1). Null when not present.</summary>
    public uint? Rotation { get; init; }

    /// <summary>32-char white-balance label from extended header (v >= 2.1). Null when not present.</summary>
    public string? WhiteBalanceLabel { get; init; }

    /// <summary>Camera-make string parsed from the property section ("CAMMANUF" key).</summary>
    public string? Make { get; init; }

    /// <summary>Camera-model string parsed from the property section ("CAMMODEL" key).</summary>
    public string? Model { get; init; }

    /// <summary>Firmware-version string parsed from the property section ("FIRMVERS" key).</summary>
    public string? Software { get; init; }

    /// <summary>Capture-time string parsed from the property section ("TIME" key, EXIF format).</summary>
    public string? DateTime { get; init; }

    /// <summary>Total number of directory entries discovered in the "SECd" trailer.</summary>
    public required int EntryCount { get; init; }
}

/// <summary>
/// One directory entry from an X3F file. Each entry references one section
/// (image, properties, or camera metadata) with its absolute file offset,
/// length, and four-character section identifier.
/// </summary>
public sealed record X3fSubImageInfo
{
    /// <summary>Classification of the section (JpegPreview / RawMosaic / Properties / CameraMetadata / Unknown).</summary>
    public required X3fSubImageKind Kind { get; init; }

    /// <summary>The four-character section identifier from the directory entry (e.g. "IMA2", "PROP", "CAMF").</summary>
    public required string SectionId { get; init; }

    /// <summary>Absolute file offset of the section header.</summary>
    public required uint Offset { get; init; }

    /// <summary>Byte length of the section (header + payload).</summary>
    public required uint Length { get; init; }

    /// <summary>Image-section width in pixels (0 for non-image sections).</summary>
    public required int Width { get; init; }

    /// <summary>Image-section height in pixels (0 for non-image sections).</summary>
    public required int Height { get; init; }

    /// <summary>Image-section row stride in bytes (0 for non-image sections).</summary>
    public required uint RowStride { get; init; }

    /// <summary>Image-section type code from the section header (1 = thumbnail, 2/3 = raw mosaic).</summary>
    public required uint ImageType { get; init; }

    /// <summary>Image-section data-format code (3 = JPEG, 6 = uncompressed raw, 11/18 = Foveon-packed raw).</summary>
    public required uint DataFormat { get; init; }

    /// <summary>True when Mediar can decode pixels for this section (currently only JPEG previews).</summary>
    public required bool CanDecodePixels { get; init; }
}

/// <summary>
/// Classification of an X3F directory entry. The proprietary Foveon raw
/// mosaic decoder is out of session scope, so RawMosaic sections are
/// surfaced as <see cref="X3fSubImageInfo.CanDecodePixels"/> = false.
/// </summary>
public enum X3fSubImageKind
{
    /// <summary>Unrecognised section identifier; payload bytes are surfaced raw.</summary>
    Unknown = 0,

    /// <summary>Embedded JPEG preview ("IMA2"/"IMAG" with data format 3).</summary>
    JpegPreview,

    /// <summary>Foveon X3 raw mosaic ("IMA2"/"IMAG" with data format 6/11/18). Decodable only when the Foveon codec is wired in (future scope).</summary>
    RawMosaic,

    /// <summary>Property section ("PROP") - key/value metadata pool.</summary>
    Properties,

    /// <summary>Camera-metadata section ("CAMF") - proprietary compressed binary blob.</summary>
    CameraMetadata,
}
