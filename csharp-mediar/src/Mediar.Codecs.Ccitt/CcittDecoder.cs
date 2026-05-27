namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Facade selecting between the T.4 (Group 3, 1D Modified Huffman) and
/// T.6 (Group 4, MMR) decoders based on the TIFF Compression tag.
/// </summary>
/// <remarks>
/// Supported TIFF compression codes:
/// <list type="bullet">
///   <item><term>2</term><description>CCITT Modified Huffman (T.4 1D without EOL).</description></item>
///   <item><term>3</term><description>CCITT T.4 Group 3 (1D, requires EOL markers; T4Options bit 0 selects 2D, bit 2 selects EOL byte-alignment).</description></item>
///   <item><term>4</term><description>CCITT T.6 Group 4 (MMR).</description></item>
/// </list>
/// T.4 2D (T4Options bit 0) and uncompressed-mode extensions are not yet
/// implemented and throw <see cref="NotSupportedException"/>.
/// </remarks>
public static class CcittDecoder
{
    /// <summary>
    /// Decode CCITT fax data tagged with a TIFF compression code.
    /// </summary>
    /// <param name="encoded">Encoded fax bytes (after any FillOrder bit-reversal).</param>
    /// <param name="width">Pixel width.</param>
    /// <param name="height">Pixel height.</param>
    /// <param name="tiffCompression">TIFF compression tag value (2, 3 or 4).</param>
    /// <param name="t4Options">TIFF tag 0x0124 (T4Options); zero for compression 2 or 4.</param>
    public static byte[] Decode(ReadOnlyMemory<byte> encoded, int width, int height,
                                 int tiffCompression, uint t4Options = 0)
    {
        return tiffCompression switch
        {
            2 => CcittG3Decoder.Decode(encoded, width, height,
                    new CcittG3Decoder.Options(HasEolMarkers: false, EolByteAligned: false)),
            3 => DecodeT4(encoded, width, height, t4Options),
            4 => CcittG4Decoder.Decode(encoded, width, height),
            _ => throw new NotSupportedException($"CCITT compression {tiffCompression} not supported."),
        };
    }

    private static byte[] DecodeT4(ReadOnlyMemory<byte> encoded, int width, int height, uint t4Options)
    {
        bool twoDimensional = (t4Options & 0x1) != 0;
        bool eolByteAligned = (t4Options & 0x4) != 0;
        if (twoDimensional)
        {
            throw new NotSupportedException(
                "CCITT T.4 two-dimensional coding (T4Options bit 0) is not yet implemented.");
        }
        return CcittG3Decoder.Decode(encoded, width, height,
            new CcittG3Decoder.Options(HasEolMarkers: true, EolByteAligned: eolByteAligned));
    }
}
