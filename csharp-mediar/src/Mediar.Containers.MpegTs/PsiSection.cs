using System.Collections.Immutable;

namespace Mediar.Containers.MpegTs;

/// <summary>
/// Program Association Table entry: one row of the PAT body per
/// ISO/IEC 13818-1 2.4.4.3.
/// </summary>
internal readonly record struct PatEntry(ushort ProgramNumber, ushort PmtPid);

/// <summary>
/// Decoded Program Association Table (single-packet section only).
/// </summary>
internal sealed record Pat(ushort TransportStreamId, byte VersionNumber, ImmutableArray<PatEntry> Entries)
{
    /// <summary>
    /// Parse a PAT section starting at a TS payload that began with a
    /// PUSI = 1 packet. The first byte is the pointer_field; the section
    /// must fit within a single TS payload.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> payload, out Pat? pat)
    {
        pat = null;
        if (payload.Length < 1) return false;
        int pointer = payload[0];
        if (1 + pointer >= payload.Length) return false;
        var section = payload.Slice(1 + pointer);

        if (section.Length < 8) return false;
        if (section[0] != 0x00) return false;

        int sectionLength = ((section[1] & 0x0F) << 8) | section[2];
        if (sectionLength < 5) return false;
        if (3 + sectionLength > section.Length) return false;

        ushort tsId = (ushort)((section[3] << 8) | section[4]);
        byte version = (byte)((section[5] >> 1) & 0x1F);

        int entriesStart = 8;
        int crcStart = 3 + sectionLength - 4;
        if (crcStart < entriesStart) return false;
        int entriesLen = crcStart - entriesStart;
        if ((entriesLen % 4) != 0) return false;

        var builder = ImmutableArray.CreateBuilder<PatEntry>(entriesLen / 4);
        for (int i = 0; i < entriesLen; i += 4)
        {
            int o = entriesStart + i;
            ushort progNum = (ushort)((section[o] << 8) | section[o + 1]);
            ushort pid = (ushort)(((section[o + 2] & 0x1F) << 8) | section[o + 3]);
            builder.Add(new PatEntry(progNum, pid));
        }

        pat = new Pat(tsId, version, builder.MoveToImmutable());
        return true;
    }
}

/// <summary>
/// Decoded entry from the elementary-stream loop of a PMT
/// (ISO/IEC 13818-1 2.4.4.9).
/// </summary>
internal readonly record struct PmtStream(byte StreamType, ushort ElementaryPid);

/// <summary>
/// Decoded Program Map Table (single-packet section only).
/// </summary>
internal sealed record Pmt(
    ushort ProgramNumber,
    byte VersionNumber,
    ushort PcrPid,
    ImmutableArray<PmtStream> Streams)
{
    /// <summary>
    /// Parse a PMT section starting at a TS payload that began with a
    /// PUSI = 1 packet. The first byte is the pointer_field; the section
    /// must fit within a single TS payload.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> payload, out Pmt? pmt)
    {
        pmt = null;
        if (payload.Length < 1) return false;
        int pointer = payload[0];
        if (1 + pointer >= payload.Length) return false;
        var section = payload.Slice(1 + pointer);

        if (section.Length < 12) return false;
        if (section[0] != 0x02) return false;

        int sectionLength = ((section[1] & 0x0F) << 8) | section[2];
        if (sectionLength < 9) return false;
        if (3 + sectionLength > section.Length) return false;

        ushort programNumber = (ushort)((section[3] << 8) | section[4]);
        byte version = (byte)((section[5] >> 1) & 0x1F);
        ushort pcrPid = (ushort)(((section[8] & 0x1F) << 8) | section[9]);
        int programInfoLength = ((section[10] & 0x0F) << 8) | section[11];

        int loopStart = 12 + programInfoLength;
        int crcStart = 3 + sectionLength - 4;
        if (loopStart > crcStart) return false;

        var streamsBuilder = ImmutableArray.CreateBuilder<PmtStream>();
        int o = loopStart;
        while (o + 5 <= crcStart)
        {
            byte streamType = section[o];
            ushort elementaryPid = (ushort)(((section[o + 1] & 0x1F) << 8) | section[o + 2]);
            int esInfoLength = ((section[o + 3] & 0x0F) << 8) | section[o + 4];
            int next = o + 5 + esInfoLength;
            if (next > crcStart) return false;
            streamsBuilder.Add(new PmtStream(streamType, elementaryPid));
            o = next;
        }

        pmt = new Pmt(programNumber, version, pcrPid, streamsBuilder.ToImmutable());
        return true;
    }
}
