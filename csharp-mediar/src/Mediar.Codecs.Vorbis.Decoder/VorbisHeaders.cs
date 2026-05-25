using System.Text;

namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>Vorbis identification header (packet 0).</summary>
internal sealed record VorbisIdentificationHeader
{
    public uint VorbisVersion { get; init; }
    public int Channels { get; init; }
    public int SampleRate { get; init; }
    public int Blocksize0 { get; init; }   // short window length
    public int Blocksize1 { get; init; }   // long window length
}

/// <summary>Vorbis comment header (packet 1) — vendor string + user-supplied key=value pairs.</summary>
internal sealed record VorbisCommentHeader
{
    public string Vendor { get; init; } = string.Empty;
    public IReadOnlyList<string> UserComments { get; init; } = [];
}

internal static class VorbisHeaders
{
    // Vorbis "magic" bytes appearing at the start of each header packet.
    // Common bytes: 0x76 0x6F 0x72 0x62 0x69 0x73 → "vorbis"
    private static readonly byte[] Magic = "vorbis"u8.ToArray();

    public static VorbisIdentificationHeader ParseIdentification(ReadOnlySpan<byte> packet)
    {
        if (packet.Length < 30) throw new InvalidDataException("Vorbis ID header too short.");
        if (packet[0] != 1) throw new InvalidDataException("Expected packet type 1 (identification).");
        if (!packet.Slice(1, 6).SequenceEqual(Magic)) throw new InvalidDataException("Bad Vorbis ID magic.");

        var r = new VorbisBitReader(packet[7..]);
        uint version = r.ReadBits(32);
        int channels = (int)r.ReadBits(8);
        int sampleRate = (int)r.ReadBits(32);
        // bitrate_maximum / nominal / minimum — informational, skip
        _ = r.ReadBits(32);
        _ = r.ReadBits(32);
        _ = r.ReadBits(32);
        int bs0Exp = (int)r.ReadBits(4);
        int bs1Exp = (int)r.ReadBits(4);
        bool frame = r.ReadBit();
        if (!frame) throw new InvalidDataException("ID header framing bit not set.");
        if (version != 0) throw new InvalidDataException($"Unsupported Vorbis version {version}.");
        if (channels < 1 || channels > 255) throw new InvalidDataException("Invalid channel count.");
        if (sampleRate <= 0) throw new InvalidDataException("Invalid sample rate.");
        int bs0 = 1 << bs0Exp;
        int bs1 = 1 << bs1Exp;
        if (bs0 < 64 || bs0 > 8192 || bs1 < bs0 || bs1 > 8192)
            throw new InvalidDataException("Invalid blocksizes.");

        return new VorbisIdentificationHeader
        {
            VorbisVersion = version,
            Channels = channels,
            SampleRate = sampleRate,
            Blocksize0 = bs0,
            Blocksize1 = bs1,
        };
    }

    public static VorbisCommentHeader ParseComment(ReadOnlySpan<byte> packet)
    {
        if (packet.Length < 16) throw new InvalidDataException("Vorbis comment header too short.");
        if (packet[0] != 3) throw new InvalidDataException("Expected packet type 3 (comment).");
        if (!packet.Slice(1, 6).SequenceEqual(Magic)) throw new InvalidDataException("Bad Vorbis comment magic.");

        int p = 7;
        uint vendorLen = BitConverter.ToUInt32(packet.Slice(p, 4));
        p += 4;
        if (vendorLen > (uint)(packet.Length - p)) throw new InvalidDataException("Vendor length overflow.");
        string vendor = Encoding.UTF8.GetString(packet.Slice(p, (int)vendorLen));
        p += (int)vendorLen;

        uint count = BitConverter.ToUInt32(packet.Slice(p, 4));
        p += 4;
        var comments = new string[count];
        for (int i = 0; i < count; i++)
        {
            uint len = BitConverter.ToUInt32(packet.Slice(p, 4));
            p += 4;
            if (len > (uint)(packet.Length - p)) throw new InvalidDataException("Comment length overflow.");
            comments[i] = Encoding.UTF8.GetString(packet.Slice(p, (int)len));
            p += (int)len;
        }
        return new VorbisCommentHeader { Vendor = vendor, UserComments = comments };
    }

    /// <summary>
    /// Pack one or more packets into Xiph lacing — the format used by
    /// Matroska/WebM <c>CodecPrivate</c> for Vorbis and Opus. Layout:
    /// <c>[count-1][lengths…][packets…]</c> where each length is encoded as
    /// a sequence of 0xFF bytes followed by the remainder.
    /// </summary>
    public static byte[] PackXiphLaced(params ReadOnlySpan<byte[]> packets)
    {
        if (packets.Length == 0) return [];
        if (packets.Length > 256) throw new ArgumentException("Xiph lacing supports at most 256 packets.");

        int header = 1;
        for (int i = 0; i < packets.Length - 1; i++)
        {
            header += packets[i].Length / 255 + 1;
        }
        int total = header;
        foreach (var p in packets) total += p.Length;

        var buf = new byte[total];
        int o = 0;
        buf[o++] = (byte)(packets.Length - 1);
        for (int i = 0; i < packets.Length - 1; i++)
        {
            int len = packets[i].Length;
            while (len >= 255) { buf[o++] = 0xFF; len -= 255; }
            buf[o++] = (byte)len;
        }
        foreach (var p in packets)
        {
            p.AsSpan().CopyTo(buf.AsSpan(o));
            o += p.Length;
        }
        return buf;
    }

    /// <summary>Reverse of <see cref="PackXiphLaced"/>.</summary>
    public static byte[][] UnpackXiphLaced(ReadOnlySpan<byte> blob)
    {
        if (blob.Length < 1) throw new InvalidDataException("Xiph-laced blob too short.");
        int count = blob[0] + 1;
        int o = 1;
        var lens = new int[count];
        for (int i = 0; i < count - 1; i++)
        {
            int len = 0;
            while (o < blob.Length && blob[o] == 0xFF) { len += 255; o++; }
            if (o >= blob.Length) throw new InvalidDataException("Xiph lacing truncated.");
            len += blob[o++];
            lens[i] = len;
        }
        int consumed = o;
        int sumLeading = 0;
        for (int i = 0; i < count - 1; i++) sumLeading += lens[i];
        if (consumed + sumLeading > blob.Length) throw new InvalidDataException("Xiph lacing overflow.");
        lens[count - 1] = blob.Length - consumed - sumLeading;

        var packets = new byte[count][];
        for (int i = 0; i < count; i++)
        {
            packets[i] = blob.Slice(o, lens[i]).ToArray();
            o += lens[i];
        }
        return packets;
    }
}
