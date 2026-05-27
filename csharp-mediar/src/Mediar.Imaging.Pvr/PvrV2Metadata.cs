namespace Mediar.Imaging.Pvr;

#pragma warning disable CA1711 // PvrV2Flags is a [Flags] enum; the suffix matches its purpose.
/// <summary>
/// Bit flags packed into the upper 24 bits of the PVR v2 pixel-format
/// field (offset 0x10 in the v2 header). The low 8 bits hold a
/// <see cref="PvrV2FormatId"/> value.
/// </summary>
[Flags]
public enum PvrV2Flags : uint
{
    /// <summary>No flags set.</summary>
    None = 0,
    /// <summary>Texture has a bump-map encoding.</summary>
    BumpMap = 0x0000_0400,
    /// <summary>Texture has mip-maps following the largest level.</summary>
    HasMipmaps = 0x0000_0100,
    /// <summary>Texture is a 2D twiddled (Morton-order) layout.</summary>
    Twiddled = 0x0000_0200,
    /// <summary>Texture is a cubemap (six faces).</summary>
    Cubemap = 0x0000_1000,
    /// <summary>Texture is a 3D volume.</summary>
    VolumeTexture = 0x0000_4000,
    /// <summary>Texture is stored vertically flipped (origin top-left vs bottom-left).</summary>
    VerticalFlip = 0x0001_0000,
    /// <summary>Texture has premultiplied alpha.</summary>
    PremultipliedAlpha = 0x0000_8000,
}

/// <summary>
/// PowerVR Texture v2 ("legacy") header parsed by
/// <see cref="PvrV2Reader"/>. The container always carries 13 u32 fields
/// little-endian (52 bytes total) followed by an optional metadata
/// block and then the pixel payload.
/// </summary>
public sealed record PvrV2Metadata
{
    /// <summary>Declared header size in bytes (always 52 in canonical files).</summary>
    public required uint HeaderSize { get; init; }
    /// <summary>Image height (pixels).</summary>
    public required uint Height { get; init; }
    /// <summary>Image width (pixels).</summary>
    public required uint Width { get; init; }
    /// <summary>Number of mip-map levels following the base image (0 = base only).</summary>
    public required uint MipMapCount { get; init; }
    /// <summary>Full 32-bit pixel-format word combining <see cref="FormatId"/> + <see cref="Flags"/>.</summary>
    public required uint PixelFormatWord { get; init; }
    /// <summary>Format identifier extracted from the low 8 bits of <see cref="PixelFormatWord"/>.</summary>
    public required PvrV2FormatId FormatId { get; init; }
    /// <summary>Flag bits extracted from the upper 24 bits of <see cref="PixelFormatWord"/>.</summary>
    public required PvrV2Flags Flags { get; init; }
    /// <summary>Declared compressed payload byte size.</summary>
    public required uint DataLength { get; init; }
    /// <summary>Bits per pixel for the uncompressed format (informational).</summary>
    public required uint BitsPerPixel { get; init; }
    /// <summary>Red channel bit-mask (uncompressed packed formats only).</summary>
    public required uint RedMask { get; init; }
    /// <summary>Green channel bit-mask (uncompressed packed formats only).</summary>
    public required uint GreenMask { get; init; }
    /// <summary>Blue channel bit-mask (uncompressed packed formats only).</summary>
    public required uint BlueMask { get; init; }
    /// <summary>Alpha channel bit-mask (uncompressed packed formats only).</summary>
    public required uint AlphaMask { get; init; }
    /// <summary>Magic word at offset 0x2C, always 'PVR!' (0x21525650 little-endian).</summary>
    public required uint Magic { get; init; }
    /// <summary>Number of surfaces (array textures + cubemap faces).</summary>
    public required uint NumSurfaces { get; init; }
}

/// <summary>
/// Per (surface, mip-level) entry discovered by <see cref="PvrV2Reader"/>.
/// </summary>
public sealed record PvrV2LevelInfo
{
    /// <summary>Mip-level index, 0 = base level.</summary>
    public required int Level { get; init; }
    /// <summary>Surface index (cubemap face, array slice).</summary>
    public required int Surface { get; init; }
    /// <summary>Per-level width in pixels.</summary>
    public required int Width { get; init; }
    /// <summary>Per-level height in pixels.</summary>
    public required int Height { get; init; }
    /// <summary>Absolute file offset to the level's payload bytes.</summary>
    public required long Offset { get; init; }
    /// <summary>Level payload size in bytes.</summary>
    public required long Length { get; init; }
}
