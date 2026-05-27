namespace Mediar.Codecs.Etc;

/// <summary>
/// Identifies an ETC / EAC class block-compressed surface and is used as
/// the runtime dispatch tag for <see cref="EtcDecoder"/>.
/// </summary>
/// <remarks>
/// All block formats are 4×4 pixels. Block sizes are:
/// <list type="bullet">
///   <item><see cref="Etc1Rgb"/> / <see cref="Etc2Rgb"/> / <see cref="Etc2RgbA1"/> / <see cref="EacR11Unorm"/> / <see cref="EacR11Snorm"/>: 8 bytes.</item>
///   <item><see cref="Etc2Rgba8"/> / <see cref="EacRg11Unorm"/> / <see cref="EacRg11Snorm"/>: 16 bytes.</item>
/// </list>
/// </remarks>
public enum EtcFormat
{
    /// <summary>Not a recognised ETC / EAC format.</summary>
    None,

    /// <summary>ETC1 — 8 byte/block, RGB only, individual or differential mode.</summary>
    Etc1Rgb,

    /// <summary>ETC2 RGB — 8 byte/block, ETC1 + T / H / Planar modes.</summary>
    Etc2Rgb,

    /// <summary>ETC2 RGB with 1-bit punch-through alpha — 8 byte/block.</summary>
    Etc2RgbA1,

    /// <summary>ETC2 RGBA8 — 16 byte/block, EAC 8-bit alpha + ETC2 RGB.</summary>
    Etc2Rgba8,

    /// <summary>EAC R11 unorm — 8 byte/block, single-channel red.</summary>
    EacR11Unorm,

    /// <summary>EAC R11 snorm — 8 byte/block, single-channel signed red.</summary>
    EacR11Snorm,

    /// <summary>EAC RG11 unorm — 16 byte/block, red + green.</summary>
    EacRg11Unorm,

    /// <summary>EAC RG11 snorm — 16 byte/block, signed red + green.</summary>
    EacRg11Snorm,
}
