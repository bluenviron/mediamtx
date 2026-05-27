namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Baseline-DCT JPEG decoder (SOF0). This class exposes the entry-point
/// the reader uses; the current implementation is a structural stub that
/// throws <see cref="NotImplementedException"/> – callers should rely on
/// <see cref="JpegReader.Info"/> and <see cref="JpegReader.Metadata"/>
/// for now. A full Huffman + IDCT + YCbCr→RGB pipeline is planned.
/// </summary>
internal static class JpegBaselineDecoder
{
    public static ImageFrame Decode(JpegFrame frame, byte[] scanData)
    {
        _ = frame; _ = scanData;
        throw new NotImplementedException(
            "JPEG baseline pixel decoding is intentionally not implemented in this Mediar release. " +
            "Use JpegReader to inspect dimensions, EXIF, GPS, and other metadata. " +
            "Pull-request welcome at https://github.com/mediar-net/mediar.");
    }
}
