using System.Collections.Frozen;
using Mediar.Codecs.Bcn;

namespace Mediar.Imaging.Ktx;

/// <summary>
/// One mip level discovered in a KTX or KTX2 container. The reader exposes
/// every level / array layer / cube face as a separate
/// <see cref="KtxLevelInfo"/> so callers can enumerate the full texture
/// pyramid without having to re-parse the level index.
/// </summary>
public sealed record KtxLevelInfo
{
    /// <summary>Mip level index (0 = largest, levelCount-1 = smallest).</summary>
    public required int Level { get; init; }

    /// <summary>Array-layer index (0 for non-array textures).</summary>
    public required int ArrayLayer { get; init; }

    /// <summary>Cube face index 0..5 for cubemaps, 0 for 2D / 3D.</summary>
    public required int Face { get; init; }

    /// <summary>Pixel width at this mip level.</summary>
    public required int Width { get; init; }

    /// <summary>Pixel height at this mip level.</summary>
    public required int Height { get; init; }

    /// <summary>3D-texture depth slice (1 for 2D).</summary>
    public required int Depth { get; init; }

    /// <summary>Absolute file offset of the level payload (after any supercompression).</summary>
    public required long Offset { get; init; }

    /// <summary>Byte length of the level payload (single face/layer/slice).</summary>
    public required long Length { get; init; }
}

/// <summary>
/// Parsed KTX 1.x header + key-value metadata pool. Populated from the
/// 64-byte fixed header that follows the 12-byte Khronos identifier.
/// </summary>
public sealed record KtxMetadata
{
    /// <summary>True when the file's endianness marker reads as native LE (no byte-swap required).</summary>
    public required bool LittleEndian { get; init; }

    /// <summary>OpenGL <c>glType</c> (0 for compressed formats).</summary>
    public required uint GlType { get; init; }

    /// <summary>OpenGL <c>glTypeSize</c> (1 for compressed formats).</summary>
    public required uint GlTypeSize { get; init; }

    /// <summary>OpenGL <c>glFormat</c> (0 for compressed formats).</summary>
    public required uint GlFormat { get; init; }

    /// <summary>OpenGL <c>glInternalFormat</c> token (e.g. 0x83F1 = COMPRESSED_RGBA_S3TC_DXT1_EXT).</summary>
    public required uint GlInternalFormat { get; init; }

    /// <summary>OpenGL <c>glBaseInternalFormat</c> (e.g. GL_RGB = 0x1907 / GL_RGBA = 0x1908).</summary>
    public required uint GlBaseInternalFormat { get; init; }

    /// <summary>Texture array element count (0 for non-array textures).</summary>
    public required uint ArrayElementCount { get; init; }

    /// <summary>Texture face count (1 for 2D, 6 for cubemaps).</summary>
    public required uint FaceCount { get; init; }

    /// <summary>Detected BCn format (BcnFormat.None when uncompressed or unrecognised).</summary>
    public required BcnFormat Bcn { get; init; }

    /// <summary>Key-value metadata pool from the bytesOfKeyValueData section.</summary>
    public required FrozenDictionary<string, string> KeyValues { get; init; }
}

/// <summary>
/// Parsed KTX 2.x header + index + level index + key-value metadata.
/// Populated from the fixed-size header that follows the 12-byte
/// Khronos identifier.
/// </summary>
public sealed record Ktx2Metadata
{
    /// <summary>Vulkan <c>VkFormat</c> enum value (0 = Basis Universal supercompressed).</summary>
    public required uint VkFormat { get; init; }

    /// <summary>Bytes per type unit (for uncompressed formats); 1 for compressed.</summary>
    public required uint TypeSize { get; init; }

    /// <summary>Texture array layer count (0 for non-array textures).</summary>
    public required uint LayerCount { get; init; }

    /// <summary>Texture face count (1 for 2D, 6 for cubemaps).</summary>
    public required uint FaceCount { get; init; }

    /// <summary>Supercompression scheme code (0=None, 1=BasisLZ, 2=Zstd, 3=ZLIB).</summary>
    public required uint SupercompressionScheme { get; init; }

    /// <summary>Detected BCn format (BcnFormat.None when uncompressed or supercompressed).</summary>
    public required BcnFormat Bcn { get; init; }

    /// <summary>Key-value metadata pool from the kvd section.</summary>
    public required FrozenDictionary<string, string> KeyValues { get; init; }
}
