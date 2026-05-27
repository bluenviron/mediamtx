namespace Mediar.Imaging.Pvr;

/// <summary>
/// PVR v3 "pre-known" pixel-format identifier values that occupy the low
/// 32 bits of the 64-bit pixel-format field when the high 32 bits are zero.
/// Codes are defined by Imagination's PowerVR SDK PVRTextureUtilities header.
/// </summary>
public enum PvrFormatId : ulong
{
    /// <summary>Unrecognised / not a pre-known format code.</summary>
    None = 0xFFFFFFFFFFFFFFFFUL,

    Pvrtc2BppRgb = 0,
    Pvrtc2BppRgba = 1,
    Pvrtc4BppRgb = 2,
    Pvrtc4BppRgba = 3,
    Pvrtc2_2BppRgba = 4,
    Pvrtc2_4BppRgba = 5,
    Etc1 = 6,
    Dxt1 = 7,
    Dxt2 = 8,
    Dxt3 = 9,
    Dxt4 = 10,
    Dxt5 = 11,
    Bc1 = 12,
    Bc2 = 13,
    Bc3 = 14,
    Bc4 = 15,
    Bc5 = 16,
    Bc6 = 17,
    Bc7 = 18,
    Uyvy = 19,
    Yuy2 = 20,
    Bw1Bpp = 21,
    R9g9b9e5SharedExponent = 22,
    Rgbg8888 = 23,
    Grgb8888 = 24,
    Etc2Rgb = 25,
    Etc2Rgba = 26,
    Etc2RgbA1 = 27,
    EacR11 = 28,
    EacRg11 = 29,
    Astc4x4 = 30,
    Astc5x4 = 31,
    Astc5x5 = 32,
    Astc6x5 = 33,
    Astc6x6 = 34,
    Astc8x5 = 35,
    Astc8x6 = 36,
    Astc8x8 = 37,
    Astc10x5 = 38,
    Astc10x6 = 39,
    Astc10x8 = 40,
    Astc10x10 = 41,
    Astc12x10 = 42,
    Astc12x12 = 43,
}
