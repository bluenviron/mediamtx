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
