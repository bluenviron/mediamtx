using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifContentColourVolumeTests
{
    [Fact]
    public void TryParse_Decodes_Cancel_Flag()
    {
        // FullBox header + cancel flag set; no further payload.
        var payload = new byte[] { 0, 0, 0, 0, 0x80 };
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.True(v.CcvCancelFlag);
        Assert.False(v.CcvPersistenceFlag);
        Assert.True(v.Primaries.IsEmpty);
        Assert.Null(v.MinLuminanceValue);
        Assert.Null(v.MaxLuminanceValue);
        Assert.Null(v.AvgLuminanceValue);
    }

    [Fact]
    public void TryParse_Decodes_All_Fields_Present()
    {
        // BT.2020 primaries: R(0.708, 0.292) G(0.170, 0.797) B(0.131, 0.046)
        // In CCV units (0.00002): 35400, 14600, 8500, 39850, 6550, 2300
        int[] primariesIn = [35400, 14600, 8500, 39850, 6550, 2300];
        uint minLum = 5;      // 0.0005 cd/m²
        uint maxLum = 10_000_000; // 1000 cd/m²
        uint avgLum = 5_000_000;  // 500 cd/m²

        var payload = BuildCclv(
            cancel: false,
            persistence: true,
            primaries: primariesIn,
            minLum: minLum,
            maxLum: maxLum,
            avgLum: avgLum);

        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.False(v.CcvCancelFlag);
        Assert.True(v.CcvPersistenceFlag);
        Assert.Equal(primariesIn, v.Primaries);
        Assert.Equal(minLum, v.MinLuminanceValue);
        Assert.Equal(maxLum, v.MaxLuminanceValue);
        Assert.Equal(avgLum, v.AvgLuminanceValue);

        Assert.Equal(0.0005, v.MinLuminanceCdM2!.Value, precision: 12);
        Assert.Equal(1000.0, v.MaxLuminanceCdM2!.Value, precision: 6);
        Assert.Equal(500.0, v.AvgLuminanceCdM2!.Value, precision: 6);
    }

    [Fact]
    public void TryParse_Decodes_Only_Max_Luminance()
    {
        var payload = BuildCclv(
            cancel: false,
            persistence: false,
            primaries: null,
            minLum: null,
            maxLum: 4_000_000u,
            avgLum: null);

        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.True(v.Primaries.IsEmpty);
        Assert.Null(v.MinLuminanceValue);
        Assert.Equal(4_000_000u, v.MaxLuminanceValue);
        Assert.Null(v.AvgLuminanceValue);
        Assert.Equal(400.0, v.MaxLuminanceCdM2!.Value, precision: 6);
    }

    [Fact]
    public void TryParse_Decodes_Primaries_Only()
    {
        int[] primaries = [1, -2, 3, -4, 5, -6];
        var payload = BuildCclv(
            cancel: false,
            persistence: false,
            primaries: primaries,
            minLum: null,
            maxLum: null,
            avgLum: null);

        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.Equal(primaries, v.Primaries);
    }

    [Fact]
    public void TryParse_Cancel_Suppresses_Trailing_Fields()
    {
        // cancel flag set AND primaries flag set — primaries must be ignored
        var payload = new byte[5];
        payload[4] = 0x80 | 0x20; // cancel + primaries present
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.True(v.CcvCancelFlag);
        Assert.True(v.Primaries.IsEmpty); // primaries skipped due to cancel
    }

    [Fact]
    public void TryParse_Luminance_Helpers_Return_Null_When_Absent()
    {
        var payload = new byte[] { 0, 0, 0, 0, 0 };
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.Null(v.MinLuminanceCdM2);
        Assert.Null(v.MaxLuminanceCdM2);
        Assert.Null(v.AvgLuminanceCdM2);
    }

    [Fact]
    public void TryParse_Rejects_Wrong_Version()
    {
        var payload = new byte[] { 1, 0, 0, 0, 0x80 };
        Assert.False(HeifContentColourVolume.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(HeifContentColourVolume.TryParse(new byte[4], out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Primaries()
    {
        // primaries_present set but only 20 of 24 primary bytes supplied.
        var payload = new byte[5 + 20];
        payload[4] = 0x20;
        Assert.False(HeifContentColourVolume.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Max_Luminance()
    {
        // max_present set but only 2 of 4 luminance bytes supplied.
        var payload = new byte[5 + 2];
        payload[4] = 0x08;
        Assert.False(HeifContentColourVolume.TryParse(payload, out _));
    }

    [Fact]
    public void HeifReader_Resolves_Cclv_Via_Ipma()
    {
        int[] primariesIn = [35400, 14600, 8500, 39850, 6550, 2300];
        var payload = BuildCclv(false, false, primariesIn, 0, 10_000_000, 4_000_000);
        var bytes = BuildHeifWithProperty("cclv", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetContentColourVolume(1, out var v));
        Assert.Equal(primariesIn, v.Primaries);
        Assert.Equal(1000.0, v.MaxLuminanceCdM2!.Value, precision: 6);
        Assert.Equal(400.0, v.AvgLuminanceCdM2!.Value, precision: 6);

        Assert.False(r.TryGetContentColourVolume(99, out _));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Cclv()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetContentColourVolume(1, out _));
    }

    // ---------- helpers ----------

    private static byte[] BuildCclv(bool cancel, bool persistence, int[]? primaries, uint? minLum, uint? maxLum, uint? avgLum)
    {
        byte flags = 0;
        if (cancel) flags |= 0x80;
        if (persistence) flags |= 0x40;
        if (primaries is not null) flags |= 0x20;
        if (minLum.HasValue) flags |= 0x10;
        if (maxLum.HasValue) flags |= 0x08;
        if (avgLum.HasValue) flags |= 0x04;

        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 }); // FullBox header
        ms.WriteByte(flags);
        if (!cancel)
        {
            if (primaries is not null)
            {
                if (primaries.Length != 6) throw new ArgumentException("primaries must be 6 entries", nameof(primaries));
                Span<byte> buf = stackalloc byte[24];
                for (int i = 0; i < 6; i++) BinaryPrimitives.WriteInt32BigEndian(buf.Slice(i * 4, 4), primaries[i]);
                ms.Write(buf);
            }
            if (minLum.HasValue) WriteU32(ms, minLum.Value);
            if (maxLum.HasValue) WriteU32(ms, maxLum.Value);
            if (avgLum.HasValue) WriteU32(ms, avgLum.Value);
        }
        return ms.ToArray();

        static void WriteU32(Stream s, uint v)
        {
            Span<byte> b = stackalloc byte[4];
            BinaryPrimitives.WriteUInt32BigEndian(b, v);
            s.Write(b);
        }
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
