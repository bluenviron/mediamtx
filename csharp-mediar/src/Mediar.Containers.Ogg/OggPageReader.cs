using System.Buffers;
using System.Buffers.Binary;
using Mediar.IO;

namespace Mediar.Containers.Ogg;

/// <summary>
/// Ogg page reader. Reads sequential pages from an arbitrary input stream and
/// returns each page's header plus a payload byte-range (filled into a
/// caller-provided buffer). The reader does not validate the CRC; integrity
/// checks are the consumer's responsibility.
/// </summary>
public sealed class OggPageReader
{
    private readonly IRandomAccessSource _source;
    private long _position;

    /// <summary>Create a reader positioned at the start of the stream.</summary>
    public OggPageReader(IRandomAccessSource source)
    {
        ArgumentNullException.ThrowIfNull(source);
        _source = source;
    }

    /// <summary>Current byte position in the underlying source.</summary>
    public long Position => _position;

    /// <summary>Length of the underlying source.</summary>
    public long Length => _source.Length;

    /// <summary>
    /// Read the next Ogg page. Returns false at end-of-stream. The payload is
    /// written into a rented buffer; the caller owns and must dispose the returned owner.
    /// </summary>
    public bool TryReadPage(out OggPageHeader header, out IMemoryOwner<byte>? payloadOwner, out int payloadLength)
    {
        header = default;
        payloadOwner = null;
        payloadLength = 0;

        if (_position + 27 > _source.Length) return false;

        Span<byte> fixedHeader = stackalloc byte[27];
        if (_source.Read(_position, fixedHeader) != 27) return false;
        if (fixedHeader[0] != (byte)'O' || fixedHeader[1] != (byte)'g' ||
            fixedHeader[2] != (byte)'g' || fixedHeader[3] != (byte)'S')
        {
            throw new InvalidDataException("Missing OggS capture pattern.");
        }
        if (fixedHeader[4] != 0)
        {
            throw new InvalidDataException($"Unsupported Ogg version {fixedHeader[4]}.");
        }

        byte headerType = fixedHeader[5];
        long granule = BinaryPrimitives.ReadInt64LittleEndian(fixedHeader[6..14]);
        uint serial = BinaryPrimitives.ReadUInt32LittleEndian(fixedHeader[14..18]);
        uint sequence = BinaryPrimitives.ReadUInt32LittleEndian(fixedHeader[18..22]);
        uint crc = BinaryPrimitives.ReadUInt32LittleEndian(fixedHeader[22..26]);
        int segmentCount = fixedHeader[26];

        Span<byte> lacing = stackalloc byte[255];
        var lacingSlice = lacing[..segmentCount];
        if (_source.Read(_position + 27, lacingSlice) != segmentCount)
        {
            throw new EndOfStreamException("Truncated Ogg lacing table.");
        }

        int payloadSize = 0;
        for (int i = 0; i < segmentCount; i++) payloadSize += lacingSlice[i];

        int headerSize = 27 + segmentCount;
        var owner = MemoryPool<byte>.Shared.Rent(payloadSize);
        int got = _source.Read(_position + headerSize, owner.Memory.Span[..payloadSize]);
        if (got != payloadSize)
        {
            owner.Dispose();
            throw new EndOfStreamException("Truncated Ogg page payload.");
        }

        header = new OggPageHeader
        {
            HeaderType = headerType,
            GranulePosition = granule,
            SerialNumber = serial,
            SequenceNumber = sequence,
            Crc = crc,
            HeaderSize = headerSize,
            PayloadSize = payloadSize,
        };
        payloadOwner = owner;
        payloadLength = payloadSize;
        _position += headerSize + payloadSize;
        return true;
    }

    /// <summary>
    /// Read the lacing table of the current page (for callers that want packet
    /// boundaries without copying the payload).
    /// </summary>
    public int ReadLacing(Span<byte> destination, out int segmentCount)
    {
        Span<byte> fixedHeader = stackalloc byte[27];
        if (_source.Read(_position, fixedHeader) != 27)
        {
            segmentCount = 0;
            return 0;
        }
        segmentCount = fixedHeader[26];
        if (destination.Length < segmentCount) throw new ArgumentException("Destination too small.", nameof(destination));
        _source.Read(_position + 27, destination[..segmentCount]);
        int total = 0;
        for (int i = 0; i < segmentCount; i++) total += destination[i];
        return total;
    }
}
