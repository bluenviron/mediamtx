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
        // GL_LUMINANCE8
        0x8040 => PixelFormat.Gray8,
        // GL_LUMINANCE8_ALPHA8
        0x8045 => PixelFormat.GrayAlpha16,
        // GL_RGB8 / GL_SRGB8
        0x8051 or 0x8C41 => PixelFormat.Rgb24,
        // GL_RGBA8 / GL_SRGB8_ALPHA8
        0x8058 or 0x8C43 => PixelFormat.Rgba32,
        // GL_R16 / GL_R16_SNORM
        0x822A or 0x8F98 => PixelFormat.Gray16,
        // GL_LUMINANCE16
        0x8042 => PixelFormat.Gray16,
        // GL_RG16 / GL_RG16_SNORM
        0x822C or 0x8F99 => PixelFormat.Rg32,
        // GL_RGB16 / GL_RGB16_SNORM
        0x8054 or 0x8F9A => PixelFormat.Rgb48,
        // GL_RGBA16 / GL_RGBA16_SNORM
        0x805B or 0x8F9B => PixelFormat.Rgba64,
        // GL_R32F - 32-bit single-channel float
        0x822E => PixelFormat.Gray32Float,
        // GL_RG32F - 32-bit RG float pair
        0x8230 => PixelFormat.Rg64Float,
        // GL_RGB32F - 96-bit RGB float
        0x8815 => PixelFormat.Rgb96Float,
        // GL_RGBA32F - 128-bit RGBA float
        0x8814 => PixelFormat.Rgba128Float,
        // GL_R16F - 16-bit single-channel half-float
        0x822D => PixelFormat.Gray16Float,
        // GL_RG16F - 16-bit two-channel half-float
        0x822F => PixelFormat.Rg32Float,
        // GL_RGB16F - 16-bit three-channel half-float
        0x881B => PixelFormat.Rgb48Float,
        // GL_RGBA16F - 16-bit four-channel half-float
        0x881A => PixelFormat.Rgba64Float,
        // GL_R32UI / GL_R32I - 32-bit integer single-channel
        0x8236 => PixelFormat.Gray32UInt,
        0x8235 => PixelFormat.Gray32SInt,
        // GL_RG32UI / GL_RG32I - 32-bit integer two-channel
        0x823C => PixelFormat.Rg64UInt,
        0x823B => PixelFormat.Rg64SInt,
        // GL_RGB32UI / GL_RGB32I - 32-bit integer three-channel
        0x8D71 => PixelFormat.Rgb96UInt,
        0x8D83 => PixelFormat.Rgb96SInt,
        // GL_RGBA32UI / GL_RGBA32I - 32-bit integer four-channel
        0x8D70 => PixelFormat.Rgba128UInt,
        0x8D82 => PixelFormat.Rgba128SInt,
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
        // VK_FORMAT_R8G8_UNORM / SNORM / SRGB -> two-channel 8-bit pair as Gray+Alpha (no R8G8 PixelFormat).
        16 or 17 or 22 => PixelFormat.GrayAlpha16,
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
        // VK_FORMAT_R16G16B16_UNORM / SNORM
        84 or 85 => PixelFormat.Rgb48,
        // VK_FORMAT_R16G16B16A16_UNORM / SNORM
        91 or 92 => PixelFormat.Rgba64,
        // VK_FORMAT_R32_SFLOAT - 32-bit single-channel float
        100 => PixelFormat.Gray32Float,
        // VK_FORMAT_R32G32_SFLOAT - 32-bit RG float pair
        103 => PixelFormat.Rg64Float,
        // VK_FORMAT_R32G32B32_SFLOAT - 96-bit RGB float
        106 => PixelFormat.Rgb96Float,
        // VK_FORMAT_R32G32B32A32_SFLOAT - 128-bit RGBA float
        109 => PixelFormat.Rgba128Float,
        // VK_FORMAT_R16_SFLOAT - 16-bit single-channel half-float
        76 => PixelFormat.Gray16Float,
        // VK_FORMAT_R16G16_SFLOAT - 16-bit two-channel half-float
        83 => PixelFormat.Rg32Float,
        // VK_FORMAT_R16G16B16_SFLOAT - 16-bit three-channel half-float
        90 => PixelFormat.Rgb48Float,
        // VK_FORMAT_R16G16B16A16_SFLOAT - 16-bit four-channel half-float
        97 => PixelFormat.Rgba64Float,
        // VK_FORMAT_R32_UINT / R32_SINT - 32-bit integer single-channel
        98 => PixelFormat.Gray32UInt,
        99 => PixelFormat.Gray32SInt,
        // VK_FORMAT_R32G32_UINT / R32G32_SINT - 32-bit integer two-channel
        101 => PixelFormat.Rg64UInt,
        102 => PixelFormat.Rg64SInt,
        // VK_FORMAT_R32G32B32_UINT / R32G32B32_SINT - 32-bit integer three-channel
        104 => PixelFormat.Rgb96UInt,
        105 => PixelFormat.Rgb96SInt,
        // VK_FORMAT_R32G32B32A32_UINT / R32G32B32A32_SINT - 32-bit integer four-channel
        107 => PixelFormat.Rgba128UInt,
        108 => PixelFormat.Rgba128SInt,
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

    /// <summary>
    /// True when an OpenGL <c>glInternalFormat</c> value is one of the
    /// well-known sRGB-encoded tokens (covers the uncompressed sRGB targets
    /// plus the sRGB variants of every BCn / ETC2 compressed enum).
    /// </summary>
    public static bool IsSrgbGlInternalFormat(uint glInternalFormat) => glInternalFormat switch
    {
        // GL_SRGB8 / GL_SRGB8_ALPHA8
        0x8C41 or 0x8C43 => true,
        // GL_COMPRESSED_SRGB_S3TC_DXT1_EXT / GL_COMPRESSED_SRGB_ALPHA_S3TC_DXT1_EXT
        0x8C4C or 0x8C4D => true,
        // GL_COMPRESSED_SRGB_ALPHA_S3TC_DXT3_EXT
        0x8C4E => true,
        // GL_COMPRESSED_SRGB_ALPHA_S3TC_DXT5_EXT
        0x8C4F => true,
        // GL_COMPRESSED_SRGB_ALPHA_BPTC_UNORM
        0x8E8D => true,
        // GL_COMPRESSED_SRGB8_ETC2 / SRGB8_PUNCHTHROUGH_ALPHA1_ETC2 /
        // SRGB8_ALPHA8_ETC2_EAC
        0x9275 or 0x9277 or 0x9279 => true,
        _ => false,
    };

    /// <summary>
    /// True when a Vulkan <c>VkFormat</c> enum value is one of the well-known
    /// <c>_SRGB</c>-suffixed variants (uncompressed sRGB targets plus the
    /// sRGB variants of every BCn / ETC2 compressed VkFormat).
    /// </summary>
    public static bool IsSrgbVkFormat(uint vkFormat) => vkFormat switch
    {
        // Uncompressed sRGB UNORM siblings
        // VK_FORMAT_R8_SRGB / R8G8_SRGB / R8G8B8_SRGB / B8G8R8_SRGB /
        // R8G8B8A8_SRGB / B8G8R8A8_SRGB / A8B8G8R8_SRGB_PACK32
        15 or 22 or 29 or 36 or 43 or 50 or 57 => true,
        // VK_FORMAT_BC1_RGB_SRGB_BLOCK / BC1_RGBA_SRGB / BC2_SRGB / BC3_SRGB
        132 or 134 or 136 or 138 => true,
        // VK_FORMAT_BC7_SRGB_BLOCK
        146 => true,
        // VK_FORMAT_ETC2_R8G8B8_SRGB / R8G8B8A1_SRGB / R8G8B8A8_SRGB
        148 or 150 or 152 => true,
        _ => false,
    };
}
