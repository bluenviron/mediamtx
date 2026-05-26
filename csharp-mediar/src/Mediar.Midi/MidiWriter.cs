using System.Buffers.Binary;

namespace Mediar.Midi;

/// <summary>
/// Writer for Standard MIDI Files. Produces format-0 or format-1 SMF blobs;
/// running-status compression is automatically applied for consecutive
/// channel-voice messages with the same status byte.
/// </summary>
public static class MidiWriter
{
    /// <summary>Serialise <paramref name="file"/> to disk.</summary>
    public static void WriteFile(string path, MidiFile file)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        ArgumentNullException.ThrowIfNull(file);
        byte[] bytes = Write(file);
        File.WriteAllBytes(path, bytes);
    }

    /// <summary>Serialise <paramref name="file"/> to a fresh byte array.</summary>
    public static byte[] Write(MidiFile file)
    {
        ArgumentNullException.ThrowIfNull(file);
        using var ms = new MemoryStream();
        WriteHeader(ms, file);
        foreach (var t in file.Tracks)
        {
            WriteTrack(ms, t);
        }
        return ms.ToArray();
    }

    private static void WriteHeader(MemoryStream ms, MidiFile file)
    {
        Span<byte> hdr = stackalloc byte[14];
        hdr[0] = (byte)'M'; hdr[1] = (byte)'T'; hdr[2] = (byte)'h'; hdr[3] = (byte)'d';
        BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 6);
        BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(8, 2), file.Format);
        BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(10, 2), (ushort)file.Tracks.Count);
        ushort div;
        if (file.Division.IsTicksPerQuarter)
        {
            div = file.Division.TicksPerQuarter;
        }
        else
        {
            // High byte: signed two's-complement of -frame_rate (high bit always set).
            byte smpte = (byte)(256 - file.Division.SmpteFrames);
            div = (ushort)((smpte << 8) | file.Division.TicksPerFrame);
        }
        BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(12, 2), div);
        ms.Write(hdr);
    }

    private static void WriteTrack(MemoryStream ms, MidiTrack track)
    {
        using var body = new MemoryStream();
        long lastTick = 0;
        byte runningStatus = 0;
        bool hasEndOfTrack = false;
        foreach (var ev in track.Events)
        {
            long delta = ev.Tick - lastTick;
            if (delta < 0) throw new InvalidDataException("MIDI events must be in non-decreasing tick order.");
            lastTick = ev.Tick;
            WriteVarLen(body, (uint)delta);

            if (ev.Type == MidiMessageType.Meta)
            {
                body.WriteByte(0xFF);
                body.WriteByte((byte)ev.MetaType);
                WriteVarLen(body, (uint)ev.Payload.Length);
                if (ev.Payload.Length > 0) body.Write(ev.Payload.Span);
                if (ev.MetaType == MidiMetaType.EndOfTrack) hasEndOfTrack = true;
                runningStatus = 0;
            }
            else if (ev.Type == MidiMessageType.SystemExclusive)
            {
                body.WriteByte(ev.Data1 == 0xF7 ? (byte)0xF7 : (byte)0xF0);
                WriteVarLen(body, (uint)ev.Payload.Length);
                if (ev.Payload.Length > 0) body.Write(ev.Payload.Span);
                runningStatus = 0;
            }
            else
            {
                byte status = (byte)((byte)ev.Type | (ev.Channel & 0x0F));
                if (status != runningStatus)
                {
                    body.WriteByte(status);
                    runningStatus = status;
                }
                body.WriteByte(ev.Data1);
                if ((byte)ev.Type is not (byte)MidiMessageType.ProgramChange
                                  and not (byte)MidiMessageType.ChannelPressure)
                {
                    body.WriteByte(ev.Data2);
                }
            }
        }
        if (!hasEndOfTrack)
        {
            // Append End-of-Track meta event.
            WriteVarLen(body, 0);
            body.WriteByte(0xFF);
            body.WriteByte((byte)MidiMetaType.EndOfTrack);
            body.WriteByte(0);
        }

        Span<byte> hdr = stackalloc byte[8];
        hdr[0] = (byte)'M'; hdr[1] = (byte)'T'; hdr[2] = (byte)'r'; hdr[3] = (byte)'k';
        BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), (uint)body.Length);
        ms.Write(hdr);
        body.Position = 0;
        body.CopyTo(ms);
    }

    internal static void WriteVarLen(MemoryStream ms, uint value)
    {
        Span<byte> buf = stackalloc byte[5];
        int i = buf.Length;
        buf[--i] = (byte)(value & 0x7F);
        value >>= 7;
        while (value > 0)
        {
            buf[--i] = (byte)((value & 0x7F) | 0x80);
            value >>= 7;
        }
        ms.Write(buf[i..]);
    }
}
