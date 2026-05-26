using System.Buffers.Binary;
using System.Text;

namespace Mediar.Midi;

/// <summary>
/// Reader for Standard MIDI Files (RFC SMF 1.0). Supports formats 0, 1 and 2;
/// channel-voice messages with running status; SysEx (F0/F7); and all
/// meta-event subtypes defined by the SMF specification.
/// </summary>
public static class MidiReader
{
    /// <summary>Load and parse an SMF file from disk.</summary>
    public static MidiFile ReadFile(string path)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        byte[] bytes = File.ReadAllBytes(path);
        return Read(bytes);
    }

    /// <summary>Parse a fully-buffered SMF blob.</summary>
    public static MidiFile Read(ReadOnlySpan<byte> bytes)
    {
        if (bytes.Length < 14) throw new InvalidDataException("File too small to be SMF.");
        if (bytes[0] != 'M' || bytes[1] != 'T' || bytes[2] != 'h' || bytes[3] != 'd')
            throw new InvalidDataException("Missing 'MThd' header chunk.");

        uint hdrLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.Slice(4, 4));
        if (hdrLen < 6) throw new InvalidDataException("Header chunk too short.");
        ushort format = BinaryPrimitives.ReadUInt16BigEndian(bytes.Slice(8, 2));
        ushort ntrks  = BinaryPrimitives.ReadUInt16BigEndian(bytes.Slice(10, 2));
        ushort div    = BinaryPrimitives.ReadUInt16BigEndian(bytes.Slice(12, 2));

        MidiDivision division;
        if ((div & 0x8000) == 0)
        {
            division = new MidiDivision { IsTicksPerQuarter = true, TicksPerQuarter = div };
        }
        else
        {
            // High byte is a two's-complement signed byte holding -frame_rate.
            byte smpte = (byte)(256 - (byte)(div >> 8));
            division = new MidiDivision
            {
                IsTicksPerQuarter = false,
                SmpteFrames = smpte,
                TicksPerFrame = (byte)(div & 0xFF),
            };
        }

        int pos = 8 + (int)hdrLen;
        var tracks = new List<MidiTrack>(ntrks);
        for (int t = 0; t < ntrks && pos + 8 <= bytes.Length; t++)
        {
            if (bytes[pos] != 'M' || bytes[pos + 1] != 'T' || bytes[pos + 2] != 'r' || bytes[pos + 3] != 'k')
            {
                // Skip unknown chunks per spec.
                uint skipLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.Slice(pos + 4, 4));
                pos += 8 + (int)skipLen;
                t--;
                continue;
            }
            uint trkLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.Slice(pos + 4, 4));
            int trkStart = pos + 8;
            int trkEnd = trkStart + (int)trkLen;
            if (trkEnd > bytes.Length) throw new InvalidDataException("Truncated MTrk chunk.");
            tracks.Add(ReadTrack(bytes[trkStart..trkEnd]));
            pos = trkEnd;
        }

        return new MidiFile
        {
            Format = format,
            Division = division,
            Tracks = tracks,
        };
    }

    private static MidiTrack ReadTrack(ReadOnlySpan<byte> body)
    {
        var events = new List<MidiEvent>(body.Length / 4);
        long tick = 0;
        byte runningStatus = 0;
        string? name = null;
        string? instrument = null;

        int p = 0;
        while (p < body.Length)
        {
            int delta = ReadVarLen(body, ref p);
            tick += delta;
            if (p >= body.Length) break;

            byte status = body[p];
            if (status < 0x80)
            {
                if (runningStatus == 0) throw new InvalidDataException("Running status with no prior status byte.");
                status = runningStatus;
            }
            else
            {
                p++;
            }

            if (status == 0xFF) // meta
            {
                if (p >= body.Length) break;
                byte metaType = body[p++];
                int len = ReadVarLen(body, ref p);
                if (p + len > body.Length) throw new InvalidDataException("Truncated meta event.");
                ReadOnlySpan<byte> payload = body.Slice(p, len);
                p += len;
                var ev = new MidiEvent
                {
                    Tick = tick,
                    Type = MidiMessageType.Meta,
                    MetaType = (MidiMetaType)metaType,
                    Payload = payload.ToArray(),
                };
                events.Add(ev);
                if (metaType == (byte)MidiMetaType.TrackName) name = Encoding.UTF8.GetString(payload);
                else if (metaType == (byte)MidiMetaType.InstrumentName) instrument = Encoding.UTF8.GetString(payload);
                else if (metaType == (byte)MidiMetaType.EndOfTrack) break;
                runningStatus = 0; // meta events reset running status
            }
            else if (status == 0xF0 || status == 0xF7) // SysEx
            {
                int len = ReadVarLen(body, ref p);
                if (p + len > body.Length) throw new InvalidDataException("Truncated SysEx event.");
                ReadOnlySpan<byte> payload = body.Slice(p, len);
                p += len;
                events.Add(new MidiEvent
                {
                    Tick = tick,
                    Type = MidiMessageType.SystemExclusive,
                    Data1 = status,
                    Payload = payload.ToArray(),
                });
                runningStatus = 0;
            }
            else
            {
                byte cmd = (byte)(status & 0xF0);
                byte ch  = (byte)(status & 0x0F);
                byte d1 = 0, d2 = 0;
                if (p < body.Length) d1 = body[p++];
                int paramCount = cmd switch
                {
                    0xC0 or 0xD0 => 1,
                    _ => 2,
                };
                if (paramCount == 2 && p < body.Length) d2 = body[p++];
                events.Add(new MidiEvent
                {
                    Tick = tick,
                    Type = (MidiMessageType)cmd,
                    Channel = ch,
                    Data1 = d1,
                    Data2 = d2,
                });
                runningStatus = status;
            }
        }

        return new MidiTrack { Name = name, Instrument = instrument, Events = events };
    }

    internal static int ReadVarLen(ReadOnlySpan<byte> b, ref int p)
    {
        int v = 0;
        int n = 0;
        while (p < b.Length && n < 4)
        {
            byte by = b[p++];
            v = (v << 7) | (by & 0x7F);
            n++;
            if ((by & 0x80) == 0) return v;
        }
        throw new InvalidDataException("Variable-length quantity overflow.");
    }
}
