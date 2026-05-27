using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;

namespace Mediar.Imaging.Pvr;

/// <summary>
/// PVR v3 format-identification helpers. Maps the
/// <see cref="PvrFormatId"/> table values to Mediar
/// <see cref="BcnFormat"/> / <see cref="EtcFormat"/> codes and well-known
/// uncompressed channel descriptors to <see cref="PixelFormat"/>.
/// </summary>
public static class PvrFormat
{
    /// <summary>
    /// Map a pre-known <see cref="PvrFormatId"/> code to a Mediar
    /// <see cref="BcnFormat"/>. Returns <see cref="BcnFormat.None"/> for
    /// non-BCn (PVRTC / ETC / ASTC / uncompressed) formats.
    /// </summary>
    public static BcnFormat MapBcn(PvrFormatId id) => id switch
    {
        PvrFormatId.Dxt1 or PvrFormatId.Bc1 => BcnFormat.Bc1,
        PvrFormatId.Dxt2 or PvrFormatId.Dxt3 or PvrFormatId.Bc2 => BcnFormat.Bc2,
        PvrFormatId.Dxt4 or PvrFormatId.Dxt5 or PvrFormatId.Bc3 => BcnFormat.Bc3,
        PvrFormatId.Bc4 => BcnFormat.Bc4,
        PvrFormatId.Bc5 => BcnFormat.Bc5,
        // BC6: PVR3 doesn't disambiguate signed vs unsigned at the
        // format-id level; default to the unsigned variant.
        PvrFormatId.Bc6 => BcnFormat.Bc6hUf16,
        PvrFormatId.Bc7 => BcnFormat.Bc7,
        _ => BcnFormat.None,
    };

    /// <summary>
    /// Map a pre-known <see cref="PvrFormatId"/> code to a Mediar
    /// <see cref="EtcFormat"/>. Returns <see cref="EtcFormat.None"/> for
    /// non-ETC formats.
    /// </summary>
    public static EtcFormat MapEtc(PvrFormatId id, uint channelType) => id switch
    {
        PvrFormatId.Etc1 => EtcFormat.Etc1Rgb,
        PvrFormatId.Etc2Rgb => EtcFormat.Etc2Rgb,
        PvrFormatId.Etc2Rgba => EtcFormat.Etc2Rgba8,
        PvrFormatId.Etc2RgbA1 => EtcFormat.Etc2RgbA1,
        PvrFormatId.EacR11 => channelType == 1 ? EtcFormat.EacR11Snorm : EtcFormat.EacR11Unorm,
        PvrFormatId.EacRg11 => channelType == 1 ? EtcFormat.EacRg11Snorm : EtcFormat.EacRg11Unorm,
        _ => EtcFormat.None,
    };

    /// <summary>
    /// Map a 64-bit PVR3 pixel-format value to an uncompressed
    /// <see cref="PixelFormat"/> when the high 32 bits encode a channel
    /// descriptor. Returns <see cref="PixelFormat.Unknown"/> for
    /// unrecognised layouts.
    /// </summary>
    /// <param name="pf">Full 64-bit pixel-format word.</param>
    public static PixelFormat MapUncompressed(ulong pf)
    {
        if ((pf >> 32) == 0) return PixelFormat.Unknown;

        // Low 4 bytes = channel names (e.g. 'r','g','b','a' / 'l','a','0','0');
        // high 4 bytes = per-channel bit widths.
        byte c0 = (byte)(pf & 0xFF);
        byte c1 = (byte)((pf >> 8) & 0xFF);
        byte c2 = (byte)((pf >> 16) & 0xFF);
        byte c3 = (byte)((pf >> 24) & 0xFF);
        byte b0 = (byte)((pf >> 32) & 0xFF);
        byte b1 = (byte)((pf >> 40) & 0xFF);
        byte b2 = (byte)((pf >> 48) & 0xFF);
        byte b3 = (byte)((pf >> 56) & 0xFF);

        // r,g,b,a 8,8,8,8
        if (c0 == 'r' && c1 == 'g' && c2 == 'b' && c3 == 'a' &&
            b0 == 8 && b1 == 8 && b2 == 8 && b3 == 8)
        {
            return PixelFormat.Rgba32;
        }
        // b,g,r,a 8,8,8,8
        if (c0 == 'b' && c1 == 'g' && c2 == 'r' && c3 == 'a' &&
            b0 == 8 && b1 == 8 && b2 == 8 && b3 == 8)
        {
            return PixelFormat.Bgra32;
        }
        // r,g,b,0 8,8,8,0
        if (c0 == 'r' && c1 == 'g' && c2 == 'b' && c3 == 0 &&
            b0 == 8 && b1 == 8 && b2 == 8 && b3 == 0)
        {
            return PixelFormat.Rgb24;
        }
        // b,g,r,0 8,8,8,0
        if (c0 == 'b' && c1 == 'g' && c2 == 'r' && c3 == 0 &&
            b0 == 8 && b1 == 8 && b2 == 8 && b3 == 0)
        {
            return PixelFormat.Bgr24;
        }
        // r,0,0,0 8,0,0,0
        if (c0 == 'r' && c1 == 0 && c2 == 0 && c3 == 0 &&
            b0 == 8 && b1 == 0 && b2 == 0 && b3 == 0)
        {
            return PixelFormat.Gray8;
        }
        // l,0,0,0 8,0,0,0 (luminance-only)
        if (c0 == 'l' && c1 == 0 && c2 == 0 && c3 == 0 &&
            b0 == 8 && b1 == 0 && b2 == 0 && b3 == 0)
        {
            return PixelFormat.Gray8;
        }
        return PixelFormat.Unknown;
    }

    /// <summary>
    /// Per-pixel bit count for an uncompressed <see cref="PixelFormat"/>
    /// produced by <see cref="MapUncompressed"/>. Returns 0 for
    /// non-supported formats.
    /// </summary>
    public static int UncompressedBytesPerPixel(PixelFormat pf) => pf switch
    {
        PixelFormat.Gray8 => 1,
        PixelFormat.Rgb24 or PixelFormat.Bgr24 => 3,
        PixelFormat.Rgba32 or PixelFormat.Bgra32 => 4,
        _ => 0,
    };

    /// <summary>
    /// Block dimension (width / height in pixels) for a PVR-recognised
    /// compressed format. Returns 4 for the BCn / ETC families, the ASTC
    /// block dimension for ASTC, 8x4 / 4x4 for PVRTC. Used to align
    /// payload-byte calculations to whole blocks.
    /// </summary>
    public static (int Width, int Height) BlockDimensions(PvrFormatId id) => id switch
    {
        PvrFormatId.Pvrtc2BppRgb or PvrFormatId.Pvrtc2BppRgba or PvrFormatId.Pvrtc2_2BppRgba => (8, 4),
        PvrFormatId.Pvrtc4BppRgb or PvrFormatId.Pvrtc4BppRgba or PvrFormatId.Pvrtc2_4BppRgba => (4, 4),
        PvrFormatId.Etc1 or PvrFormatId.Etc2Rgb or PvrFormatId.Etc2Rgba or PvrFormatId.Etc2RgbA1
            or PvrFormatId.EacR11 or PvrFormatId.EacRg11 => (4, 4),
        PvrFormatId.Dxt1 or PvrFormatId.Dxt2 or PvrFormatId.Dxt3 or PvrFormatId.Dxt4 or PvrFormatId.Dxt5
            or PvrFormatId.Bc1 or PvrFormatId.Bc2 or PvrFormatId.Bc3 or PvrFormatId.Bc4
            or PvrFormatId.Bc5 or PvrFormatId.Bc6 or PvrFormatId.Bc7 => (4, 4),
        PvrFormatId.Astc4x4 => (4, 4),
        PvrFormatId.Astc5x4 => (5, 4),
        PvrFormatId.Astc5x5 => (5, 5),
        PvrFormatId.Astc6x5 => (6, 5),
        PvrFormatId.Astc6x6 => (6, 6),
        PvrFormatId.Astc8x5 => (8, 5),
        PvrFormatId.Astc8x6 => (8, 6),
        PvrFormatId.Astc8x8 => (8, 8),
        PvrFormatId.Astc10x5 => (10, 5),
        PvrFormatId.Astc10x6 => (10, 6),
        PvrFormatId.Astc10x8 => (10, 8),
        PvrFormatId.Astc10x10 => (10, 10),
        PvrFormatId.Astc12x10 => (12, 10),
        PvrFormatId.Astc12x12 => (12, 12),
        _ => (1, 1),
    };

    /// <summary>
    /// Bits per block for a PVR-recognised compressed format. ASTC is
    /// always 128 bits per block (16 bytes); BC1/BC4/ETC1/ETC2-RGB/EAC-R11
    /// are 64 bits per block (8 bytes); BC2/BC3/BC5/BC6H/BC7/ETC2-RGBA/
    /// ETC2-RGB+A1/EAC-RG11 are 128 bits per block (16 bytes). PVRTC
    /// 2-bpp = 64-bit blocks of 8x4 pixels; PVRTC 4-bpp = 64-bit blocks
    /// of 4x4 pixels.
    /// </summary>
    public static int BitsPerBlock(PvrFormatId id) => id switch
    {
        PvrFormatId.Pvrtc2BppRgb or PvrFormatId.Pvrtc2BppRgba or PvrFormatId.Pvrtc2_2BppRgba
            or PvrFormatId.Pvrtc4BppRgb or PvrFormatId.Pvrtc4BppRgba or PvrFormatId.Pvrtc2_4BppRgba => 64,
        PvrFormatId.Etc1 or PvrFormatId.Etc2Rgb or PvrFormatId.EacR11 => 64,
        PvrFormatId.Etc2Rgba or PvrFormatId.Etc2RgbA1 or PvrFormatId.EacRg11 => 128,
        PvrFormatId.Dxt1 or PvrFormatId.Bc1 or PvrFormatId.Bc4 => 64,
        PvrFormatId.Dxt2 or PvrFormatId.Dxt3 or PvrFormatId.Dxt4 or PvrFormatId.Dxt5
            or PvrFormatId.Bc2 or PvrFormatId.Bc3 or PvrFormatId.Bc5 or PvrFormatId.Bc6 or PvrFormatId.Bc7 => 128,
        PvrFormatId.Astc4x4 or PvrFormatId.Astc5x4 or PvrFormatId.Astc5x5 or PvrFormatId.Astc6x5
            or PvrFormatId.Astc6x6 or PvrFormatId.Astc8x5 or PvrFormatId.Astc8x6 or PvrFormatId.Astc8x8
            or PvrFormatId.Astc10x5 or PvrFormatId.Astc10x6 or PvrFormatId.Astc10x8
            or PvrFormatId.Astc10x10 or PvrFormatId.Astc12x10 or PvrFormatId.Astc12x12 => 128,
        _ => 0,
    };
}
