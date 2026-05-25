namespace Mediar.Containers.Ogg;

/// <summary>
/// Decoded Ogg page header. Pages carry a logical-stream serial number, a
/// monotonic granule position (codec-specific), and a segment table that
/// describes how the page payload splits across packets.
/// </summary>
public readonly struct OggPageHeader
{
    /// <summary>Header type flags (0x01=continuation, 0x02=bos, 0x04=eos).</summary>
    public byte HeaderType { get; init; }

    /// <summary>Codec-specific monotonic position (samples, frames, ...).</summary>
    public long GranulePosition { get; init; }

    /// <summary>Logical stream serial number (unique per stream).</summary>
    public uint SerialNumber { get; init; }

    /// <summary>Monotonic page sequence number within this logical stream.</summary>
    public uint SequenceNumber { get; init; }

    /// <summary>CRC32 stored in the header (not validated by the demuxer).</summary>
    public uint Crc { get; init; }

    /// <summary>Number of bytes consumed by header + lacing table.</summary>
    public int HeaderSize { get; init; }

    /// <summary>Number of bytes in the page payload (sum of lacing values).</summary>
    public int PayloadSize { get; init; }

    /// <summary>True for the first page of a logical stream.</summary>
    public bool IsBeginningOfStream => (HeaderType & 0x02) != 0;

    /// <summary>True for the last page of a logical stream.</summary>
    public bool IsEndOfStream => (HeaderType & 0x04) != 0;

    /// <summary>True if the page begins with a packet that continues from a previous page.</summary>
    public bool IsContinuation => (HeaderType & 0x01) != 0;
}
