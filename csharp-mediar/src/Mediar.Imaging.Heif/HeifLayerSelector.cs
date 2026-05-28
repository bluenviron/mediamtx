using System.Buffers.Binary;

namespace Mediar.Imaging.Heif;

/// <summary>
/// HEIF Layer Selector property (<c>lsel</c>) per ISO/IEC 23008-12.
/// Selects a single layer from a multi-layer image item (typically
/// L-HEVC) for rendering. Encoded as two bytes carrying the
/// <c>layer_id</c>.
/// </summary>
public sealed record HeifLayerSelector
{
    /// <summary>Layer identifier into the underlying multi-layer
    /// bitstream. A value of 0xFFFF (the maximum) selects the layer
    /// targeted by the embedded operating-point information.</summary>
    public required ushort LayerId { get; init; }

    /// <summary>Decodes a raw <c>lsel</c> payload (2 bytes, big-endian
    /// layer_id).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifLayerSelector selector)
    {
        selector = null!;
        if (data.Length < 2) return false;
        selector = new HeifLayerSelector
        {
            LayerId = BinaryPrimitives.ReadUInt16BigEndian(data[..2]),
        };
        return true;
    }
}
