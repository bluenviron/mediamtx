namespace Mediar.Codecs.Lzw;

/// <summary>
/// Bit packing order used by an LZW byte stream.
/// </summary>
/// <remarks>
/// LZW codes are variable-width and do not align on byte boundaries, so
/// the container chooses how successive codes are packed inside each
/// source byte. GIF packs the first code in the low bits of the first
/// byte (<see cref="LsbFirst"/>); TIFF 6.0 packs it in the high bits
/// (<see cref="MsbFirst"/>).
/// </remarks>
public enum LzwBitOrder
{
    /// <summary>
    /// First code occupies the least-significant bits of the first byte;
    /// successive codes shift into the next bit positions upward. Used
    /// by GIF87a / GIF89a and (with different parameters) by the
    /// Postscript LZWDecode filter in Adobe documents.
    /// </summary>
    LsbFirst,

    /// <summary>
    /// First code occupies the most-significant bits of the first byte
    /// and the bit buffer is filled high-to-low. Used by TIFF 6.0 and
    /// PDF <c>/LZWDecode</c> when <c>EarlyChange</c> is in effect.
    /// </summary>
    MsbFirst,
}

/// <summary>
/// Parameters that configure the variable-width LZW algorithm for a
/// specific container dialect.
/// </summary>
/// <param name="BitOrder">Bit packing order — see <see cref="LzwBitOrder"/>.</param>
/// <param name="InitialBits">
/// Width, in bits, of the literal alphabet — i.e. how many input symbols
/// are pre-loaded into the dictionary. GIF passes the byte stored in the
/// "LZW Minimum Code Size" frame header (typically 2..8); TIFF always
/// uses 8. The first code emitted is one bit wider than this value.
/// </param>
/// <param name="MaxBits">
/// Highest code width before the dictionary stops growing. The canonical
/// value is 12 for both GIF and TIFF; values up to 16 are also supported.
/// </param>
/// <param name="EarlyChange">
/// When <see langword="true"/>, the code width grows one entry before it
/// strictly needs to (TIFF 6.0 § Section 13 "early change" semantics).
/// GIF requires <see langword="false"/>.
/// </param>
public readonly record struct LzwOptions(
    LzwBitOrder BitOrder,
    int InitialBits,
    int MaxBits = 12,
    bool EarlyChange = false)
{
    /// <summary>Canonical GIF 87a / 89a LZW options.</summary>
    public static LzwOptions Gif(int lzwMinCodeSize) =>
        new(LzwBitOrder.LsbFirst, lzwMinCodeSize, MaxBits: 12, EarlyChange: false);

    /// <summary>Canonical TIFF 6.0 / PDF <c>/LZWDecode</c> options.</summary>
    public static LzwOptions Tiff() =>
        new(LzwBitOrder.MsbFirst, 8, MaxBits: 12, EarlyChange: true);
}
