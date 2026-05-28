using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class Av1CodecConfigurationRecordTests
{
    [Fact]
    public void TryParse_Decodes_AVIF_8bit_420_Main_Profile()
    {
        // Typical AVIF still: profile=0, level_idx=4 (3.0), tier=0, 8-bit, 4:2:0.
        byte[] payload = new byte[4];
        payload[0] = 0x80 | 0x01;            // marker=1, version=1
        payload[1] = (0 << 5) | 4;            // seq_profile=0, seq_level_idx_0=4
        payload[2] = (0 << 7) | (0 << 6) | (0 << 5) | (0 << 4) | (1 << 3) | (1 << 2) | 0;
        // tier=0, high_bd=0, twelve_bit=0, mono=0, chroma_x=1, chroma_y=1, chroma_pos=0.
        payload[3] = 0x00;                    // reserved + no IPD

        Assert.True(Av1CodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)1, rec.Version);
        Assert.Equal((byte)0, rec.SeqProfile);
        Assert.Equal((byte)4, rec.SeqLevelIdx0);
        Assert.Equal((byte)0, rec.SeqTier0);
        Assert.False(rec.HighBitDepth);
        Assert.False(rec.TwelveBit);
        Assert.False(rec.Monochrome);
        Assert.Equal((byte)1, rec.ChromaSubsamplingX);
        Assert.Equal((byte)1, rec.ChromaSubsamplingY);
        Assert.Equal(Av1ChromaSamplePosition.Unknown, rec.ChromaSamplePosition);
        Assert.Null(rec.InitialPresentationDelay);
        Assert.Empty(rec.ConfigObus);
        Assert.Equal(8, rec.BitDepth);
        Assert.Equal("4:2:0", rec.ChromaFormat);
    }

    [Fact]
    public void TryParse_Decodes_10bit_Profile2_444_HighTier()
    {
        // profile=2, level_idx=10 (5.0), tier=1, 10-bit, 4:4:4 (no subsample).
        byte[] payload = new byte[4];
        payload[0] = 0x80 | 0x01;
        payload[1] = (2 << 5) | 10;
        payload[2] = (1 << 7) | (1 << 6) | (0 << 5) | (0 << 4) | (0 << 3) | (0 << 2) | 2;
        // tier=1, high_bd=1, twelve_bit=0, mono=0, chroma_x=0, chroma_y=0, chroma_pos=2 (Colocated).
        payload[3] = 0x00;

        Assert.True(Av1CodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)2, rec.SeqProfile);
        Assert.Equal((byte)10, rec.SeqLevelIdx0);
        Assert.Equal((byte)1, rec.SeqTier0);
        Assert.True(rec.HighBitDepth);
        Assert.False(rec.TwelveBit);
        Assert.Equal(Av1ChromaSamplePosition.Colocated, rec.ChromaSamplePosition);
        Assert.Equal(10, rec.BitDepth);
        Assert.Equal("4:4:4", rec.ChromaFormat);
    }

    [Fact]
    public void TryParse_Decodes_12bit_Monochrome()
    {
        byte[] payload = new byte[4];
        payload[0] = 0x80 | 0x01;
        payload[1] = 0;
        payload[2] = (0 << 7) | (1 << 6) | (1 << 5) | (1 << 4) | (1 << 3) | (1 << 2) | 0;
        // high_bd=1, twelve_bit=1, mono=1, chroma_x=1, chroma_y=1 (chroma subsampling
        // values are ignored when mono=1; spec mandates they be set to 1 anyway).
        payload[3] = 0;

        Assert.True(Av1CodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.True(rec.HighBitDepth);
        Assert.True(rec.TwelveBit);
        Assert.True(rec.Monochrome);
        Assert.Equal(12, rec.BitDepth);
        Assert.Equal("4:0:0", rec.ChromaFormat);
    }

    [Fact]
    public void TryParse_Decodes_4_2_2_Format()
    {
        byte[] payload = new byte[4];
        payload[0] = 0x80 | 0x01;
        payload[1] = (1 << 5) | 5;            // profile=1, level=5
        payload[2] = (0 << 7) | (0 << 6) | (0 << 5) | (0 << 4) | (1 << 3) | (0 << 2) | 0;
        // chroma_x=1, chroma_y=0 -> 4:2:2
        payload[3] = 0;

        Assert.True(Av1CodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal("4:2:2", rec.ChromaFormat);
        Assert.Equal(8, rec.BitDepth);
    }

    [Fact]
    public void TryParse_Decodes_InitialPresentationDelay()
    {
        byte[] payload = new byte[4];
        payload[0] = 0x80 | 0x01;
        payload[1] = 0;
        payload[2] = 0x0C; // 4:2:0 fields (mono=0, twelve=0, chroma_x=1, chroma_y=1)
        payload[3] = 0x17; // ipd_present=1 + ipd_minus_one=7 -> ipd=8.

        Assert.True(Av1CodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)8, rec.InitialPresentationDelay);
    }

    [Fact]
    public void TryParse_Preserves_Trailing_ConfigObus_Bytes()
    {
        byte[] payload = new byte[4 + 5];
        payload[0] = 0x80 | 0x01;
        payload[1] = 0;
        payload[2] = 0;
        payload[3] = 0;
        payload[4] = 0xDE;
        payload[5] = 0xAD;
        payload[6] = 0xBE;
        payload[7] = 0xEF;
        payload[8] = 0xAA;

        Assert.True(Av1CodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(5, rec.ConfigObus.Length);
        Assert.Equal((byte)0xDE, rec.ConfigObus[0]);
        Assert.Equal((byte)0xAA, rec.ConfigObus[4]);
    }

    [Fact]
    public void TryParse_Rejects_Missing_Marker_Bit()
    {
        byte[] payload = new byte[4];
        payload[0] = 0x01; // marker bit not set
        Assert.False(Av1CodecConfigurationRecord.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated()
    {
        Assert.False(Av1CodecConfigurationRecord.TryParse(new byte[3], out _));
    }

    [Fact]
    public void HeifReader_Resolves_Av1C_Via_Ipma()
    {
        byte[] av1c = new byte[4];
        av1c[0] = 0x80 | 0x01;
        av1c[1] = (0 << 5) | 4;
        av1c[2] = 0x0C; // 4:2:0 (mono=0, twelve=0, chroma_x=1, chroma_y=1)
        av1c[3] = 0x00;

        var bytes = BuildHeifWithProperty("av1C", av1c);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetAv1CodecConfiguration(1, out var rec));
        Assert.Equal((byte)4, rec.SeqLevelIdx0);
        Assert.Equal("4:2:0", rec.ChromaFormat);
        Assert.Equal(8, rec.BitDepth);

        Assert.False(r.TryGetAv1CodecConfiguration(99, out _));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Av1C()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.False(r.TryGetAv1CodecConfiguration(1, out _));
    }

    private static byte[] BuildIspePayload(uint width, uint height)
    {
        byte[] payload = new byte[12];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), width);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), height);
        return payload;
    }

    // Builder mirrors HeifImagePropertiesTests fixture.
    private static byte[] BuildHeifWithProperty(string propertyType, byte[] propertyPayload)
    {
        using var ms = new MemoryStream();
        WriteBox(ms, "ftyp", w =>
        {
            w.Write("avif"u8);
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write("mif1"u8);
            w.Write("avif"u8);
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
                    Encoding.ASCII.GetBytes("av01").CopyTo(data.Slice(8));
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
