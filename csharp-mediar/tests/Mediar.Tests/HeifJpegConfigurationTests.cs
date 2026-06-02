using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifJpegConfigurationTests
{
    [Fact]
    public void TryParse_Decodes_Single_Byte_Prefix()
    {
        var payload = new byte[] { 0xFF };
        Assert.True(HeifJpegConfiguration.TryParse(payload, out var cfg));
        Assert.Single(cfg.JpegPrefix);
        Assert.Equal((byte)0xFF, cfg.JpegPrefix[0]);
    }

    [Fact]
    public void TryParse_Decodes_Minimal_Soi_Prefix()
    {
        // SOI marker only.
        byte[] payload = [0xFF, 0xD8];
        Assert.True(HeifJpegConfiguration.TryParse(payload, out var cfg));
        Assert.Equal(payload, cfg.JpegPrefix);
    }

    [Fact]
    public void TryParse_Decodes_Realistic_Prefix()
    {
        // SOI + minimal DQT + minimal DHT + tiny SOF0 marker stub. Not a valid
        // JPEG by itself, but representative of jpgC content shape.
        byte[] payload =
        [
            0xFF, 0xD8,                         // SOI
            0xFF, 0xDB, 0x00, 0x03, 0x00,       // DQT (truncated stub)
            0xFF, 0xC4, 0x00, 0x03, 0x00,       // DHT (truncated stub)
            0xFF, 0xC0, 0x00, 0x05, 0x08, 0x01, // SOF0 (truncated stub)
        ];
        Assert.True(HeifJpegConfiguration.TryParse(payload, out var cfg));
        Assert.Equal(payload.Length, cfg.JpegPrefix.Length);
        Assert.Equal(payload, cfg.JpegPrefix);
    }

    [Fact]
    public void TryParse_Rejects_Empty_Payload()
    {
        Assert.False(HeifJpegConfiguration.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void BuildJpegBitstream_Concatenates_Prefix_And_Payload()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8, 0xFF, 0xDB]),
        };
        byte[] itemPayload = [0xFF, 0xDA, 0x00, 0x08, 0xAA, 0xBB, 0xCC, 0xFF, 0xD9];

        var bitstream = cfg.BuildJpegBitstream(itemPayload);

        byte[] expected = [.. cfg.JpegPrefix, .. itemPayload];
        Assert.Equal(expected, bitstream);
    }

    [Fact]
    public void BuildJpegBitstream_Empty_Payload_Returns_Prefix_Only()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8]),
        };
        var bitstream = cfg.BuildJpegBitstream(ReadOnlySpan<byte>.Empty);
        Assert.Equal(cfg.JpegPrefix, bitstream);
    }

    [Fact]
    public void HeifReader_Resolves_JpgC_Via_Ipma()
    {
        byte[] prefix = [0xFF, 0xD8, 0xFF, 0xDB, 0x00, 0x03, 0x00, 0xFF, 0xD9];
        var bytes = BuildHeifWithProperty("jpgC", prefix);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetJpegConfiguration(1, out var cfg));
        Assert.Equal(prefix, cfg.JpegPrefix);

        Assert.False(r.TryGetJpegConfiguration(99, out _));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_JpgC()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetJpegConfiguration(1, out _));
    }

    [Fact]
    public void TryParse_Empty_Yields_Null_Out()
    {
        Assert.False(HeifJpegConfiguration.TryParse(ReadOnlySpan<byte>.Empty, out var cfg));
        Assert.Null(cfg);
    }

    [Theory]
    [InlineData(0x00)]
    [InlineData(0x42)]
    [InlineData(0xFF)]
    public void TryParse_Single_Byte_Round_Trips(byte b)
    {
        Assert.True(HeifJpegConfiguration.TryParse(new[] { b }, out var cfg));
        Assert.Single(cfg.JpegPrefix);
        Assert.Equal(b, cfg.JpegPrefix[0]);
    }

    [Fact]
    public void TryParse_Long_Prefix_Round_Trips()
    {
        var data = new byte[1024];
        for (int i = 0; i < data.Length; i++) data[i] = (byte)((i * 7) & 0xFF);
        Assert.True(HeifJpegConfiguration.TryParse(data, out var cfg));
        Assert.Equal(data.Length, cfg.JpegPrefix.Length);
        for (int i = 0; i < data.Length; i++)
        {
            Assert.Equal(data[i], cfg.JpegPrefix[i]);
        }
    }

    [Fact]
    public void BuildJpegBitstream_Length_Equals_Sum_Of_Inputs()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8, 0xAA]),
        };
        byte[] payload = new byte[37];
        var bits = cfg.BuildJpegBitstream(payload);
        Assert.Equal(cfg.JpegPrefix.Length + payload.Length, bits.Length);
    }

    [Fact]
    public void BuildJpegBitstream_With_Empty_Prefix_Returns_Payload_Copy()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray<byte>.Empty,
        };
        byte[] payload = [0xFF, 0xDA, 0x42, 0xFF, 0xD9];
        var bits = cfg.BuildJpegBitstream(payload);
        Assert.Equal(payload.Length, bits.Length);
        for (int i = 0; i < payload.Length; i++) Assert.Equal(payload[i], bits[i]);
    }

    [Fact]
    public void BuildJpegBitstream_With_Empty_Prefix_And_Empty_Payload_Yields_Empty()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray<byte>.Empty,
        };
        var bits = cfg.BuildJpegBitstream(ReadOnlySpan<byte>.Empty);
        Assert.Empty(bits);
    }

    [Fact]
    public void BuildJpegBitstream_Returns_Fresh_Array_Each_Call()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8]),
        };
        byte[] payload = [0xAA, 0xBB];
        var a = cfg.BuildJpegBitstream(payload);
        var b = cfg.BuildJpegBitstream(payload);
        Assert.NotSame(a, b);
        Assert.Equal(a.Length, b.Length);
        for (int i = 0; i < a.Length; i++) Assert.Equal(a[i], b[i]);
    }

    [Fact]
    public void BuildJpegBitstream_Mutation_Does_Not_Affect_Prefix()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8, 0x11, 0x22]),
        };
        byte[] payload = [0x33, 0x44];
        var bits = cfg.BuildJpegBitstream(payload);
        bits[0] = 0; bits[^1] = 0;
        // Prefix is immutable so a fresh build must still produce 0xFF.
        var bits2 = cfg.BuildJpegBitstream(payload);
        Assert.Equal((byte)0xFF, bits2[0]);
        Assert.Equal((byte)0x44, bits2[^1]);
    }

    [Fact]
    public void Records_With_Equal_Prefix_Compare_Equal()
    {
        // ImmutableArray<byte> equality is by reference; explicitly
        // confirm that record .Equals follows that ImmutableArray
        // semantics (two arrays built from the same bytes share the
        // underlying buffer only when constructed via Create).
        var prefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8]);
        var a = new HeifJpegConfiguration { JpegPrefix = prefix };
        var b = new HeifJpegConfiguration { JpegPrefix = prefix };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
    }

    [Fact]
    public void Record_With_Expression_Replaces_Prefix()
    {
        var a = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8]),
        };
        var b = a with
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0x11, 0x22, 0x33]),
        };
        Assert.NotEqual(a, b);
        Assert.Equal(3, b.JpegPrefix.Length);
    }

    [Fact]
    public void TryParse_Long_Realistic_Prefix_Round_Trips()
    {
        // SOI + DQT block + DHT block + 4 KB of canned table content.
        var ms = new MemoryStream();
        ms.WriteByte(0xFF); ms.WriteByte(0xD8);
        ms.WriteByte(0xFF); ms.WriteByte(0xDB);
        for (int i = 0; i < 64; i++) ms.WriteByte((byte)i);
        ms.WriteByte(0xFF); ms.WriteByte(0xC4);
        for (int i = 0; i < 4096; i++) ms.WriteByte((byte)((i * 31) & 0xFF));
        byte[] payload = ms.ToArray();

        Assert.True(HeifJpegConfiguration.TryParse(payload, out var cfg));
        Assert.Equal(payload.Length, cfg.JpegPrefix.Length);
        for (int i = 0; i < payload.Length; i++)
            Assert.Equal(payload[i], cfg.JpegPrefix[i]);
    }

    [Fact]
    public void BuildJpegBitstream_First_Bytes_Match_Prefix_Order()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8, 0xFF, 0xDB, 0x00, 0x43]),
        };
        byte[] payload = [0x11, 0x22, 0x33];
        var bits = cfg.BuildJpegBitstream(payload);
        for (int i = 0; i < cfg.JpegPrefix.Length; i++)
            Assert.Equal(cfg.JpegPrefix[i], bits[i]);
    }

    [Fact]
    public void BuildJpegBitstream_Last_Bytes_Match_Payload()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8]),
        };
        byte[] payload = [0xAA, 0xBB, 0xCC, 0xDD];
        var bits = cfg.BuildJpegBitstream(payload);
        int offset = cfg.JpegPrefix.Length;
        for (int i = 0; i < payload.Length; i++)
            Assert.Equal(payload[i], bits[offset + i]);
    }

    [Fact]
    public void BuildJpegBitstream_Large_Payload_Roundtrips()
    {
        var prefixBytes = new byte[16];
        for (int i = 0; i < prefixBytes.Length; i++) prefixBytes[i] = (byte)i;
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>(prefixBytes),
        };
        var payload = new byte[8192];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i * 13);
        var bits = cfg.BuildJpegBitstream(payload);
        Assert.Equal(prefixBytes.Length + payload.Length, bits.Length);
        Assert.Equal(prefixBytes[0], bits[0]);
        Assert.Equal(payload[^1], bits[^1]);
    }

    [Fact]
    public void HeifReader_TryGetJpegConfiguration_Returns_False_For_Unknown_Item()
    {
        byte[] prefix = [0xFF, 0xD8];
        var bytes = BuildHeifWithProperty("jpgC", prefix);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetJpegConfiguration(99, out _));
    }

    [Fact]
    public void HeifReader_TryGetJpegConfiguration_Returns_False_For_Empty_JpgC_Box()
    {
        // 0-byte jpgC payload → property parser rejects, reader surfaces false.
        var bytes = BuildHeifWithProperty("jpgC", Array.Empty<byte>());
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetJpegConfiguration(1, out _));
    }

    [Theory]
    [InlineData(2)]
    [InlineData(32)]
    [InlineData(512)]
    public void TryParse_VariousLengths_Roundtrip(int length)
    {
        var data = new byte[length];
        for (int i = 0; i < length; i++) data[i] = (byte)i;
        Assert.True(HeifJpegConfiguration.TryParse(data, out var cfg));
        Assert.Equal(length, cfg.JpegPrefix.Length);
        Assert.Equal(data[0], cfg.JpegPrefix[0]);
        Assert.Equal(data[^1], cfg.JpegPrefix[^1]);
    }

    [Fact]
    public void Record_ToString_Includes_JpegPrefix_Member_Name()
    {
        var cfg = new HeifJpegConfiguration
        {
            JpegPrefix = System.Collections.Immutable.ImmutableArray.Create<byte>([0xFF, 0xD8]),
        };
        Assert.Contains("JpegPrefix", cfg.ToString());
    }

    [Fact]
    public void TryParse_Single_Zero_Byte_Still_Decodes()
    {
        // Even a single 0x00 byte is allowed; the parser only fails on
        // a fully empty span.
        Assert.True(HeifJpegConfiguration.TryParse(new byte[] { 0 }, out var cfg));
        Assert.Single(cfg.JpegPrefix);
        Assert.Equal((byte)0, cfg.JpegPrefix[0]);
    }

    // ---------- helpers ----------

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
                    Encoding.ASCII.GetBytes("jpeg").CopyTo(data.Slice(8));
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
