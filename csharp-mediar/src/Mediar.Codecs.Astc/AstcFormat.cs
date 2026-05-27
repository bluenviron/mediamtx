namespace Mediar.Codecs.Astc;

/// <summary>
/// Identifies an ASTC (Adaptive Scalable Texture Compression) block layout
/// per Khronos KDF 1.4 section 19. All ASTC blocks are 128 bits (16 bytes);
/// the variant identifies the per-block pixel footprint.
/// </summary>
public enum AstcFormat
{
    /// <summary>Not a recognised ASTC format.</summary>
    None,

    /// <summary>4x4 LDR.</summary>
    Astc4x4Unorm,
    /// <summary>4x4 sRGB.</summary>
    Astc4x4Srgb,
    /// <summary>5x4 LDR.</summary>
    Astc5x4Unorm,
    /// <summary>5x4 sRGB.</summary>
    Astc5x4Srgb,
    /// <summary>5x5 LDR.</summary>
    Astc5x5Unorm,
    /// <summary>5x5 sRGB.</summary>
    Astc5x5Srgb,
    /// <summary>6x5 LDR.</summary>
    Astc6x5Unorm,
    /// <summary>6x5 sRGB.</summary>
    Astc6x5Srgb,
    /// <summary>6x6 LDR.</summary>
    Astc6x6Unorm,
    /// <summary>6x6 sRGB.</summary>
    Astc6x6Srgb,
    /// <summary>8x5 LDR.</summary>
    Astc8x5Unorm,
    /// <summary>8x5 sRGB.</summary>
    Astc8x5Srgb,
    /// <summary>8x6 LDR.</summary>
    Astc8x6Unorm,
    /// <summary>8x6 sRGB.</summary>
    Astc8x6Srgb,
    /// <summary>8x8 LDR.</summary>
    Astc8x8Unorm,
    /// <summary>8x8 sRGB.</summary>
    Astc8x8Srgb,
    /// <summary>10x5 LDR.</summary>
    Astc10x5Unorm,
    /// <summary>10x5 sRGB.</summary>
    Astc10x5Srgb,
    /// <summary>10x6 LDR.</summary>
    Astc10x6Unorm,
    /// <summary>10x6 sRGB.</summary>
    Astc10x6Srgb,
    /// <summary>10x8 LDR.</summary>
    Astc10x8Unorm,
    /// <summary>10x8 sRGB.</summary>
    Astc10x8Srgb,
    /// <summary>10x10 LDR.</summary>
    Astc10x10Unorm,
    /// <summary>10x10 sRGB.</summary>
    Astc10x10Srgb,
    /// <summary>12x10 LDR.</summary>
    Astc12x10Unorm,
    /// <summary>12x10 sRGB.</summary>
    Astc12x10Srgb,
    /// <summary>12x12 LDR.</summary>
    Astc12x12Unorm,
    /// <summary>12x12 sRGB.</summary>
    Astc12x12Srgb,
}
