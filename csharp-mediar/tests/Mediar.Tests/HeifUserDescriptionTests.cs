using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifUserDescriptionTests
{
    [Fact]
    public void TryParse_Decodes_All_Four_Strings()
    {
        var payload = BuildUdesPayload("en-US", "Sunset over Tahoe", "Captured on a Sony A7R V.", "landscape,sunset,tahoe");

        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal("en-US", rec.Lang);
        Assert.Equal("Sunset over Tahoe", rec.Name);
        Assert.Equal("Captured on a Sony A7R V.", rec.Description);
        Assert.Equal("landscape,sunset,tahoe", rec.Tags);
    }

    [Fact]
    public void TryParse_Decodes_Empty_Strings()
    {
        var payload = BuildUdesPayload("", "", "", "");

        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal("", rec.Lang);
        Assert.Equal("", rec.Name);
        Assert.Equal("", rec.Description);
        Assert.Equal("", rec.Tags);
    }

    [Fact]
    public void TryParse_Decodes_Mixed_Empty_And_Populated()
    {
        var payload = BuildUdesPayload("", "Untitled", "", "");

        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal("", rec.Lang);
        Assert.Equal("Untitled", rec.Name);
        Assert.Equal("", rec.Description);
        Assert.Equal("", rec.Tags);
    }

    [Fact]
    public void TryParse_Decodes_NonAscii_Utf8()
    {
        // mix of Cyrillic, CJK, emoji and combining marks.
        var payload = BuildUdesPayload("ja", "東京の夜", "Tokyo at night — Café 🌙", "夜景,東京");

        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal("ja", rec.Lang);
        Assert.Equal("東京の夜", rec.Name);
        Assert.Equal("Tokyo at night — Café 🌙", rec.Description);
        Assert.Equal("夜景,東京", rec.Tags);
    }

    [Fact]
    public void TryParse_Rejects_Wrong_Version()
    {
        var payload = BuildUdesPayload("en", "A", "B", "C");
        payload[0] = 1; // bogus version
        Assert.False(HeifUserDescription.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(HeifUserDescription.TryParse(new byte[3], out _));
    }

    [Fact]
    public void TryParse_Rejects_Missing_Null_Terminator()
    {
        // Header + "lang" without a trailing NUL.
        var bytes = new byte[] { 0, 0, 0, 0, (byte)'e', (byte)'n' };
        Assert.False(HeifUserDescription.TryParse(bytes, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_After_Two_Strings()
    {
        // Header + "en\0Title\0" - then truncated before description.
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 });
        ms.Write(Encoding.UTF8.GetBytes("en")); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes("Title")); ms.WriteByte(0);
        // No description / tags strings.
        Assert.False(HeifUserDescription.TryParse(ms.ToArray(), out _));
    }

    [Fact]
    public void HeifReader_Resolves_Udes_Via_Ipma()
    {
        var udes = BuildUdesPayload("en", "Test Image", "Captured for unit tests.", "test,fixture");
        var bytes = BuildHeifWithProperty("udes", udes);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetUserDescription(1, out var rec));
        Assert.Equal("en", rec.Lang);
        Assert.Equal("Test Image", rec.Name);
        Assert.Equal("Captured for unit tests.", rec.Description);
        Assert.Equal("test,fixture", rec.Tags);

        Assert.False(r.TryGetUserDescription(99, out _));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Udes()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetUserDescription(1, out _));
    }

    [Fact]
    public void Record_Equality_And_With_Expression()
    {
        var a = new HeifUserDescription { Lang = "en", Name = "T", Description = "D", Tags = "x" };
        var b = new HeifUserDescription { Lang = "en", Name = "T", Description = "D", Tags = "x" };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());

        var c = a with { Lang = "fr" };
        Assert.NotEqual(a, c);
        Assert.Equal("fr", c.Lang);
    }

    [Fact]
    public void TryParse_EmptyData_Returns_False()
    {
        Assert.False(HeifUserDescription.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void TryParse_OnlyHeader_NoStrings_Returns_False()
    {
        // Header present but no string bytes follow.
        Assert.False(HeifUserDescription.TryParse(new byte[4], out _));
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(127)]
    [InlineData(255)]
    public void TryParse_NonZero_Version_Rejected(byte version)
    {
        var payload = BuildUdesPayload("en", "A", "B", "C");
        payload[0] = version;
        Assert.False(HeifUserDescription.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_FlagBytes_Are_Ignored()
    {
        var payload = BuildUdesPayload("en", "Name", "Desc", "Tags");
        payload[1] = 0xAB;
        payload[2] = 0xCD;
        payload[3] = 0xEF;
        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal("en", rec.Lang);
        Assert.Equal("Name", rec.Name);
    }

    [Theory]
    [InlineData("en")]
    [InlineData("fr-FR")]
    [InlineData("zh-Hant-TW")]
    [InlineData("x-custom")]
    public void TryParse_AcceptsCommonBcp47Tags(string lang)
    {
        var payload = BuildUdesPayload(lang, "n", "d", "t");
        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal(lang, rec.Lang);
    }

    [Fact]
    public void TryParse_Long_Description_Roundtrip()
    {
        var description = new string('X', 4096);
        var payload = BuildUdesPayload("en", "Name", description, "tag");
        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal(4096, rec.Description.Length);
        Assert.Equal(description, rec.Description);
    }

    [Fact]
    public void TryParse_Ignores_Extra_Bytes_After_Tags_String()
    {
        // Build a normal payload, then append junk bytes after the final NUL.
        var payload = BuildUdesPayload("en", "n", "d", "t");
        var extended = new byte[payload.Length + 4];
        payload.CopyTo(extended, 0);
        extended[^1] = 0xAB;
        Assert.True(HeifUserDescription.TryParse(extended, out var rec));
        Assert.Equal("en", rec.Lang);
        Assert.Equal("t", rec.Tags);
    }

    [Fact]
    public void TryParse_Header_With_Only_Lang_String_Returns_False()
    {
        // Header + "en\0" → lang OK, but name string is missing.
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 });
        ms.Write(Encoding.UTF8.GetBytes("en")); ms.WriteByte(0);
        Assert.False(HeifUserDescription.TryParse(ms.ToArray(), out _));
    }

    [Fact]
    public void TryParse_Header_With_Three_Strings_But_No_Tags_Returns_False()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 });
        ms.Write(Encoding.UTF8.GetBytes("en")); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes("Name")); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes("Description")); ms.WriteByte(0);
        // No tags string at all.
        Assert.False(HeifUserDescription.TryParse(ms.ToArray(), out _));
    }

    [Fact]
    public void TryParse_Header_With_Three_Strings_Plus_Unterminated_Tags_Returns_False()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 });
        ms.Write(Encoding.UTF8.GetBytes("en")); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes("Name")); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes("Desc")); ms.WriteByte(0);
        // Tags bytes but missing terminating null.
        ms.Write(Encoding.UTF8.GetBytes("tags"));
        Assert.False(HeifUserDescription.TryParse(ms.ToArray(), out _));
    }

    [Fact]
    public void TryParse_FullNul_Block_Yields_Four_Empty_Strings()
    {
        // Header + four bare NULs.
        byte[] payload = [0, 0, 0, 0, 0, 0, 0, 0];
        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal("", rec.Lang);
        Assert.Equal("", rec.Name);
        Assert.Equal("", rec.Description);
        Assert.Equal("", rec.Tags);
    }

    [Fact]
    public void HeifReader_TryGetUserDescription_Returns_False_For_Malformed_Udes()
    {
        // Truncated udes payload (just version byte) → property parser fails.
        var bytes = BuildHeifWithProperty("udes", [0]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetUserDescription(1, out _));
    }

    [Fact]
    public void HeifReader_TryGetUserDescription_Returns_False_For_Wrong_Version_Udes()
    {
        var udes = BuildUdesPayload("en", "n", "d", "t");
        udes[0] = 1; // bogus version
        var bytes = BuildHeifWithProperty("udes", udes);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetUserDescription(1, out _));
    }

    [Fact]
    public void Record_ToString_Includes_All_Four_String_Fields()
    {
        var rec = new HeifUserDescription
        {
            Lang = "L",
            Name = "N",
            Description = "D",
            Tags = "T",
        };
        var s = rec.ToString();
        Assert.Contains("Lang", s);
        Assert.Contains("Name", s);
        Assert.Contains("Description", s);
        Assert.Contains("Tags", s);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(16)]
    [InlineData(256)]
    [InlineData(8192)]
    public void TryParse_VariousSizes_Round_Trip(int nameLength)
    {
        var name = new string('a', nameLength);
        var payload = BuildUdesPayload("en", name, "d", "t");
        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal(nameLength, rec.Name.Length);
        Assert.Equal(name, rec.Name);
    }

    [Fact]
    public void TryParse_Lang_Embedded_NonAscii_Is_Still_Returned()
    {
        // The parser does not validate BCP-47; arbitrary UTF-8 is allowed.
        var payload = BuildUdesPayload("デタラメ-XX", "Name", "Desc", "Tags");
        Assert.True(HeifUserDescription.TryParse(payload, out var rec));
        Assert.Equal("デタラメ-XX", rec.Lang);
    }

    private static byte[] BuildUdesPayload(string lang, string name, string description, string tags)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 }); // version + flags
        ms.Write(Encoding.UTF8.GetBytes(lang)); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes(name)); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes(description)); ms.WriteByte(0);
        ms.Write(Encoding.UTF8.GetBytes(tags)); ms.WriteByte(0);
        return ms.ToArray();
    }

    private static byte[] BuildIspePayload(uint width, uint height)
    {
        byte[] payload = new byte[12];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), width);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), height);
        return payload;
    }

    private static byte[] BuildHeifWithProperty(string propertyType, byte[] propertyPayload)
    {
        using var ms = new MemoryStream();
        WriteBox(ms, "ftyp", w =>
        {
            w.Write("heic"u8);
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write("mif1"u8);
            w.Write("heic"u8);
        });
        WriteBox(ms, "meta", meta =>
        {
            Span<byte> vf = stackalloc byte[4];
            meta.Write(vf);
            WriteBox(meta, "hdlr", h =>
            {
                Span<byte> b = stackalloc byte[25];
                Encoding.ASCII.GetBytes("pict").CopyTo(b.Slice(8));
                h.Write(b);
            });
            WriteBox(meta, "pitm", h =>
            {
                Span<byte> b = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(b.Slice(4, 2), 1);
                h.Write(b);
            });
            WriteBox(meta, "iinf", h =>
            {
                Span<byte> hdr = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(4, 2), 1);
                h.Write(hdr);
                WriteBox(h, "infe", inf =>
                {
                    Span<byte> data = stackalloc byte[15];
                    data[0] = 2;
                    BinaryPrimitives.WriteUInt16BigEndian(data.Slice(4, 2), 1);
                    Encoding.ASCII.GetBytes("hvc1").CopyTo(data.Slice(8));
                    inf.Write(data);
                });
            });
            WriteBox(meta, "iprp", iprp =>
            {
                WriteBox(iprp, "ipco", ipco =>
                {
                    WriteBox(ipco, "ispe", isp =>
                    {
                        Span<byte> data = stackalloc byte[12];
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), 64);
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(8, 4), 64);
                        isp.Write(data);
                    });
                    if (propertyType != "ispe")
                    {
                        WriteBox(ipco, propertyType, p => p.Write(propertyPayload));
                    }
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    int assocCount = propertyType == "ispe" ? 1 : 2;
                    Span<byte> entry = stackalloc byte[3 + assocCount];
                    BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), 1);
                    entry[2] = (byte)assocCount;
                    entry[3] = 1;
                    if (assocCount == 2) entry[4] = 2;
                    ipma.Write(entry);
                });
            });
        });
        return ms.ToArray();
    }

    private static void WriteBox(Stream s, string type, Action<MemoryStream> writePayload)
    {
        using var inner = new MemoryStream();
        writePayload(inner);
        var payload = inner.ToArray();
        int total = payload.Length + 8;
        Span<byte> hdr = stackalloc byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(0, 4), (uint)total);
        Encoding.ASCII.GetBytes(type).CopyTo(hdr.Slice(4, 4));
        s.Write(hdr);
        s.Write(payload);
    }
}
