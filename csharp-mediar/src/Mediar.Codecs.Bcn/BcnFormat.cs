namespace Mediar.Codecs.Bcn;

/// <summary>
/// Identifies a BCn-class block-compressed surface and is used as the
/// runtime dispatch tag for <see cref="BcnDecoder"/>, <see cref="Bc6hDecoder"/>
/// and <see cref="Bc7Decoder"/>.
/// </summary>
public enum BcnFormat
{
    /// <summary>Not a recognised BCn format.</summary>
    None,

    /// <summary>BC1 / DXT1 — 8 byte/block, 5:6:5 + optional 1-bit alpha.</summary>
    Bc1,

    /// <summary>BC2 / DXT3 — 16 byte/block, 4-bit explicit alpha + BC1 colour.</summary>
    Bc2,

    /// <summary>BC3 / DXT5 — 16 byte/block, interpolated 3-bit alpha + BC1 colour.</summary>
    Bc3,

    /// <summary>BC4 / RGTC1 — 8 byte/block, single-channel red.</summary>
    Bc4,

    /// <summary>BC5 / RGTC2 — 16 byte/block, red + green.</summary>
    Bc5,

    /// <summary>BC6H UF16 — 16 byte/block, HDR unsigned half-float.</summary>
    Bc6hUf16,

    /// <summary>BC6H SF16 — 16 byte/block, HDR signed half-float.</summary>
    Bc6hSf16,

    /// <summary>BC7 — 16 byte/block, advanced adaptive (BPTC unorm).</summary>
    Bc7,
}
