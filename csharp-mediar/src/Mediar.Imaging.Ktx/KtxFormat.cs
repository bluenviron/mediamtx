using Mediar.Codecs.Bcn;

namespace Mediar.Imaging.Ktx;

/// <summary>
/// Format-identification helpers shared by <see cref="KtxReader"/> (KTX 1.x,
/// OpenGL <c>glInternalFormat</c> tokens) and <see cref="Ktx2Reader"/> (KTX 2.x,
/// Vulkan <c>VkFormat</c> enum). Maps recognised compressed enums to
/// <see cref="BcnFormat"/> and well-known uncompressed enums to
/// <see cref="PixelFormat"/> for direct copy.
/// </summary>
public static class KtxFormat
{
    /// <summary>
    /// Map an OpenGL <c>glInternalFormat</c> value to a Mediar
    /// <see cref="BcnFormat"/>. Returns <see cref="BcnFormat.None"/> for
    /// non-BCn (or uncompressed) formats.
    /// </summary>
    public static BcnFormat MapGlInternalFormat(uint glInternalFormat) => glInternalFormat switch
    {
        // GL_COMPRESSED_RGB_S3TC_DXT1_EXT / RGBA variant
        0x83F0 or 0x83F1 => BcnFormat.Bc1,
        // GL_COMPRESSED_RGBA_S3TC_DXT3_EXT
        0x83F2 => BcnFormat.Bc2,
        // GL_COMPRESSED_RGBA_S3TC_DXT5_EXT
        0x83F3 => BcnFormat.Bc3,
        // GL_COMPRESSED_(SIGNED_)RED_RGTC1
        0x8DBB or 0x8DBC => BcnFormat.Bc4,
        // GL_COMPRESSED_(SIGNED_)RG_RGTC2
        0x8DBD or 0x8DBE => BcnFormat.Bc5,
        // GL_COMPRESSED_RGB_BPTC_UNSIGNED_FLOAT
        0x8E8F => BcnFormat.Bc6hUf16,
        // GL_COMPRESSED_RGB_BPTC_SIGNED_FLOAT
        0x8E8E => BcnFormat.Bc6hSf16,
        // GL_COMPRESSED_RGBA_BPTC_UNORM / SRGB variant
        0x8E8C or 0x8E8D => BcnFormat.Bc7,
        _ => BcnFormat.None,
    };

    /// <summary>
    /// Map an OpenGL <c>glInternalFormat</c> to a top-down uncompressed
    /// <see cref="PixelFormat"/>. Returns <see cref="PixelFormat.Unknown"/>
    /// for compressed or unrecognised formats.
    /// </summary>
    public static PixelFormat MapGlUncompressed(uint glInternalFormat) => glInternalFormat switch
    {
        // GL_R8 / GL_R8_SNORM
        0x8229 or 0x8F94 => PixelFormat.Gray8,
        // GL_RGB8 / GL_SRGB8
        0x8051 or 0x8C41 => PixelFormat.Rgb24,
        // GL_RGBA8 / GL_SRGB8_ALPHA8
        0x8058 or 0x8C43 => PixelFormat.Rgba32,
        _ => PixelFormat.Unknown,
    };

    /// <summary>
    /// Map a Vulkan <c>VkFormat</c> enum value to a Mediar
    /// <see cref="BcnFormat"/>. Returns <see cref="BcnFormat.None"/> for
    /// non-BCn (or uncompressed) formats.
    /// </summary>
    public static BcnFormat MapVkFormat(uint vkFormat) => vkFormat switch
    {
        // VK_FORMAT_BC1_RGB_UNORM_BLOCK / SRGB / RGBA variants
        131 or 132 or 133 or 134 => BcnFormat.Bc1,
        // VK_FORMAT_BC2_UNORM_BLOCK / SRGB
        135 or 136 => BcnFormat.Bc2,
        // VK_FORMAT_BC3_UNORM_BLOCK / SRGB
        137 or 138 => BcnFormat.Bc3,
        // VK_FORMAT_BC4_UNORM_BLOCK / SNORM
        139 or 140 => BcnFormat.Bc4,
        // VK_FORMAT_BC5_UNORM_BLOCK / SNORM
        141 or 142 => BcnFormat.Bc5,
        // VK_FORMAT_BC6H_UFLOAT_BLOCK
        143 => BcnFormat.Bc6hUf16,
        // VK_FORMAT_BC6H_SFLOAT_BLOCK
        144 => BcnFormat.Bc6hSf16,
        // VK_FORMAT_BC7_UNORM_BLOCK / SRGB
        145 or 146 => BcnFormat.Bc7,
        _ => BcnFormat.None,
    };

    /// <summary>
    /// Map a Vulkan <c>VkFormat</c> to a top-down uncompressed
    /// <see cref="PixelFormat"/>. Returns <see cref="PixelFormat.Unknown"/>
    /// for compressed or unrecognised formats.
    /// </summary>
    public static PixelFormat MapVkUncompressed(uint vkFormat) => vkFormat switch
    {
        // VK_FORMAT_R8_UNORM / SNORM / SRGB
        9 or 10 or 15 => PixelFormat.Gray8,
        // VK_FORMAT_R8G8B8_UNORM / SRGB
        23 or 29 => PixelFormat.Rgb24,
        // VK_FORMAT_R8G8B8A8_UNORM / SRGB
        37 or 43 => PixelFormat.Rgba32,
        // VK_FORMAT_B8G8R8A8_UNORM / SRGB
        44 or 50 => PixelFormat.Bgra32,
        _ => PixelFormat.Unknown,
    };

    /// <summary>
    /// Bits-per-pixel of a decoded BCn surface (BC1/BC4 = 4bpp blocks,
    /// others = 8bpp blocks). Used to compute mip payload bounds.
    /// </summary>
    public static int BcnBitsPerPixel(BcnFormat f) => f switch
    {
        BcnFormat.Bc1 or BcnFormat.Bc4 => 4,
        BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc5
            or BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16 or BcnFormat.Bc7 => 8,
        _ => 0,
    };

    /// <summary>Decoded-pixel format for a BCn surface after decode.</summary>
    public static PixelFormat BcnToDecodedPixelFormat(BcnFormat f) => f switch
    {
        BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc7 => PixelFormat.Bgra32,
        BcnFormat.Bc4 => PixelFormat.Gray8,
        BcnFormat.Bc5 => PixelFormat.Rgb24,
        BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16 => PixelFormat.Rgb96Float,
        _ => PixelFormat.Unknown,
    };

    /// <summary>
    /// Decode a BCn surface to a top-down byte buffer in the layout reported
    /// by <see cref="BcnToDecodedPixelFormat"/>. Returns the decoded buffer
    /// or throws <see cref="NotSupportedException"/> for unrecognised formats.
    /// </summary>
    public static (byte[] Pixels, int Stride, PixelFormat Format) DecodeBcn(
        BcnFormat f, ReadOnlySpan<byte> payload, int width, int height)
    {
        return f switch
        {
            BcnFormat.Bc1 => (BcnDecoder.DecodeBc1(payload, width, height), width * 4, PixelFormat.Bgra32),
            BcnFormat.Bc2 => (BcnDecoder.DecodeBc2(payload, width, height), width * 4, PixelFormat.Bgra32),
            BcnFormat.Bc3 => (BcnDecoder.DecodeBc3(payload, width, height), width * 4, PixelFormat.Bgra32),
            BcnFormat.Bc4 => (BcnDecoder.DecodeBc4(payload, width, height), width, PixelFormat.Gray8),
            BcnFormat.Bc5 => (BcnDecoder.DecodeBc5(payload, width, height), width * 3, PixelFormat.Rgb24),
            BcnFormat.Bc7 => (Bc7Decoder.DecodeBc7(payload, width, height), width * 4, PixelFormat.Bgra32),
            BcnFormat.Bc6hUf16 => (Bc6hDecoder.DecodeBc6h(payload, width, height, isSigned: false), width * 12, PixelFormat.Rgb96Float),
            BcnFormat.Bc6hSf16 => (Bc6hDecoder.DecodeBc6h(payload, width, height, isSigned: true), width * 12, PixelFormat.Rgb96Float),
            _ => throw new NotSupportedException($"KTX/KTX2 BCn format {f} cannot be decoded."),
        };
    }
}
