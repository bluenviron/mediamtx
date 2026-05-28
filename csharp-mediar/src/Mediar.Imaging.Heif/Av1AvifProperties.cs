using System.Buffers.Binary;
using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// AVIF AV1 Operating Point Selector property (<c>a1op</c>) per the
/// AVIF specification. Selects which AV1 operating point inside the
/// associated AV1 sequence header is to be decoded for the item.
/// Encoded as a single byte holding the operating-point index.
/// </summary>
public sealed record HeifAv1OperatingPoint
{
    /// <summary>Index into the AV1 sequence header's operating_points
    /// array. Value 0 selects the base operating point and is the only
    /// value defined when the underlying AV1 sequence header advertises
    /// a single operating point.</summary>
    public required byte OpIndex { get; init; }

    /// <summary>Decodes a raw <c>a1op</c> payload (1 byte op_index).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifAv1OperatingPoint op)
    {
        op = null!;
        if (data.Length < 1) return false;
        op = new HeifAv1OperatingPoint { OpIndex = data[0] };
        return true;
    }
}

/// <summary>
/// AVIF AV1 Layered Image Indexing property (<c>a1lx</c>) per the AVIF
/// specification. Provides the sizes of the first three spatial layers
/// in a layered AV1 image so a decoder can skip directly to the
/// desired layer without parsing the AV1 bitstream end-to-end.
/// </summary>
/// <remarks>
/// Layout: <c>reserved(7) | large_size(1)</c> followed by three layer
/// sizes encoded as <c>uint16</c> when <c>large_size = 0</c> or
/// <c>uint32</c> when <c>large_size = 1</c>. A layer size of zero
/// indicates the layer is not present in the item data.
/// </remarks>
public sealed record HeifAv1LayeredImageIndexing
{
    /// <summary>True when each layer size is encoded as a 32-bit
    /// unsigned integer; false when sizes are 16-bit.</summary>
    public required bool LargeSize { get; init; }

    /// <summary>The three per-layer payload sizes (in bytes) covering
    /// the first three spatial layers. Trailing layers beyond what
    /// this property can describe are inferred from the remaining
    /// item-data tail.</summary>
    public required ImmutableArray<uint> LayerSizes { get; init; }

    /// <summary>Decodes a raw <c>a1lx</c> payload (1 byte large-size
    /// flag + three uint16 or uint32 layer sizes).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifAv1LayeredImageIndexing rec)
    {
        rec = null!;
        if (data.Length < 1) return false;
        bool large = (data[0] & 1) == 1;
        int sizeWidth = large ? 4 : 2;
        int needed = 1 + 3 * sizeWidth;
        if (data.Length < needed) return false;
        var builder = ImmutableArray.CreateBuilder<uint>(3);
        for (int i = 0; i < 3; i++)
        {
            int off = 1 + i * sizeWidth;
            uint sz = large
                ? BinaryPrimitives.ReadUInt32BigEndian(data.Slice(off, 4))
                : BinaryPrimitives.ReadUInt16BigEndian(data.Slice(off, 2));
            builder.Add(sz);
        }
        rec = new HeifAv1LayeredImageIndexing
        {
            LargeSize = large,
            LayerSizes = builder.ToImmutable(),
        };
        return true;
    }
}
