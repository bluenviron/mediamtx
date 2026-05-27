using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;

namespace Mediar.Imaging.Ktx;

/// <summary>
/// Format-identification helpers shared by <see cref="KtxReader"/> (KTX 1.x,
/// OpenGL <c>glInternalFormat</c> tokens) and <see cref="Ktx2Reader"/> (KTX 2.x,
/// Vulkan <c>VkFormat</c> enum). Maps recognised compressed enums to
/// <see cref="BcnFormat"/> / <see cref="EtcFormat"/> and well-known
/// uncompressed enums to <see cref="PixelFormat"/> for direct copy.
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
    /// Map an OpenGL <c>glInternalFormat</c> value to a Mediar
    /// <see cref="EtcFormat"/>. Returns <see cref="EtcFormat.None"/> for
    /// non-ETC formats.
    /// </summary>
    public static EtcFormat MapGlInternalFormatEtc(uint glInternalFormat) => glInternalFormat switch
    {
        // GL_ETC1_RGB8_OES
        0x8D64 => EtcFormat.Etc1Rgb,
        // GL_COMPRESSED_RGB8_ETC2 / GL_COMPRESSED_SRGB8_ETC2
        0x9274 or 0x9275 => EtcFormat.Etc2Rgb,
        // GL_COMPRESSED_RGB8_PUNCHTHROUGH_ALPHA1_ETC2 / SRGB
        0x9276 or 0x9277 => EtcFormat.Etc2RgbA1,
        // GL_COMPRESSED_RGBA8_ETC2_EAC / SRGB8_ALPHA8_ETC2_EAC
        0x9278 or 0x9279 => EtcFormat.Etc2Rgba8,
        // GL_COMPRESSED_R11_EAC
        0x9270 => EtcFormat.EacR11Unorm,
        // GL_COMPRESSED_SIGNED_R11_EAC
        0x9271 => EtcFormat.EacR11Snorm,
        // GL_COMPRESSED_RG11_EAC
        0x9272 => EtcFormat.EacRg11Unorm,
        // GL_COMPRESSED_SIGNED_RG11_EAC
        0x9273 => EtcFormat.EacRg11Snorm,
        _ => EtcFormat.None,
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
        // GL_R16 / GL_R16_SNORM
        0x822A or 0x8F98 => PixelFormat.Gray16,
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
    /// Map a Vulkan <c>VkFormat</c> enum value to a Mediar
    /// <see cref="EtcFormat"/>. Returns <see cref="EtcFormat.None"/> for
    /// non-ETC formats.
    /// </summary>
    public static EtcFormat MapVkFormatEtc(uint vkFormat) => vkFormat switch
    {
        // VK_FORMAT_ETC2_R8G8B8_UNORM_BLOCK / SRGB
        147 or 148 => EtcFormat.Etc2Rgb,
        // VK_FORMAT_ETC2_R8G8B8A1_UNORM_BLOCK / SRGB
        149 or 150 => EtcFormat.Etc2RgbA1,
        // VK_FORMAT_ETC2_R8G8B8A8_UNORM_BLOCK / SRGB
        151 or 152 => EtcFormat.Etc2Rgba8,
        // VK_FORMAT_EAC_R11_UNORM_BLOCK
        153 => EtcFormat.EacR11Unorm,
        // VK_FORMAT_EAC_R11_SNORM_BLOCK
        154 => EtcFormat.EacR11Snorm,
        // VK_FORMAT_EAC_R11G11_UNORM_BLOCK
        155 => EtcFormat.EacRg11Unorm,
        // VK_FORMAT_EAC_R11G11_SNORM_BLOCK
        156 => EtcFormat.EacRg11Snorm,
        _ => EtcFormat.None,
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
        // VK_FORMAT_B8G8R8_UNORM / SRGB
        30 or 36 => PixelFormat.Bgr24,
        // VK_FORMAT_R8G8B8A8_UNORM / SRGB
        37 or 43 => PixelFormat.Rgba32,
        // VK_FORMAT_B8G8R8A8_UNORM / SRGB
        44 or 50 => PixelFormat.Bgra32,
        // VK_FORMAT_R16_UNORM / SNORM
        70 or 71 => PixelFormat.Gray16,
        // VK_FORMAT_R16G16_UNORM / SNORM
        77 or 78 => PixelFormat.Rg32,
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

    /// <summary>Decoded-pixel format for an ETC / EAC surface after decode.</summary>
    public static PixelFormat EtcToDecodedPixelFormat(EtcFormat f) => f switch
    {
        EtcFormat.Etc1Rgb or EtcFormat.Etc2Rgb or EtcFormat.Etc2RgbA1
            or EtcFormat.Etc2Rgba8 => PixelFormat.Rgba32,
        EtcFormat.EacR11Unorm or EtcFormat.EacR11Snorm => PixelFormat.Gray16,
        EtcFormat.EacRg11Unorm or EtcFormat.EacRg11Snorm => PixelFormat.Rg32,
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

    /// <summary>
    /// Decode an ETC / EAC surface to a top-down byte buffer in the layout
    /// reported by <see cref="EtcToDecodedPixelFormat"/>. Throws
    /// <see cref="NotSupportedException"/> for formats not yet wired into
    /// the reader (currently EAC RG11 unorm / snorm).
    /// </summary>
    public static (byte[] Pixels, int Stride, PixelFormat Format) DecodeEtc(
        EtcFormat f, ReadOnlySpan<byte> payload, int width, int height)
    {
        return f switch
        {
            EtcFormat.Etc1Rgb => (EtcDecoder.DecodeEtc1(payload, width, height), width * 4, PixelFormat.Rgba32),
            EtcFormat.Etc2Rgb => (EtcDecoder.DecodeEtc2Rgb(payload, width, height), width * 4, PixelFormat.Rgba32),
            EtcFormat.Etc2RgbA1 => (EtcDecoder.DecodeEtc2RgbA1(payload, width, height), width * 4, PixelFormat.Rgba32),
            EtcFormat.Etc2Rgba8 => (EtcDecoder.DecodeEtc2Rgba8(payload, width, height), width * 4, PixelFormat.Rgba32),
            EtcFormat.EacR11Unorm => (EtcDecoder.DecodeEacR11Unorm(payload, width, height), width * 2, PixelFormat.Gray16),
            EtcFormat.EacR11Snorm => (EtcDecoder.DecodeEacR11Snorm(payload, width, height), width * 2, PixelFormat.Gray16),
            EtcFormat.EacRg11Unorm => (EtcDecoder.DecodeEacRg11Unorm(payload, width, height), width * 4, PixelFormat.Rg32),
            EtcFormat.EacRg11Snorm => (EtcDecoder.DecodeEacRg11Snorm(payload, width, height), width * 4, PixelFormat.Rg32),
            _ => throw new NotSupportedException($"KTX/KTX2 ETC format {f} cannot be decoded."),
        };
    }

    /// <summary>True when <see cref="DecodeEtc"/> can decode the format.</summary>
    public static bool CanDecodeEtc(EtcFormat f) => f is
        EtcFormat.Etc1Rgb or EtcFormat.Etc2Rgb or EtcFormat.Etc2RgbA1
        or EtcFormat.Etc2Rgba8 or EtcFormat.EacR11Unorm or EtcFormat.EacR11Snorm
        or EtcFormat.EacRg11Unorm or EtcFormat.EacRg11Snorm;
}
