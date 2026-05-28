namespace Mediar.Containers.MpegTs;

/// <summary>
/// Decoded PES packet header per ISO/IEC 13818-1 2.4.3.6. Only the
/// fields needed for sample emission - PTS, DTS, payload start offset -
/// are surfaced. Other PES extension fields are skipped.
/// </summary>
internal readonly struct PesHeader
{
    /// <summary>3-byte packet_start_code_prefix (0x00 0x00 0x01).</summary>
    public const uint StartCode = 0x000001u;

    public byte StreamId { get; init; }
    public int PacketLength { get; init; }
    public long? Pts { get; init; }
    public long? Dts { get; init; }
    public int PayloadOffset { get; init; }

    /// <summary>
    /// Attempts to parse the PES header at the start of <paramref name="data"/>.
    /// Returns false on a missing start code, on truncated input, or on a
    /// stream id that uses the simplified PES-payload variant (program
    /// stream map, padding, etc.).
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out PesHeader header)
    {
        header = default;
        if (data.Length < 6) return false;
        if (data[0] != 0x00 || data[1] != 0x00 || data[2] != 0x01) return false;

        byte streamId = data[3];
        int packetLength = (data[4] << 8) | data[5];

        if (IsSimplifiedStream(streamId))
        {
            header = new PesHeader
            {
                StreamId = streamId,
                PacketLength = packetLength,
                Pts = null,
                Dts = null,
                PayloadOffset = 6,
            };
            return true;
        }

        if (data.Length < 9) return false;
        if ((data[6] & 0xC0) != 0x80) return false;

        int ptsDtsFlags = (data[7] >> 6) & 0x3;
        int headerDataLength = data[8];
        int payloadStart = 9 + headerDataLength;
        if (data.Length < payloadStart) return false;

        long? pts = null;
        long? dts = null;
        if (ptsDtsFlags == 0b10)
        {
            if (data.Length < 9 + 5) return false;
            if (!TryDecodeTimestamp(data.Slice(9, 5), 0b0010, out long ptsVal)) return false;
            pts = ptsVal;
        }
        else if (ptsDtsFlags == 0b11)
        {
            if (data.Length < 9 + 10) return false;
            if (!TryDecodeTimestamp(data.Slice(9, 5), 0b0011, out long ptsVal)) return false;
            if (!TryDecodeTimestamp(data.Slice(14, 5), 0b0001, out long dtsVal)) return false;
            pts = ptsVal;
            dts = dtsVal;
        }

        header = new PesHeader
        {
            StreamId = streamId,
            PacketLength = packetLength,
            Pts = pts,
            Dts = dts,
            PayloadOffset = payloadStart,
        };
        return true;
    }

    private static bool IsSimplifiedStream(byte streamId)
        => streamId == 0xBC // program_stream_map
        || streamId == 0xBE // padding_stream
        || streamId == 0xBF // private_stream_2
        || streamId == 0xF0 // ECM
        || streamId == 0xF1 // EMM
        || streamId == 0xFF // program_stream_directory
        || streamId == 0xF2 // DSMCC
        || streamId == 0xF8; // ITU-T H.222.1 type E

    private static bool TryDecodeTimestamp(ReadOnlySpan<byte> b, int expectedPrefix, out long value)
    {
        value = 0;
        if (b.Length < 5) return false;
        if (((b[0] >> 4) & 0xF) != expectedPrefix) return false;
        if ((b[0] & 0x1) != 1) return false;
        if ((b[2] & 0x1) != 1) return false;
        if ((b[4] & 0x1) != 1) return false;

        long high = ((long)(b[0] >> 1) & 0x7);
        long mid = ((long)b[1] << 7) | ((long)(b[2] >> 1) & 0x7F);
        long low = ((long)b[3] << 7) | ((long)(b[4] >> 1) & 0x7F);
        value = (high << 30) | (mid << 15) | low;
        return true;
    }
}
