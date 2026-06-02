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

    [Theory]
    [InlineData("WAR", "https://example.com/artist", "Website")]
    [InlineData("WPB", "https://example.com/pub", "Website")]
    [InlineData("WCM", "https://example.com/buy", "Website")]
    [InlineData("WCP", "https://example.com/license", "License")]
    public void V22_Url_Frame_Maps_To_Strong_Property(string frameId, string url, string propertyName)
    {
        // ID3v2.2 URL frames are 3-character IDs, no encoding byte.
        byte[] payload = Encoding.Latin1.GetBytes(url);
        byte[] frame = new byte[6 + payload.Length];
        Encoding.ASCII.GetBytes(frameId).CopyTo(frame.AsSpan(0, 3));
        frame[3] = (byte)((payload.Length >> 16) & 0xFF);
        frame[4] = (byte)((payload.Length >> 8) & 0xFF);
        frame[5] = (byte)(payload.Length & 0xFF);
        payload.CopyTo(frame.AsSpan(6));

        var meta = Decode(version: 2, frame);
        var prop = typeof(MediaMetadata).GetProperty(propertyName)!;
        Assert.Equal(url, prop.GetValue(meta));
    }

    [Fact]
    public void V23_TBPM_Empty_Value_Leaves_Bpm_Null()
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TBPM", ""));
        Assert.Null(meta.Bpm);
    }

    [Fact]
    public void V23_TBPM_Non_Numeric_Value_Leaves_Bpm_Null()
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TBPM", "fast"));
        Assert.Null(meta.Bpm);
    }

    [Fact]
    public void V23_TBPM_Above_1000_Decimal_Ignored()
    {
        // BPM accepts doubles in (0, 1000); 1234.5 falls outside the range.
        var meta = Decode(version: 3, BuildV23TextFrame("TBPM", "1234.5"));
        Assert.Null(meta.Bpm);
    }

    [Fact]
    public void V23_TBPM_Negative_Value_Stored_As_Negative_Int()
    {
        // int.TryParse accepts "-5"; range check only applies to the
        // double branch, so a negative integer is stored verbatim.
        var meta = Decode(version: 3, BuildV23TextFrame("TBPM", "-5"));
        Assert.Equal(-5, meta.Bpm);
    }

    [Theory]
    [InlineData("true")]
    [InlineData("TRUE")]
    [InlineData("yes")]
    [InlineData("YES")]
    public void V23_TCMP_Truthy_Values_Map_True(string value)
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TCMP", value));
        Assert.True(meta.Compilation);
    }

    [Theory]
    [InlineData("false")]
    [InlineData("FALSE")]
    [InlineData("no")]
    [InlineData("NO")]
    public void V23_TCMP_Falsy_Values_Map_False(string value)
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TCMP", value));
        Assert.False(meta.Compilation);
    }

    [Fact]
    public void V23_TCMP_Unrecognised_Value_Leaves_Compilation_Null()
    {
        var meta = Decode(version: 3, BuildV23TextFrame("TCMP", "maybe"));
        Assert.Null(meta.Compilation);
    }

    [Fact]
    public void V23_TCMP_First_Wins_Across_Frames()
    {
        byte[] a = BuildV23TextFrame("TCMP", "1");
        byte[] b = BuildV23TextFrame("TCMP", "0");
        var meta = Decode(version: 3, [.. a, .. b]);
        Assert.True(meta.Compilation);
    }

    [Fact]
    public void V23_Unknown_Text_Frame_Is_Silently_Ignored()
    {
        // "TZZZ" is not in MapTextFrame so it maps to "" and is dropped.
        var meta = Decode(version: 3, BuildV23TextFrame("TZZZ", "ignored"));
        // No strong property gets set; metadata stays "empty" of the
        // extended fields this test class cares about.
        Assert.Null(meta.Lyricist);
        Assert.Null(meta.Conductor);
        Assert.Null(meta.Mood);
    }

    [Fact]
    public void V23_TMOO_Has_No_V22_Alias()
    {
        // MapTextFrame only lists "TMOO" -> "MOOD" with no v2.2 alias.
        // Sending a 3-char "TMO" should be silently ignored.
        var meta = Decode(version: 2, BuildV22TextFrame("TMO", "Sad"));
        Assert.Null(meta.Mood);
    }

    [Fact]
    public void V23_Lyricist_Utf16_With_Bom_Decodes_Correctly()
    {
        // Encoding 1 = UTF-16 with BOM
        const string value = "Joni Mitchell";
        var utf16 = Encoding.Unicode.GetPreamble().Concat(Encoding.Unicode.GetBytes(value)).ToArray();
        byte[] payload = new byte[1 + utf16.Length];
        payload[0] = 1;
        utf16.CopyTo(payload, 1);
        byte[] frame = BuildFrameWithPayload("TEXT", payload);
        var meta = Decode(version: 3, frame);
        Assert.Equal(value, meta.Lyricist);
    }

    [Fact]
    public void V23_Conductor_Utf8_Decodes_Correctly()
    {
        // Encoding 3 = UTF-8 (v2.4 addition; supported by the decoder).
        const string value = "Ludwig van Beethoven";
        var utf8 = Encoding.UTF8.GetBytes(value);
        byte[] payload = new byte[1 + utf8.Length];
        payload[0] = 3;
        utf8.CopyTo(payload, 1);
        byte[] frame = BuildFrameWithPayload("TPE3", payload);
        var meta = Decode(version: 3, frame);
        Assert.Equal(value, meta.Conductor);
    }

    [Fact]
    public void V23_WXXX_With_Empty_Description_Still_Captures_Url()
    {
        // [encoding=0][\0 description terminator][url]
        byte[] payload =
        [
            0x00,
            0x00, // empty description terminator
            (byte)'h', (byte)'t', (byte)'t', (byte)'p', (byte)'s', (byte)':', (byte)'/', (byte)'/', (byte)'q',
        ];
        var meta = Decode(version: 3, BuildFrameWithPayload("WXXX", payload));
        Assert.Equal("https://q", meta.Website);
    }

    [Fact]
    public void V23_Url_Frame_With_Empty_Payload_Is_Ignored()
    {
        var meta = Decode(version: 3, BuildFrameWithPayload("WCOP", Array.Empty<byte>()));
        Assert.Null(meta.License);
    }

    [Fact]
    public void V23_License_First_Wins_Across_WCOP_Then_WCP()
    {
        // First WCOP locks in the License before a v2.2-aliased WCP arrives.
        byte[] a = BuildV23UrlFrame("WCOP", "https://license-a");
        byte[] b = BuildV23UrlFrame("WCOP", "https://license-b");
        var meta = Decode(version: 3, [.. a, .. b]);
        Assert.Equal("https://license-a", meta.License);
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
