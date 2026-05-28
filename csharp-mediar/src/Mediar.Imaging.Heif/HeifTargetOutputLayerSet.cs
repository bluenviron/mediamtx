using System.Buffers.Binary;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over a HEIF <c>tols</c> (Target Output Layer Set)
/// item property as defined by ISO/IEC 14496-15 (L-HEVC carriage
/// in ISO base media file format). The property identifies which
/// output layer set is to be rendered for a layered HEVC item and
/// always appears in conjunction with an <c>oinf</c> property.
/// </summary>
public sealed record HeifTargetOutputLayerSet
{
    /// <summary>Zero-based index into the operating points list
    /// carried by the companion <c>oinf</c> property. Maps to the
    /// <c>output_layer_set_idx</c> field of the chosen operating
    /// point.</summary>
    public required ushort TargetOlsIndex { get; init; }

    /// <summary>Parses a raw <c>tols</c> payload. Expects a 4-byte
    /// FullBox header (version must be 0) followed by a single
    /// 16-bit big-endian <c>target_ols</c> field, for 6 bytes total.
    /// Returns false on any layout violation.</summary>
    public static bool TryParse(ReadOnlySpan<byte> payload, out HeifTargetOutputLayerSet? result)
    {
        result = null;
        if (payload.Length < 6) return false;
        if (payload[0] != 0) return false; // FullBox version must be 0

        ushort idx = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(4, 2));
        result = new HeifTargetOutputLayerSet { TargetOlsIndex = idx };
        return true;
    }
}
