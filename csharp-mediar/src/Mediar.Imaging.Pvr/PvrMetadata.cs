using System.Collections.Frozen;
using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;

namespace Mediar.Imaging.Pvr;

/// <summary>
/// One mip / array surface / cube face entry discovered in a PVR v3
/// container. The PVR3 spec stores the texture data with the iteration
/// order: for each MIP level (largest first), for each surface, for each
/// face, for each depth slice, payload.
/// </summary>
public sealed record PvrLevelInfo
{
    /// <summary>Mip level index (0 = largest, levelCount-1 = smallest).</summary>
    public required int Level { get; init; }

    /// <summary>Array surface index (0 for non-array textures).</summary>
    public required int Surface { get; init; }

    /// <summary>Cube face index 0..5 for cubemaps, 0 for 2D / 3D.</summary>
    public required int Face { get; init; }

    /// <summary>Pixel width at this mip level.</summary>
    public required int Width { get; init; }

    /// <summary>Pixel height at this mip level.</summary>
    public required int Height { get; init; }

    /// <summary>Depth slice count at this mip level (1 for 2D).</summary>
    public required int Depth { get; init; }

    /// <summary>Absolute file offset of the level payload.</summary>
    public required long Offset { get; init; }

    /// <summary>Byte length of the level payload (single surface/face/slice).</summary>
    public required long Length { get; init; }
}

/// <summary>
/// One typed entry inside the PVR v3 metadata block. Entries are
/// (4-byte FourCC, 4-byte key, byte[] data) triples laid back-to-back
/// after the fixed header. Common FourCC = "PVR\x03"; well-known keys
/// include 1 (texture atlas), 2 (bump-map info), 3 (cube-map order),
/// 4 (orientation), 5 (border), 6 (padding).
/// </summary>
public sealed record PvrMetaEntry
{
    /// <summary>4-byte FourCC identifying the vendor / namespace.</summary>
    public required uint FourCc { get; init; }

    /// <summary>Vendor-defined 32-bit key.</summary>
    public required uint Key { get; init; }

    /// <summary>Raw bytes of the metadata payload.</summary>
    public required byte[] Data { get; init; }
}

/// <summary>
/// Parsed PVR v3 header + metadata block. Populated from the 52-byte
/// fixed header.
/// </summary>
public sealed record PvrMetadata
{
    /// <summary>True when the file's version word reads little-endian (0x03525650).</summary>
    public required bool LittleEndian { get; init; }

    /// <summary>Raw 32-bit flags field (bit 1 = premultiplied alpha).</summary>
    public required uint Flags { get; init; }

    /// <summary>Full 64-bit pixel-format value (low 32 = format id when high 32 = 0).</summary>
    public required ulong PixelFormat { get; init; }

    /// <summary>
    /// Decoded format-id from the low 32 bits when the high 32 bits are
    /// zero. Returns <see cref="PvrFormatId.None"/> when the format is a
    /// custom 64-bit channel descriptor.
    /// </summary>
    public required PvrFormatId FormatId { get; init; }

    /// <summary>Colour space (0 = linear RGB, 1 = sRGB).</summary>
    public required uint ColourSpace { get; init; }

    /// <summary>Per-channel data type code (0..12: u8 norm, s8 norm, ...).</summary>
    public required uint ChannelType { get; init; }

    /// <summary>Texture height at mip level 0.</summary>
    public required uint Height { get; init; }

    /// <summary>Texture width at mip level 0.</summary>
    public required uint Width { get; init; }

    /// <summary>Texture depth at mip level 0 (1 for 2D).</summary>
    public required uint Depth { get; init; }

    /// <summary>Array surface count (1 for non-array textures).</summary>
    public required uint NumSurfaces { get; init; }

    /// <summary>Face count (1 for 2D, 6 for cubemaps).</summary>
    public required uint NumFaces { get; init; }

    /// <summary>Mip map count (>= 1).</summary>
    public required uint NumMipMaps { get; init; }

    /// <summary>Metadata block size in bytes (immediately follows the header).</summary>
    public required uint MetaDataSize { get; init; }

    /// <summary>Detected BCn format (BcnFormat.None when not BCn).</summary>
    public required BcnFormat Bcn { get; init; }

    /// <summary>Detected ETC / EAC format (EtcFormat.None when not ETC).</summary>
    public required EtcFormat Etc { get; init; }

    /// <summary>Metadata block entries.</summary>
    public required IReadOnlyList<PvrMetaEntry> MetaEntries { get; init; }

    /// <summary>FourCC -> Key -> raw byte payload view of <see cref="MetaEntries"/>.</summary>
    public required FrozenDictionary<ulong, byte[]> MetaByFourCcKey { get; init; }
}
