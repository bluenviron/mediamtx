using System.Buffers.Binary;
using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Scans box headers in an <see cref="IRandomAccessSource"/> without loading payloads.
/// </summary>
internal static class BoxScanner
{
    private const int MinHeader = 8;
    private const int LargeHeader = 16;

    /// <summary>
    /// Try to read a single box header at <paramref name="offset"/>.
    /// Returns false at EOF.
    /// </summary>
    public static bool TryReadHeader(IRandomAccessSource source, long offset, long endLimit, out BoxHeader header)
    {
        header = default;
        if (offset + MinHeader > endLimit) return false;

        Span<byte> small = stackalloc byte[LargeHeader];
        int n = source.Read(offset, small[..MinHeader]);
        if (n < MinHeader) return false;

        uint size = BinaryPrimitives.ReadUInt32BigEndian(small[..4]);
        uint type = BinaryPrimitives.ReadUInt32BigEndian(small.Slice(4, 4));

        long payloadOffset;
        long payloadLength;

        if (size == 1)
        {
            // 64-bit large size
            if (offset + LargeHeader > endLimit) return false;
            int n2 = source.Read(offset + MinHeader, small.Slice(MinHeader, 8));
            if (n2 < 8) return false;
            ulong large = BinaryPrimitives.ReadUInt64BigEndian(small.Slice(MinHeader, 8));
            if (large < (ulong)LargeHeader) return false;
            payloadOffset = offset + LargeHeader;
            payloadLength = (long)large - LargeHeader;
        }
        else if (size == 0)
        {
            // Runs to end of source
            payloadOffset = offset + MinHeader;
            payloadLength = endLimit - payloadOffset;
        }
        else
        {
            if (size < MinHeader) return false;
            payloadOffset = offset + MinHeader;
            payloadLength = size - MinHeader;
        }

        if (payloadOffset + payloadLength > endLimit) return false;
        header = new BoxHeader(new FourCc(type), offset, payloadOffset, payloadLength);
        return true;
    }

    /// <summary>
    /// Enumerate top-level (or nested-region) box headers in <paramref name="source"/>
    /// from <paramref name="startOffset"/> up to (and not including) <paramref name="endLimit"/>.
    /// </summary>
    public static IEnumerable<BoxHeader> Scan(IRandomAccessSource source, long startOffset, long endLimit)
    {
        long pos = startOffset;
        while (pos < endLimit)
        {
            if (!TryReadHeader(source, pos, endLimit, out var hdr))
            {
                yield break;
            }
            yield return hdr;
            pos = hdr.EndOffset;
        }
    }
}
