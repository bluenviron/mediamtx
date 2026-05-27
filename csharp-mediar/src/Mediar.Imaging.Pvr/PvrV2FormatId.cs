namespace Mediar.Imaging.Pvr;

/// <summary>
/// Legacy PowerVR Texture v2 pixel-format codes. These occupy the low 8
/// bits of the v2 header's pixel-format-flags field; the upper 24 bits
/// carry flags such as mip-map present (0x100), cubemap (0x1000),
/// volume texture (0x4000), vertical-flip (0x10000), bump-map (0x400),
/// and premultiplied alpha (0x8000).
/// </summary>
/// <remarks>
/// Values sourced from the Imagination Technologies legacy PowerVR SDK
/// header <c>PVRTextureFormat.h</c> (PVR Texture Format v2).
/// </remarks>
public enum PvrV2FormatId : byte
{
    /// <summary>Default / sentinel value.</summary>
    None = 0xFF,

    /// <summary>5-6-5 packed 16-bit RGB.</summary>
    Argb4444 = 0x00,
    /// <summary>5-5-5-1 packed 16-bit RGBA.</summary>
    Argb1555 = 0x01,
    /// <summary>5-6-5 packed 16-bit RGB.</summary>
    Rgb565 = 0x02,
    /// <summary>3-3-2 8-bit RGB.</summary>
    Rgb555 = 0x03,
    /// <summary>8-8-8 24-bit BGR.</summary>
    Rgb888 = 0x04,
    /// <summary>8-8-8-8 32-bit ARGB.</summary>
    Argb8888 = 0x05,
    /// <summary>8-8-8-8 32-bit ARGB (legacy alias).</summary>
    Argb8332 = 0x06,
    /// <summary>1-channel 8-bit indexed.</summary>
    I8 = 0x07,
    /// <summary>1-channel 8-bit alpha + 8-bit intensity.</summary>
    Ai88 = 0x08,
    /// <summary>1-bit monochrome.</summary>
    Monochrome = 0x09,
    /// <summary>5-5-5-1 packed 16-bit BGRA.</summary>
    V_Y1_U_Y0 = 0x0A,
    /// <summary>YUV 4:2:2 alternate layout.</summary>
    Y1_V_Y0_U = 0x0B,
    /// <summary>PVRTC 2 bits-per-pixel (legacy mode).</summary>
    Pvrtc2 = 0x18,
    /// <summary>PVRTC 4 bits-per-pixel (legacy mode).</summary>
    Pvrtc4 = 0x19,

    // OpenGL ES extensions surfaced through PVR2:
    /// <summary>OpenGL ES RGBA 8888 little-endian.</summary>
    GlRgba8888 = 0x12,
    /// <summary>OpenGL ES RGBA 4444 little-endian.</summary>
    GlRgba4444 = 0x10,
    /// <summary>OpenGL ES RGBA 5551 little-endian.</summary>
    GlRgba5551 = 0x11,
    /// <summary>OpenGL ES RGB 565 little-endian.</summary>
    GlRgb565 = 0x13,
    /// <summary>OpenGL ES RGB 555 little-endian.</summary>
    GlRgb555 = 0x14,
    /// <summary>OpenGL ES RGB 888 (24-bit).</summary>
    GlRgb888 = 0x15,
    /// <summary>OpenGL ES luminance 8-bit.</summary>
    GlIntensity8 = 0x16,
    /// <summary>OpenGL ES luminance+alpha 8-8.</summary>
    GlAi88 = 0x17,
    /// <summary>OpenGL ES BGRA 8888.</summary>
    GlBgra8888 = 0x1A,
    /// <summary>OpenGL ES ETC1 RGB.</summary>
    GlEtc1 = 0x36,
    /// <summary>OpenGL ES DXT1 / BC1.</summary>
    Dxt1 = 0x20,
    /// <summary>OpenGL ES DXT2.</summary>
    Dxt2 = 0x21,
    /// <summary>OpenGL ES DXT3 / BC2.</summary>
    Dxt3 = 0x22,
    /// <summary>OpenGL ES DXT4.</summary>
    Dxt4 = 0x23,
    /// <summary>OpenGL ES DXT5 / BC3.</summary>
    Dxt5 = 0x24,
}
