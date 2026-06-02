using System.Buffers.Binary;
using System.Collections.Immutable;
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

    [Fact]
    public void TryParse_Rejects_Truncated_Min_Luminance()
    {
        // min_present set but only 3 of 4 luminance bytes supplied.
        var payload = new byte[5 + 3];
        payload[4] = 0x10;
        Assert.False(HeifContentColourVolume.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Avg_Luminance()
    {
        var payload = new byte[5 + 1];
        payload[4] = 0x04;
        Assert.False(HeifContentColourVolume.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Empty_Payload()
    {
        Assert.False(HeifContentColourVolume.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void TryParse_Rejects_4_Byte_Payload()
    {
        Assert.False(HeifContentColourVolume.TryParse(new byte[4], out _));
    }

    [Fact]
    public void TryParse_Decodes_Min_Only()
    {
        var payload = BuildCclv(false, false, null, 123u, null, null);
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.Equal(123u, v.MinLuminanceValue);
        Assert.Null(v.MaxLuminanceValue);
        Assert.Null(v.AvgLuminanceValue);
        Assert.True(v.Primaries.IsEmpty);
        Assert.Equal(0.0123, v.MinLuminanceCdM2!.Value, precision: 12);
    }

    [Fact]
    public void TryParse_Decodes_Avg_Only()
    {
        var payload = BuildCclv(false, false, null, null, null, 4_500_000u);
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.Null(v.MinLuminanceValue);
        Assert.Null(v.MaxLuminanceValue);
        Assert.Equal(4_500_000u, v.AvgLuminanceValue);
        Assert.Equal(450.0, v.AvgLuminanceCdM2!.Value, precision: 6);
    }

    [Fact]
    public void TryParse_Decodes_Max_And_Avg_Without_Primaries()
    {
        var payload = BuildCclv(false, false, null, null, 5_000_000u, 2_500_000u);
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.True(v.Primaries.IsEmpty);
        Assert.Equal(500.0, v.MaxLuminanceCdM2!.Value, precision: 6);
        Assert.Equal(250.0, v.AvgLuminanceCdM2!.Value, precision: 6);
    }

    [Fact]
    public void TryParse_Decodes_Max_Uint32_Luminance()
    {
        var payload = BuildCclv(false, false, null, null, uint.MaxValue, null);
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.Equal(uint.MaxValue, v.MaxLuminanceValue);
        Assert.Equal(uint.MaxValue * 0.0001, v.MaxLuminanceCdM2!.Value, precision: 4);
    }

    [Fact]
    public void TryParse_Cancel_With_PersistenceFlag_Preserves_Persistence()
    {
        var payload = new byte[5];
        payload[4] = 0x80 | 0x40; // cancel + persistence
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.True(v.CcvCancelFlag);
        Assert.True(v.CcvPersistenceFlag);
        Assert.True(v.Primaries.IsEmpty);
    }

    [Fact]
    public void TryParse_Cancel_Suppresses_Min_Max_Avg_Luminance()
    {
        // Cancel set; all luminance flags also set — the parser must NOT
        // advance into the conditional body.
        var payload = new byte[5];
        payload[4] = 0x80 | 0x10 | 0x08 | 0x04;
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.True(v.CcvCancelFlag);
        Assert.Null(v.MinLuminanceValue);
        Assert.Null(v.MaxLuminanceValue);
        Assert.Null(v.AvgLuminanceValue);
    }

    [Fact]
    public void TryParse_Tolerates_Trailing_Bytes_After_Final_Field()
    {
        // Valid payload with max_lum present, plus 8 trailing junk bytes.
        var payload = BuildCclv(false, false, null, null, 100u, null);
        var padded = new byte[payload.Length + 8];
        Array.Copy(payload, padded, payload.Length);
        for (int i = payload.Length; i < padded.Length; i++) padded[i] = 0xFF;
        Assert.True(HeifContentColourVolume.TryParse(padded, out var v));
        Assert.Equal(100u, v.MaxLuminanceValue);
    }

    [Fact]
    public void Record_Equality_Compares_All_Fields()
    {
        var a = new HeifContentColourVolume
        {
            CcvCancelFlag = false,
            CcvPersistenceFlag = true,
            Primaries = ImmutableArray<int>.Empty,
            MinLuminanceValue = 1u,
            MaxLuminanceValue = 2u,
            AvgLuminanceValue = 3u,
        };
        var b = new HeifContentColourVolume
        {
            CcvCancelFlag = false,
            CcvPersistenceFlag = true,
            Primaries = ImmutableArray<int>.Empty,
            MinLuminanceValue = 1u,
            MaxLuminanceValue = 2u,
            AvgLuminanceValue = 3u,
        };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
    }

    [Fact]
    public void Record_With_Expression_Mutates_MaxLuminance_Only()
    {
        var a = new HeifContentColourVolume
        {
            CcvCancelFlag = false,
            CcvPersistenceFlag = false,
            Primaries = ImmutableArray<int>.Empty,
            MinLuminanceValue = null,
            MaxLuminanceValue = 100u,
            AvgLuminanceValue = null,
        };
        var b = a with { MaxLuminanceValue = 500u };
        Assert.Equal(500u, b.MaxLuminanceValue);
        Assert.Equal(100u, a.MaxLuminanceValue);
        Assert.Null(b.MinLuminanceValue);
        Assert.Null(b.AvgLuminanceValue);
    }

    [Fact]
    public void TryParse_Primaries_Preserves_Extreme_Int32_Values()
    {
        int[] primariesIn = [int.MinValue, int.MaxValue, 0, -1, 1, 0];
        var payload = BuildCclv(false, false, primariesIn, null, null, null);
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.Equal(primariesIn, v.Primaries);
    }

    [Fact]
    public void TryParse_Primaries_Six_Entries_Preserves_Order()
    {
        int[] primariesIn = [10, 20, 30, 40, 50, 60];
        var payload = BuildCclv(false, false, primariesIn, null, null, null);
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        for (int i = 0; i < 6; i++) Assert.Equal(primariesIn[i], v.Primaries[i]);
    }

    [Theory]
    [InlineData(0, 0.0)]
    [InlineData(1, 0.0001)]
    [InlineData(10_000, 1.0)]
    [InlineData(1_000_000, 100.0)]
    [InlineData(40_000_000u, 4000.0)]
    public void TryParse_MaxLuminance_CdM2_Conversion_Theory(long valueInUnits, double expectedNits)
    {
        var payload = BuildCclv(false, false, null, null, (uint)valueInUnits, null);
        Assert.True(HeifContentColourVolume.TryParse(payload, out var v));
        Assert.Equal(expectedNits, v.MaxLuminanceCdM2!.Value, precision: 6);
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
