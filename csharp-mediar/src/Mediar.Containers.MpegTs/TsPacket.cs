namespace Mediar.Containers.MpegTs;

/// <summary>
/// Decoded MPEG-TS packet header per ISO/IEC 13818-1 2.4.3.2 (188-byte
/// packets only). Constructed by <see cref="TryParse"/> from a 188-byte
/// span; on success <see cref="PayloadOffset"/> points past the fixed
/// 4-byte header and any optional adaptation field.
/// </summary>
internal readonly struct TsPacket
{
    /// <summary>MPEG-TS sync byte (0x47).</summary>
    public const byte SyncByte = 0x47;

    /// <summary>Total packet length used by this demuxer (188 bytes).</summary>
    public const int PacketSize = 188;

    public bool TransportError { get; init; }
    public bool PayloadUnitStartIndicator { get; init; }
    public bool TransportPriority { get; init; }
    public ushort Pid { get; init; }
    public byte TransportScramblingControl { get; init; }
    public byte AdaptationFieldControl { get; init; }
    public byte ContinuityCounter { get; init; }
    public int PayloadOffset { get; init; }
    public int PayloadLength { get; init; }

    public bool HasAdaptationField => (AdaptationFieldControl & 0x2) != 0;
    public bool HasPayload => (AdaptationFieldControl & 0x1) != 0;

    /// <summary>
    /// Attempts to parse a single 188-byte MPEG-TS packet header from the
    /// supplied buffer. Returns false on a missing sync byte, on a
    /// transport-error flag, or when the adaptation field overflows the
    /// payload region.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> packet, out TsPacket value)
    {
        value = default;
        if (packet.Length < PacketSize) return false;
        if (packet[0] != SyncByte) return false;

        bool tei = (packet[1] & 0x80) != 0;
        if (tei) return false;

        bool pusi = (packet[1] & 0x40) != 0;
        bool tp = (packet[1] & 0x20) != 0;
        int pid = ((packet[1] & 0x1F) << 8) | packet[2];
        byte tsc = (byte)((packet[3] >> 6) & 0x3);
        byte afc = (byte)((packet[3] >> 4) & 0x3);
        byte cc = (byte)(packet[3] & 0xF);

        int offset = 4;
        if ((afc & 0x2) != 0)
        {
            if (offset >= PacketSize) return false;
            int afLen = packet[offset];
            offset += 1 + afLen;
            if (offset > PacketSize) return false;
        }

        int payloadLength = (afc & 0x1) != 0 ? PacketSize - offset : 0;

        value = new TsPacket
        {
            TransportError = tei,
            PayloadUnitStartIndicator = pusi,
            TransportPriority = tp,
            Pid = (ushort)pid,
            TransportScramblingControl = tsc,
            AdaptationFieldControl = afc,
            ContinuityCounter = cc,
            PayloadOffset = offset,
            PayloadLength = payloadLength,
        };
        return true;
    }
}
