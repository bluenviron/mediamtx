using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the extended ID3v2 text-frame + URL-frame mappings that
/// route additional ID3 frames into the canonical Vorbis-comment key
/// vocabulary on <see cref="MediaMetadataBuilder"/>.
/// </summary>
public sealed class Id3v2ExtendedFrameMappingsTests
{
    [Theory]
    [InlineData("TEXT", "Bob Dylan", "Lyricist")]
    [InlineData("TPE3", "Karajan", "Conductor")]
    [InlineData("TPE4", "Tiesto", "Remixer")]
    [InlineData("TKEY", "Am", "MusicalKey")]
    [InlineData("TMOO", "Melancholic", "Mood")]
    [InlineData("TIT3", "Live Mix", "Subtitle")]
    [InlineData("TIT1", "Symphony", "Work")]
    [InlineData("TSST", "Disc One", "DiscSubtitle")]
    public void V23_Text_Frame_Maps_To_Strong_Property(string frameId, string value, string propertyName)
    {
        var meta = Decode(version: 3, BuildV23TextFrame(frameId, value));
        var prop = typeof(MediaMetadata).GetProperty(propertyName)!;
        Assert.Equal(value, prop.GetValue(meta));
    }

    [Fact]
    public void V23_TBPM_Maps_To_Bpm_Integer()
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TBPM", "128"));
        Assert.Equal(128, meta.Bpm);
    }

    [Fact]
    public void V23_TBPM_Decimal_Rounds()
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TBPM", "127.6"));
        Assert.Equal(128, meta.Bpm);
    }

    [Fact]
    public void V23_TCMP_Maps_To_Compilation_True()
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TCMP", "1"));
        Assert.True(meta.Compilation);
    }

    [Fact]
    public void V23_TCMP_Zero_Maps_To_False()
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TCMP", "0"));
        Assert.False(meta.Compilation);
    }

    [Theory]
    [InlineData("TXT", "Bob Dylan", "Lyricist")]
    [InlineData("TP3", "Karajan", "Conductor")]
    [InlineData("TP4", "Tiesto", "Remixer")]
    [InlineData("TKE", "Am", "MusicalKey")]
    [InlineData("TT3", "Live Mix", "Subtitle")]
    [InlineData("TT1", "Symphony", "Work")]
    public void V22_Three_Char_Aliases_Map_To_Strong_Property(string frameId, string value, string propertyName)
    {
        var meta = Decode(version: 2, BuildV22TextFrame(frameId, value));
        var prop = typeof(MediaMetadata).GetProperty(propertyName)!;
        Assert.Equal(value, prop.GetValue(meta));
    }

    [Fact]
    public void V22_TBP_Maps_To_Bpm()
    {
        var meta = Decode(version: 2, BuildV22TextFrame("TBP", "140"));
        Assert.Equal(140, meta.Bpm);
    }

    [Theory]
    [InlineData("WCOP", "https://example.com/license", "License")]
    [InlineData("WOAR", "https://example.com/artist", "Website")]
    [InlineData("WPUB", "https://example.com/pub", "Website")]
    [InlineData("WCOM", "https://example.com/buy", "Website")]
    [InlineData("WORS", "https://example.com/radio", "Website")]
    public void V23_Url_Frame_Maps_To_Strong_Property(string frameId, string url, string propertyName)
    {
        var meta = Decode(version: 3, BuildV23UrlFrame(frameId, url));
        var prop = typeof(MediaMetadata).GetProperty(propertyName)!;
        Assert.Equal(url, prop.GetValue(meta));
    }

    [Fact]
    public void V23_WXXX_Maps_To_Website()
    {
        // WXXX = [encoding=0][description \0][url]
        byte[] payload =
        [
            0x00,
            (byte)'I', (byte)'D', 0x00,
            (byte)'h', (byte)'t', (byte)'t', (byte)'p', (byte)'s', (byte)':', (byte)'/', (byte)'/', (byte)'a',
        ];
        var meta = Decode(version: 3, BuildFrameWithPayload("WXXX", payload));
        Assert.Equal("https://a", meta.Website);
    }

    [Fact]
    public void V23_Url_Frame_Strips_Trailing_Nul()
    {
        // WOAR with a trailing 0x00; ISO-8859-1 URL.
        byte[] urlBytes = Encoding.Latin1.GetBytes("https://x");
        byte[] payload = new byte[urlBytes.Length + 1];
        urlBytes.CopyTo(payload, 0);
        payload[^1] = 0;
        var meta = Decode(version: 3, BuildFrameWithPayload("WOAR", payload));
        Assert.Equal("https://x", meta.Website);
    }

    [Fact]
    public void V23_First_Wins_Across_Frames()
    {
        // First WOAR wins over WPUB - both target WEBSITE.
        byte[] a = BuildV23UrlFrame("WOAR", "https://first");
        byte[] b = BuildV23UrlFrame("WPUB", "https://second");
        byte[] frames = [.. a, .. b];
        var meta = Decode(version: 3, frames);
        Assert.Equal("https://first", meta.Website);
    }

    // ----- helpers -----

    private static MediaMetadata Decode(int version, byte[] frames)
    {
        byte[] tag = BuildTagHeader(version, frames);
        byte[] mpegFrame = new byte[417];
        mpegFrame[0] = 0xFF; mpegFrame[1] = 0xFB; mpegFrame[2] = 0x90; mpegFrame[3] = 0x00;
        byte[] all = [.. tag, .. mpegFrame];
        using var src = new IO.MemoryRandomAccessSource(all);
        using var dx = Mp3Demuxer.Open(src);
        return dx.Metadata;
    }

    private static byte[] BuildV23TextFrame(string id, string value)
    {
        byte[] payload = new byte[1 + Encoding.Latin1.GetByteCount(value)];
        payload[0] = 0;
        Encoding.Latin1.GetBytes(value).CopyTo(payload.AsSpan(1));
        return BuildFrameWithPayload(id, payload);
    }

    private static byte[] BuildV22TextFrame(string id, string value)
    {
        // ID3v2.2 frame: 3-char ID, 3-byte size BE, payload.
        if (id.Length != 3) throw new ArgumentException("v2.2 ids are 3 chars", nameof(id));
        byte[] payload = new byte[1 + Encoding.Latin1.GetByteCount(value)];
        payload[0] = 0;
        Encoding.Latin1.GetBytes(value).CopyTo(payload.AsSpan(1));
        byte[] frame = new byte[6 + payload.Length];
        Encoding.ASCII.GetBytes(id).CopyTo(frame.AsSpan(0, 3));
        frame[3] = (byte)((payload.Length >> 16) & 0xFF);
        frame[4] = (byte)((payload.Length >> 8) & 0xFF);
        frame[5] = (byte)(payload.Length & 0xFF);
        payload.CopyTo(frame.AsSpan(6));
        return frame;
    }

    private static byte[] BuildV23UrlFrame(string id, string url)
    {
        // URL frames: no encoding byte, just ISO-8859-1 URL.
        byte[] payload = Encoding.Latin1.GetBytes(url);
        return BuildFrameWithPayload(id, payload);
    }

    private static byte[] BuildFrameWithPayload(string id, byte[] payload)
    {
        byte[] frame = new byte[10 + payload.Length];
        Encoding.ASCII.GetBytes(id).CopyTo(frame.AsSpan(0, 4));
        BinaryPrimitives.WriteUInt32BigEndian(frame.AsSpan(4, 4), (uint)payload.Length);
        payload.CopyTo(frame.AsSpan(10));
        return frame;
    }

    private static byte[] BuildTagHeader(int version, byte[] frames)
    {
        byte[] hdr = new byte[10];
        hdr[0] = (byte)'I'; hdr[1] = (byte)'D'; hdr[2] = (byte)'3';
        hdr[3] = (byte)version; hdr[4] = 0;
        hdr[5] = 0;
        uint v = (uint)frames.Length;
        hdr[6] = (byte)((v >> 21) & 0x7F);
        hdr[7] = (byte)((v >> 14) & 0x7F);
        hdr[8] = (byte)((v >> 7) & 0x7F);
        hdr[9] = (byte)(v & 0x7F);
        return [.. hdr, .. frames];
    }
}
