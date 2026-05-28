using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// HEIF JPEG Codec Configuration property (<c>jpgC</c>) per ISO/IEC
/// 23008-12. Carries the shared JPEG prefix (typically SOI + DQT
/// quantization tables + DHT Huffman tables + SOF / DRI markers) that
/// must be prepended to the per-item JPEG entropy-coded segment to
/// form a complete, decodable JPEG bitstream. By extracting the
/// shared prefix into a single property the storage cost of many
/// JPEG-encoded items in one HEIF container is minimized.
/// </summary>
public sealed record HeifJpegConfiguration
{
    /// <summary>The raw JPEG prefix bytes (no encapsulation). Typical
    /// content begins with the SOI marker (0xFF 0xD8) and ends just
    /// before the SOS entropy-coded segment that lives in the item
    /// payload.</summary>
    public required ImmutableArray<byte> JpegPrefix { get; init; }

    /// <summary>Concatenates <see cref="JpegPrefix"/> with the
    /// supplied item entropy-coded segment to produce a complete
    /// JPEG bitstream ready to hand to a JPEG decoder.</summary>
    public byte[] BuildJpegBitstream(ReadOnlySpan<byte> itemPayload)
    {
        var result = new byte[JpegPrefix.Length + itemPayload.Length];
        JpegPrefix.AsSpan().CopyTo(result.AsSpan(0, JpegPrefix.Length));
        itemPayload.CopyTo(result.AsSpan(JpegPrefix.Length));
        return result;
    }

    /// <summary>Decodes a raw <c>jpgC</c> payload. The entire payload
    /// is the JPEG prefix; there is no FullBox header per spec.</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifJpegConfiguration config)
    {
        config = null!;
        if (data.Length < 1) return false;
        config = new HeifJpegConfiguration { JpegPrefix = ImmutableArray.Create(data) };
        return true;
    }
}
