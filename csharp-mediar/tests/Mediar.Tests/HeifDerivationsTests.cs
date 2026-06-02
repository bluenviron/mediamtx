using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifDerivationsTests
{
    [Fact]
    public void Grid_TryParse_Decodes_16Bit_Output_Dimensions()
    {
        // version=0, flags=0 (16-bit), rows-1=1 (=> 2 rows), cols-1=2 (=> 3 cols),
        // output_width=300, output_height=200.
        byte[] payload = new byte[8];
        payload[0] = 0;
        payload[1] = 0;
        payload[2] = 1;
        payload[3] = 2;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(4), 300);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(6), 200);

        bool ok = HeifGridDerivation.TryParse(payload, out var grid);
        Assert.True(ok);
        Assert.Equal(0, grid.Version);
        Assert.Equal(0, grid.Flags);
        Assert.Equal(2, grid.Rows);
        Assert.Equal(3, grid.Columns);
        Assert.Equal(300u, grid.OutputWidth);
        Assert.Equal(200u, grid.OutputHeight);
    }

    [Fact]
    public void Grid_TryParse_Decodes_32Bit_Output_Dimensions_When_Flag_Set()
    {
        // version=0, flags=1 (32-bit), rows-1=0, cols-1=0,
        // output_width=70000 (>65535), output_height=50000.
        byte[] payload = new byte[12];
        payload[0] = 0;
        payload[1] = 1;
        payload[2] = 0;
        payload[3] = 0;
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), 70000);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), 50000);

        bool ok = HeifGridDerivation.TryParse(payload, out var grid);
        Assert.True(ok);
        Assert.Equal(1, grid.Rows);
        Assert.Equal(1, grid.Columns);
        Assert.Equal(70000u, grid.OutputWidth);
        Assert.Equal(50000u, grid.OutputHeight);
    }

    [Fact]
    public void Grid_TryParse_Rejects_Truncated_Payload()
    {
        byte[] payload = new byte[6];
        Assert.False(HeifGridDerivation.TryParse(payload, out _));
    }

    [Fact]
    public void Overlay_TryParse_Decodes_16Bit_Canvas_And_Offsets()
    {
        // version=0, flags=0 (16-bit), R=0xFF00 G=0x00FF B=0x1234 A=0xFFFF,
        // canvas 100x80, 2 source tiles at (10, 20) and (-5, 30).
        byte[] payload = new byte[2 + 8 + 4 + 2 * 4];
        payload[0] = 0;
        payload[1] = 0;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2), 0xFF00);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(4), 0x00FF);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(6), 0x1234);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(8), 0xFFFF);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(10), 100);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(12), 80);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(14), 10);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(16), 20);
        // Use unchecked cast for negative offset (-5 as int16 -> 0xFFFB).
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(18), unchecked((ushort)(short)-5));
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(20), 30);

        bool ok = HeifOverlayDerivation.TryParse(payload, referenceCount: 2, out var ov);
        Assert.True(ok);
        Assert.Equal((ushort)0xFF00, ov.CanvasFill.R);
        Assert.Equal((ushort)0x00FF, ov.CanvasFill.G);
        Assert.Equal((ushort)0x1234, ov.CanvasFill.B);
        Assert.Equal((ushort)0xFFFF, ov.CanvasFill.A);
        Assert.Equal(100u, ov.OutputWidth);
        Assert.Equal(80u, ov.OutputHeight);
        Assert.Equal(2, ov.Offsets.Length);
        Assert.Equal((10, 20), ov.Offsets[0]);
        Assert.Equal((-5, 30), ov.Offsets[1]);
    }

    [Fact]
    public void Overlay_TryParse_Decodes_32Bit_When_Flag_Set()
    {
        // version=0, flags=1 (32-bit fields), 1 source tile.
        byte[] payload = new byte[2 + 8 + 8 + 8];
        payload[0] = 0;
        payload[1] = 1;
        // canvas fill = transparent black.
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(10), 100000);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(14), 80000);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(18), 1234);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(22), 5678);

        bool ok = HeifOverlayDerivation.TryParse(payload, referenceCount: 1, out var ov);
        Assert.True(ok);
        Assert.Equal(100000u, ov.OutputWidth);
        Assert.Equal(80000u, ov.OutputHeight);
        Assert.Single(ov.Offsets);
        Assert.Equal((1234, 5678), ov.Offsets[0]);
    }

    [Fact]
    public void Overlay_TryParse_Rejects_When_Payload_Smaller_Than_Reference_Count()
    {
        byte[] payload = new byte[2 + 8 + 4]; // header but no offset records
        Assert.False(HeifOverlayDerivation.TryParse(payload, referenceCount: 2, out _));
    }

    [Fact]
    public void HeifReader_Resolves_Grid_Derivation_From_Idat()
    {
        // 2x2 grid item, output 32x32, with 4 dimg sources.
        byte[] gridPayload = new byte[8];
        gridPayload[2] = 1; // rows-1 = 1
        gridPayload[3] = 1; // cols-1 = 1
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(4), 32);
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(6), 32);

        var bytes = BuildHeifWithDerivedItem(
            derivedItemId: 100,
            derivedItemType: "grid",
            payload: gridPayload,
            sourceTileIds: [10, 11, 12, 13],
            useIdat: true);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.True(r.TryGetGridDerivation(100, out var grid));
        Assert.Equal(2, grid.Rows);
        Assert.Equal(2, grid.Columns);
        Assert.Equal(32u, grid.OutputWidth);
        Assert.Equal(32u, grid.OutputHeight);
        // dimg should list the four tile IDs.
        Assert.Equal(4, r.ReferenceGraph.GetDerivedSourcesOf(100).Length);
    }

    [Fact]
    public void HeifReader_Resolves_Overlay_Derivation_From_Idat()
    {
        // 2-tile overlay, canvas 64x32, tiles at (0,0) and (32, 0).
        byte[] iovlPayload = new byte[2 + 8 + 4 + 2 * 4];
        BinaryPrimitives.WriteUInt16BigEndian(iovlPayload.AsSpan(10), 64);
        BinaryPrimitives.WriteUInt16BigEndian(iovlPayload.AsSpan(12), 32);
        BinaryPrimitives.WriteUInt16BigEndian(iovlPayload.AsSpan(14), 0);
        BinaryPrimitives.WriteUInt16BigEndian(iovlPayload.AsSpan(16), 0);
        BinaryPrimitives.WriteUInt16BigEndian(iovlPayload.AsSpan(18), 32);
        BinaryPrimitives.WriteUInt16BigEndian(iovlPayload.AsSpan(20), 0);

        var bytes = BuildHeifWithDerivedItem(
            derivedItemId: 200,
            derivedItemType: "iovl",
            payload: iovlPayload,
            sourceTileIds: [20, 21],
            useIdat: true);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.True(r.TryGetOverlayDerivation(200, out var ov));
        Assert.Equal(64u, ov.OutputWidth);
        Assert.Equal(32u, ov.OutputHeight);
        Assert.Equal(2, ov.Offsets.Length);
        Assert.Equal((0, 0), ov.Offsets[0]);
        Assert.Equal((32, 0), ov.Offsets[1]);
    }

    [Fact]
    public void HeifReader_Identifies_Iden_Derivation()
    {
        var bytes = BuildHeifWithDerivedItem(
            derivedItemId: 300,
            derivedItemType: "iden",
            payload: [],
            sourceTileIds: [30],
            useIdat: true);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.True(r.IsIdentityDerivation(300));
        Assert.False(r.IsIdentityDerivation(30));
    }

    [Fact]
    public void HeifReader_Returns_False_For_Wrong_Derivation_Type()
    {
        byte[] gridPayload = new byte[8];
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(4), 16);
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(6), 16);

        var bytes = BuildHeifWithDerivedItem(
            derivedItemId: 400,
            derivedItemType: "grid",
            payload: gridPayload,
            sourceTileIds: [40, 41],
            useIdat: true);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        // Wrong type lookups return false.
        Assert.False(r.TryGetOverlayDerivation(400, out _));
        Assert.False(r.IsIdentityDerivation(400));
        // Non-existent item returns false.
        Assert.False(r.TryGetGridDerivation(999, out _));
        Assert.False(r.TryGetItemData(999, out _));
    }

    [Fact]
    public void HeifReader_Captures_Construction_Method_From_Iloc()
    {
        byte[] gridPayload = new byte[8];
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(4), 16);
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(6), 16);

        var bytes = BuildHeifWithDerivedItem(
            derivedItemId: 500,
            derivedItemType: "grid",
            payload: gridPayload,
            sourceTileIds: [50, 51],
            useIdat: true);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        // Grid items live in idat -> construction method = 1.
        Assert.Equal((byte)1, r.GetConstructionMethod(500));
        // Tile items omitted from iloc default to 0.
        Assert.Equal((byte)0, r.GetConstructionMethod(50));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    public void Overlay_TryParse_Rejects_Too_Short_Header(int length)
    {
        Assert.False(HeifOverlayDerivation.TryParse(new byte[length], referenceCount: 0, out _));
    }

    [Fact]
    public void Overlay_TryParse_Rejects_When_32Bit_Field_Missing_Canvas_Size()
    {
        // 32-bit mode requires 2 + 8 + 8 = 18 bytes minimum even with 0 references.
        byte[] payload = new byte[2 + 8 + 4]; // header + fill but no width/height
        payload[1] = 1; // flags=1 -> 32-bit
        Assert.False(HeifOverlayDerivation.TryParse(payload, referenceCount: 0, out _));
    }

    [Fact]
    public void Overlay_TryParse_Accepts_Zero_References()
    {
        byte[] payload = new byte[2 + 8 + 4];
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(10), 320);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(12), 240);
        Assert.True(HeifOverlayDerivation.TryParse(payload, referenceCount: 0, out var ov));
        Assert.Equal(320u, ov.OutputWidth);
        Assert.Equal(240u, ov.OutputHeight);
        Assert.Empty(ov.Offsets);
    }

    [Fact]
    public void Overlay_TryParse_32Bit_Mode_Preserves_Negative_Offsets()
    {
        byte[] payload = new byte[2 + 8 + 8 + 8];
        payload[1] = 1; // 32-bit
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(10), 1000);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(14), 500);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(18), unchecked((uint)(int)-100));
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(22), unchecked((uint)(int)-50));

        Assert.True(HeifOverlayDerivation.TryParse(payload, referenceCount: 1, out var ov));
        Assert.Single(ov.Offsets);
        Assert.Equal((-100, -50), ov.Offsets[0]);
    }

    [Fact]
    public void Overlay_TryParse_Preserves_Nonzero_Version_And_Flags()
    {
        byte[] payload = new byte[2 + 8 + 4 + 4];
        payload[0] = 7;       // version
        payload[1] = 0;       // 16-bit
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2), 0x1234);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(4), 0x5678);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(6), 0x9ABC);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(8), 0xDEF0);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(10), 16);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(12), 16);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(14), 1);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(16), 2);

        Assert.True(HeifOverlayDerivation.TryParse(payload, referenceCount: 1, out var ov));
        Assert.Equal((byte)7, ov.Version);
        Assert.Equal((byte)0, ov.Flags);
        Assert.Equal((ushort)0x1234, ov.CanvasFill.R);
        Assert.Equal((ushort)0x5678, ov.CanvasFill.G);
        Assert.Equal((ushort)0x9ABC, ov.CanvasFill.B);
        Assert.Equal((ushort)0xDEF0, ov.CanvasFill.A);
    }

    [Fact]
    public void Grid_TryParse_Rejects_Wide_Flag_With_Only_Eight_Bytes()
    {
        byte[] payload = new byte[8];
        payload[1] = 1; // wide flag set but only 8 bytes (needs 12)
        Assert.False(HeifGridDerivation.TryParse(payload, out _));
    }

    [Fact]
    public void Grid_TryParse_Preserves_Nonzero_Version()
    {
        byte[] payload = new byte[8];
        payload[0] = 5;
        payload[1] = 0;
        payload[2] = 0; payload[3] = 0; // 1x1 grid
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(4), 100);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(6), 50);
        Assert.True(HeifGridDerivation.TryParse(payload, out var g));
        Assert.Equal((byte)5, g.Version);
        Assert.Equal((byte)0, g.Flags);
        Assert.Equal(1, g.Rows);
        Assert.Equal(1, g.Columns);
    }

    [Fact]
    public void Grid_TryParse_Decodes_Max_Rows_And_Columns()
    {
        byte[] payload = new byte[8];
        payload[2] = 255; // rows-1
        payload[3] = 255; // cols-1
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(4), 1);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(6), 1);
        Assert.True(HeifGridDerivation.TryParse(payload, out var g));
        Assert.Equal(256, g.Rows);
        Assert.Equal(256, g.Columns);
    }

    [Fact]
    public void Grid_TryParse_Decodes_32Bit_Output_Max_Values()
    {
        byte[] payload = new byte[12];
        payload[1] = 1;
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), uint.MaxValue);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), uint.MaxValue - 1);
        Assert.True(HeifGridDerivation.TryParse(payload, out var g));
        Assert.Equal(uint.MaxValue, g.OutputWidth);
        Assert.Equal(uint.MaxValue - 1, g.OutputHeight);
    }

    [Fact]
    public void Grid_Record_Equality_And_With_Expression()
    {
        byte[] payload = new byte[8];
        payload[2] = 1; payload[3] = 2;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(4), 100);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(6), 80);
        Assert.True(HeifGridDerivation.TryParse(payload, out var a));
        Assert.True(HeifGridDerivation.TryParse(payload, out var b));
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());

        var c = a with { OutputWidth = 200 };
        Assert.NotEqual(a, c);
        Assert.Equal(200u, c.OutputWidth);
        Assert.Equal(100u, a.OutputWidth);
    }

    [Fact]
    public void Overlay_Record_Equality_And_With_Expression()
    {
        // Use zero references so Offsets resolves to ImmutableArray<T>.Empty singleton
        // (record equality on ImmutableArray<T> uses reference comparison).
        byte[] payload = new byte[2 + 8 + 4];
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(10), 16);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(12), 16);
        Assert.True(HeifOverlayDerivation.TryParse(payload, referenceCount: 0, out var a));
        Assert.True(HeifOverlayDerivation.TryParse(payload, referenceCount: 0, out var b));
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());

        var c = a with { OutputWidth = 64 };
        Assert.NotEqual(a, c);
        Assert.Equal(64u, c.OutputWidth);
    }

    [Fact]
    public void HeifReader_Returns_Empty_Sources_For_NonDerived_Item()
    {
        byte[] gridPayload = new byte[8];
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(4), 16);
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(6), 16);

        var bytes = BuildHeifWithDerivedItem(
            derivedItemId: 600,
            derivedItemType: "grid",
            payload: gridPayload,
            sourceTileIds: [60, 61],
            useIdat: true);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        // Tile items themselves are not derived; they have no sources.
        Assert.Empty(r.ReferenceGraph.GetDerivedSourcesOf(60));
        Assert.Empty(r.ReferenceGraph.GetDerivedSourcesOf(61));
        // Non-existent item also returns empty.
        Assert.Empty(r.ReferenceGraph.GetDerivedSourcesOf(9999));
    }

    [Fact]
    public void HeifReader_TryGetItemData_Returns_Grid_Payload_Bytes()
    {
        byte[] gridPayload = new byte[8];
        gridPayload[2] = 1; gridPayload[3] = 1;
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(4), 48);
        BinaryPrimitives.WriteUInt16BigEndian(gridPayload.AsSpan(6), 48);

        var bytes = BuildHeifWithDerivedItem(
            derivedItemId: 700,
            derivedItemType: "grid",
            payload: gridPayload,
            sourceTileIds: [70, 71, 72, 73],
            useIdat: true);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.True(r.TryGetItemData(700, out var data));
        Assert.Equal(gridPayload, data.ToArray());
    }

    // ---- fixture builder ----
    private static byte[] BuildHeifWithDerivedItem(
        uint derivedItemId,
        string derivedItemType,
        byte[] payload,
        uint[] sourceTileIds,
        bool useIdat)
    {
        using var ms = new MemoryStream();

        WriteBox(ms, "ftyp", w =>
        {
            w.Write(Encoding.ASCII.GetBytes("heic"));
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write(Encoding.ASCII.GetBytes("mif1"));
            w.Write(Encoding.ASCII.GetBytes("heic"));
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
                BinaryPrimitives.WriteUInt16BigEndian(b.Slice(4, 2), (ushort)derivedItemId);
                h.Write(b);
            });

            WriteBox(meta, "iinf", h =>
            {
                var ids = new List<(uint Id, string Type)> { (derivedItemId, derivedItemType) };
                foreach (var sid in sourceTileIds) ids.Add((sid, "hvc1"));

                Span<byte> hdr = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(4, 2), (ushort)ids.Count);
                h.Write(hdr);
                foreach (var (id, type) in ids)
                {
                    WriteBox(h, "infe", inf =>
                    {
                        Span<byte> data = stackalloc byte[15];
                        data[0] = 2;
                        BinaryPrimitives.WriteUInt16BigEndian(data.Slice(4, 2), (ushort)id);
                        Encoding.ASCII.GetBytes(type).CopyTo(data.Slice(8));
                        inf.Write(data);
                    });
                }
            });

            // iloc v1: derivedItemId at offset 0, length = payload length, method = 1 (idat).
            WriteBox(meta, "iloc", h =>
            {
                // FullBox: version 1, flags 0.
                h.WriteByte(1);
                h.WriteByte(0);
                h.WriteByte(0);
                h.WriteByte(0);
                // offset_size=4, length_size=4, base_offset_size=0, index_size=0.
                h.WriteByte((4 << 4) | 4);
                h.WriteByte((0 << 4) | 0);
                // item_count = 1.
                Span<byte> count = stackalloc byte[2];
                BinaryPrimitives.WriteUInt16BigEndian(count, 1);
                h.Write(count);
                // item_id = derivedItemId (u16).
                Span<byte> idBuf = stackalloc byte[2];
                BinaryPrimitives.WriteUInt16BigEndian(idBuf, (ushort)derivedItemId);
                h.Write(idBuf);
                // reserved(12) | construction_method(4) = useIdat ? 1 : 0.
                Span<byte> ctor = stackalloc byte[2];
                BinaryPrimitives.WriteUInt16BigEndian(ctor, useIdat ? (ushort)1 : (ushort)0);
                h.Write(ctor);
                // data_reference_index = 0.
                Span<byte> dri = stackalloc byte[2];
                h.Write(dri);
                // extent_count = 1.
                Span<byte> ec = stackalloc byte[2];
                BinaryPrimitives.WriteUInt16BigEndian(ec, 1);
                h.Write(ec);
                // extent_offset (u32) = 0, extent_length (u32) = payload.Length.
                Span<byte> offBuf = stackalloc byte[4];
                BinaryPrimitives.WriteUInt32BigEndian(offBuf, 0);
                h.Write(offBuf);
                Span<byte> lenBuf = stackalloc byte[4];
                BinaryPrimitives.WriteUInt32BigEndian(lenBuf, (uint)payload.Length);
                h.Write(lenBuf);
            });

            // iref with dimg entries from derivedItemId -> sourceTileIds.
            if (sourceTileIds.Length > 0)
            {
                WriteBox(meta, "iref", h =>
                {
                    Span<byte> vf2 = stackalloc byte[4];
                    h.Write(vf2);
                    WriteBox(h, "dimg", body =>
                    {
                        Span<byte> hdrBuf = stackalloc byte[4];
                        BinaryPrimitives.WriteUInt16BigEndian(hdrBuf.Slice(0, 2), (ushort)derivedItemId);
                        BinaryPrimitives.WriteUInt16BigEndian(hdrBuf.Slice(2, 2), (ushort)sourceTileIds.Length);
                        body.Write(hdrBuf);
                        Span<byte> tb = stackalloc byte[2];
                        foreach (var t in sourceTileIds)
                        {
                            BinaryPrimitives.WriteUInt16BigEndian(tb, (ushort)t);
                            body.Write(tb);
                        }
                    });
                });
            }

            WriteBox(meta, "iprp", iprp =>
            {
                WriteBox(iprp, "ipco", ipco =>
                {
                    WriteBox(ipco, "ispe", isp =>
                    {
                        Span<byte> data = stackalloc byte[12];
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), 32);
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(8, 4), 32);
                        isp.Write(data);
                    });
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    Span<byte> entry = stackalloc byte[4];
                    BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), (ushort)derivedItemId);
                    entry[2] = 1;
                    entry[3] = 1;
                    ipma.Write(entry);
                });
            });

            // idat box carrying the derived-item payload bytes.
            if (useIdat)
            {
                WriteBox(meta, "idat", id => id.Write(payload));
            }
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
