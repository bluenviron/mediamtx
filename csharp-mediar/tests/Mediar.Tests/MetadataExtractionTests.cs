using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.Flac;
using Mediar.Containers.Mp3;
using Mediar.Containers.Ogg;
using Mediar.Containers.Wav;
using Xunit;

namespace Mediar.Tests;

public sealed class MetadataExtractionTests
{
    // -----------------------------------------------------------------------
    // GeoLocation parsing
    // -----------------------------------------------------------------------

    [Fact]
    public void GeoLocation_Parses_Short_Iso6709_String()
    {
        Assert.True(GeoLocation.TryParseIso6709("+47.5234-122.3456+0042/", out var loc));
        Assert.Equal(47.5234, loc.Latitude, 4);
        Assert.Equal(-122.3456, loc.Longitude, 4);
        Assert.Equal(42.0, loc.Altitude!.Value, 1);
    }

    [Fact]
    public void GeoLocation_Parses_Without_Altitude()
    {
        Assert.True(GeoLocation.TryParseIso6709("+47.5-122.3/", out var loc));
        Assert.Equal(47.5, loc.Latitude, 2);
        Assert.Equal(-122.3, loc.Longitude, 2);
        Assert.Null(loc.Altitude);
    }

    // -----------------------------------------------------------------------
    // WAV LIST INFO
    // -----------------------------------------------------------------------

    [Fact]
    public void Wav_LIST_INFO_Populates_Metadata()
    {
        // Build a 1-frame PCM s16 mono WAV with LIST INFO chunk.
        byte[] pcm = new byte[]
        {
            0x00, 0x00,
        };
        byte[] info = BuildWavInfo([
            ("INAM", "Song Title"),
            ("IART", "The Artist"),
            ("ICRD", "2024-09-15"),
            ("IGNR", "Ambient"),
        ]);

        byte[] wav = BuildWav(sampleRate: 8000, channels: 1, bits: 16, pcm, infoList: info);

        using var src = new IO.MemoryRandomAccessSource(wav);
        using var dx = WavDemuxer.Open(src);

        Assert.Equal("Song Title", dx.Metadata.Title);
        Assert.Equal("The Artist", dx.Metadata.Artist);
        Assert.Equal("2024-09-15", dx.Metadata.Date);
        Assert.Equal("Ambient", dx.Metadata.Genre);
    }

    // -----------------------------------------------------------------------
    // MP3 ID3v1 + ID3v2
    // -----------------------------------------------------------------------

    [Fact]
    public void Mp3_Id3v1_Populates_Metadata()
    {
        // Build: one valid MP3 frame followed by an ID3v1 trailer.
        byte[] frame = new byte[417];
        frame[0] = 0xFF; frame[1] = 0xFB; frame[2] = 0x90; frame[3] = 0x00;

        byte[] id3v1 = new byte[128];
        Encoding.ASCII.GetBytes("TAG").CopyTo(id3v1.AsSpan(0));
        Encoding.ASCII.GetBytes("Cool Song".PadRight(30, ' ')).CopyTo(id3v1.AsSpan(3, 30));
        Encoding.ASCII.GetBytes("Cool Artist".PadRight(30, ' ')).CopyTo(id3v1.AsSpan(33, 30));
        Encoding.ASCII.GetBytes("Cool Album".PadRight(30, ' ')).CopyTo(id3v1.AsSpan(63, 30));
        Encoding.ASCII.GetBytes("2024").CopyTo(id3v1.AsSpan(93, 4));
        Encoding.ASCII.GetBytes("Comment".PadRight(28, ' ')).CopyTo(id3v1.AsSpan(97, 28));
        id3v1[125] = 0;
        id3v1[126] = 7; // track 7
        id3v1[127] = 0; // blues

        byte[] all = [.. frame, .. id3v1];
        using var src = new IO.MemoryRandomAccessSource(all);
        using var dx = Mp3Demuxer.Open(src);

        Assert.Equal("Cool Song", dx.Metadata.Title);
        Assert.Equal("Cool Artist", dx.Metadata.Artist);
        Assert.Equal("Cool Album", dx.Metadata.Album);
        Assert.Equal("2024", dx.Metadata.Date);
        Assert.Equal(7, dx.Metadata.TrackNumber);
    }

    [Fact]
    public void Mp3_Id3v23_Populates_Metadata()
    {
        // Build a TIT2 + TPE1 ID3v2.3 tag.
        byte[] tit2Frame = BuildId3v23TextFrame("TIT2", "Title 2 Test");
        byte[] tpe1Frame = BuildId3v23TextFrame("TPE1", "Artist 2 Test");
        byte[] frames = [.. tit2Frame, .. tpe1Frame];

        byte[] tag = BuildId3v2Header(version: 3, frames);

        byte[] mpegFrame = new byte[417];
        mpegFrame[0] = 0xFF; mpegFrame[1] = 0xFB; mpegFrame[2] = 0x90; mpegFrame[3] = 0x00;

        byte[] all = [.. tag, .. mpegFrame];
        using var src = new IO.MemoryRandomAccessSource(all);
        using var dx = Mp3Demuxer.Open(src);

        Assert.Equal("Title 2 Test", dx.Metadata.Title);
        Assert.Equal("Artist 2 Test", dx.Metadata.Artist);
    }

    // -----------------------------------------------------------------------
    // FLAC VORBIS_COMMENT
    // -----------------------------------------------------------------------

    [Fact]
    public void Flac_VorbisComment_Populates_Metadata()
    {
        // STREAMINFO (block type 0, mandatory, 34 bytes).
        byte[] streamInfo = new byte[34];
        // minBlockSize, maxBlockSize, minFrameSize, maxFrameSize all zero is fine.
        // sample rate / channels / bps / total samples packed:
        // 20 bits sample rate, 3 bits (channels-1), 5 bits (bps-1), 36 bits totalSamples.
        // Set sample rate = 44100 -> 0xAC44
        // channels = 2 -> 1, bps = 16 -> 15.
        // We'll just write byte-by-byte at offset 10..17 (sample rate starts at bit 0 of byte 10).
        uint sr = 44100;
        int channelsMinus1 = 1;
        int bpsMinus1 = 15;
        // bits 0..19 sample rate, 20..22 channels-1, 23..27 bps-1, 28..63 total samples
        ulong packed = ((ulong)sr << 44) | ((ulong)channelsMinus1 << 41) | ((ulong)bpsMinus1 << 36);
        BinaryPrimitives.WriteUInt64BigEndian(streamInfo.AsSpan(10, 8), packed);

        // VORBIS_COMMENT (block type 4).
        byte[] vorbis = BuildVorbisComment("Mediar", ["TITLE=Flac Title", "ARTIST=Flac Artist", "DATE=2026"]);

        using var ms = new MemoryStream();
        ms.Write(Encoding.ASCII.GetBytes("fLaC"));
        WriteFlacBlock(ms, type: 0, isLast: false, streamInfo);
        WriteFlacBlock(ms, type: 4, isLast: true, vorbis);

        byte[] flac = ms.ToArray();
        using var src = new IO.MemoryRandomAccessSource(flac);
        using var dx = FlacDemuxer.Open(src);

        Assert.Equal("Flac Title", dx.Metadata.Title);
        Assert.Equal("Flac Artist", dx.Metadata.Artist);
        Assert.Equal("2026", dx.Metadata.Date);
    }

    // -----------------------------------------------------------------------
    // Ogg Vorbis comment header
    // -----------------------------------------------------------------------

    [Fact]
    public void Ogg_Vorbis_Comment_Populates_Metadata()
    {
        // ident header (packet 1)
        byte[] ident = new byte[30];
        ident[0] = 1; // type
        Encoding.ASCII.GetBytes("vorbis").CopyTo(ident.AsSpan(1, 6));
        BinaryPrimitives.WriteUInt32LittleEndian(ident.AsSpan(7, 4), 0); // version 0
        ident[11] = 2;          // channels
        BinaryPrimitives.WriteUInt32LittleEndian(ident.AsSpan(12, 4), 44100); // sample rate
        // bytes 16..27 zero rates; 28 blocksize 0/1; 29 framing
        ident[28] = 0; // dummy block sizes
        ident[29] = 1; // framing bit

        // comment header (packet 2) — 0x03 "vorbis" + vorbis comment + framing
        byte[] comment = BuildVorbisComment("Mediar", ["TITLE=Ogg Title", "ARTIST=Ogg Artist"]);
        using var cms = new MemoryStream();
        cms.WriteByte(0x03);
        cms.Write(Encoding.ASCII.GetBytes("vorbis"));
        cms.Write(comment);
        cms.WriteByte(0x01); // framing bit
        byte[] commentPacket = cms.ToArray();

        // setup header (packet 3) — opaque blob for our purposes
        byte[] setup = new byte[16];
        setup[0] = 5;
        Encoding.ASCII.GetBytes("vorbis").CopyTo(setup.AsSpan(1, 6));

        byte[] ogg = BuildOggThreeHeaderStream(ident, commentPacket, setup);

        using var src = new IO.MemoryRandomAccessSource(ogg);
        using var dx = OggDemuxer.Open(src);

        Assert.Equal("Ogg Title", dx.Metadata.Title);
        Assert.Equal("Ogg Artist", dx.Metadata.Artist);
    }

    // -----------------------------------------------------------------------
    // helpers
    // -----------------------------------------------------------------------

    private static byte[] BuildWav(int sampleRate, int channels, int bits, ReadOnlySpan<byte> pcm, byte[] infoList)
    {
        using var ms = new MemoryStream();
        ms.Write(Encoding.ASCII.GetBytes("RIFF"));
        long sizeOffset = ms.Position;
        WriteLe32(ms, 0);
        ms.Write(Encoding.ASCII.GetBytes("WAVE"));

        // fmt
        byte[] fmt = new byte[16];
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.AsSpan(0, 2), 1); // PCM
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.AsSpan(2, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32LittleEndian(fmt.AsSpan(4, 4), (uint)sampleRate);
        BinaryPrimitives.WriteUInt32LittleEndian(fmt.AsSpan(8, 4), (uint)(sampleRate * channels * bits / 8));
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.AsSpan(12, 2), (ushort)(channels * bits / 8));
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.AsSpan(14, 2), (ushort)bits);
        WriteChunk(ms, "fmt ", fmt);

        WriteChunk(ms, "data", pcm);

        ms.Write(Encoding.ASCII.GetBytes("LIST"));
        WriteLe32(ms, (uint)(infoList.Length + 4));
        ms.Write(Encoding.ASCII.GetBytes("INFO"));
        ms.Write(infoList);

        long fileEnd = ms.Position;
        ms.Position = sizeOffset;
        WriteLe32(ms, (uint)(fileEnd - 8));
        return ms.ToArray();
    }

    private static byte[] BuildWavInfo((string id, string value)[] items)
    {
        using var ms = new MemoryStream();
        foreach (var (id, value) in items)
        {
            byte[] data = Encoding.Latin1.GetBytes(value + "\0");
            WriteChunk(ms, id, data);
        }
        return ms.ToArray();
    }

    private static void WriteChunk(MemoryStream ms, string id, ReadOnlySpan<byte> data)
    {
        ms.Write(Encoding.ASCII.GetBytes(id));
        WriteLe32(ms, (uint)data.Length);
        ms.Write(data);
        if ((data.Length & 1) != 0) ms.WriteByte(0);
    }

    private static void WriteLe32(MemoryStream ms, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        ms.Write(b);
    }

    private static byte[] BuildId3v23TextFrame(string id, string value)
    {
        // 4-byte ID, 4-byte size (be), 2-byte flags, 1-byte encoding (0=Latin1), text.
        byte[] payload = new byte[1 + Encoding.Latin1.GetByteCount(value)];
        payload[0] = 0; // ISO-8859-1
        Encoding.Latin1.GetBytes(value).CopyTo(payload.AsSpan(1));

        byte[] frame = new byte[10 + payload.Length];
        Encoding.ASCII.GetBytes(id).CopyTo(frame.AsSpan(0, 4));
        BinaryPrimitives.WriteUInt32BigEndian(frame.AsSpan(4, 4), (uint)payload.Length);
        payload.CopyTo(frame.AsSpan(10));
        return frame;
    }

    private static byte[] BuildId3v2Header(int version, ReadOnlySpan<byte> frames)
    {
        byte[] hdr = new byte[10];
        hdr[0] = (byte)'I'; hdr[1] = (byte)'D'; hdr[2] = (byte)'3';
        hdr[3] = (byte)version; hdr[4] = 0;
        hdr[5] = 0; // no flags
        WriteSynchsafe(hdr.AsSpan(6, 4), (uint)frames.Length);
        return [.. hdr, .. frames];
    }

    private static void WriteSynchsafe(Span<byte> dest, uint value)
    {
        dest[0] = (byte)((value >> 21) & 0x7F);
        dest[1] = (byte)((value >> 14) & 0x7F);
        dest[2] = (byte)((value >> 7) & 0x7F);
        dest[3] = (byte)(value & 0x7F);
    }

    private static byte[] BuildVorbisComment(string vendor, string[] entries)
    {
        using var ms = new MemoryStream();
        byte[] vendorBytes = Encoding.UTF8.GetBytes(vendor);
        WriteLe32(ms, (uint)vendorBytes.Length);
        ms.Write(vendorBytes);
        WriteLe32(ms, (uint)entries.Length);
        foreach (var e in entries)
        {
            byte[] eb = Encoding.UTF8.GetBytes(e);
            WriteLe32(ms, (uint)eb.Length);
            ms.Write(eb);
        }
        return ms.ToArray();
    }

    private static void WriteFlacBlock(MemoryStream ms, byte type, bool isLast, byte[] payload)
    {
        byte head = (byte)((isLast ? 0x80 : 0) | (type & 0x7F));
        ms.WriteByte(head);
        // 24-bit big-endian length
        ms.WriteByte((byte)((payload.Length >> 16) & 0xFF));
        ms.WriteByte((byte)((payload.Length >> 8) & 0xFF));
        ms.WriteByte((byte)(payload.Length & 0xFF));
        ms.Write(payload);
    }

    private static byte[] BuildOggThreeHeaderStream(byte[] ident, byte[] comment, byte[] setup)
    {
        // Page 0: ident (continuation = none, type = bos)
        // Page 1: comment + setup, but to keep things simple, separate pages.
        using var ms = new MemoryStream();
        WriteOggPage(ms, ident,  beginningOfStream: true, endOfStream: false, granule: 0, serial: 1, sequence: 0);
        WriteOggPage(ms, comment, beginningOfStream: false, endOfStream: false, granule: 0, serial: 1, sequence: 1);
        WriteOggPage(ms, setup,   beginningOfStream: false, endOfStream: true,  granule: 0, serial: 1, sequence: 2);
        return ms.ToArray();
    }

    private static void WriteOggPage(
        MemoryStream ms, byte[] packet,
        bool beginningOfStream, bool endOfStream, long granule, uint serial, uint sequence)
    {
        // Segment table: split into 255-byte segments.
        var segs = new List<byte>();
        int remaining = packet.Length;
        while (remaining >= 255)
        {
            segs.Add(255);
            remaining -= 255;
        }
        segs.Add((byte)remaining);

        byte flags = 0;
        if (beginningOfStream) flags |= 0x02;
        if (endOfStream) flags |= 0x04;

        byte[] header = new byte[27 + segs.Count];
        header[0] = (byte)'O'; header[1] = (byte)'g'; header[2] = (byte)'g'; header[3] = (byte)'S';
        header[4] = 0; // version
        header[5] = flags;
        BinaryPrimitives.WriteInt64LittleEndian(header.AsSpan(6, 8), granule);
        BinaryPrimitives.WriteUInt32LittleEndian(header.AsSpan(14, 4), serial);
        BinaryPrimitives.WriteUInt32LittleEndian(header.AsSpan(18, 4), sequence);
        // CRC placeholder at 22..25
        header[26] = (byte)segs.Count;
        for (int i = 0; i < segs.Count; i++) header[27 + i] = segs[i];

        // CRC32 over header (with CRC=0) + packet, using Ogg's polynomial 0x04C11DB7.
        uint crc = OggCrc(header);
        crc = OggCrc(packet, crc);
        BinaryPrimitives.WriteUInt32LittleEndian(header.AsSpan(22, 4), crc);

        ms.Write(header);
        ms.Write(packet);
    }

    private static uint OggCrc(ReadOnlySpan<byte> data, uint seed = 0)
    {
        uint crc = seed;
        for (int i = 0; i < data.Length; i++)
        {
            crc = (crc << 8) ^ OggCrcTable[((crc >> 24) ^ data[i]) & 0xFF];
        }
        return crc;
    }

    private static readonly uint[] OggCrcTable = BuildOggCrcTable();

    private static uint[] BuildOggCrcTable()
    {
        var t = new uint[256];
        for (int i = 0; i < 256; i++)
        {
            uint r = (uint)i << 24;
            for (int j = 0; j < 8; j++) r = (r & 0x80000000u) != 0 ? (r << 1) ^ 0x04C11DB7u : (r << 1);
            t[i] = r;
        }
        return t;
    }
}
