using System.Buffers.Binary;
using System.Collections.Immutable;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifReferenceGraphTests
{
    [Fact]
    public void Empty_References_Yield_Empty_Lookups()
    {
        var graph = new HeifReferenceGraph([]);

        Assert.Empty(graph.GetThumbnailsFor(1));
        Assert.Null(graph.GetMasterOfThumbnail(1));
        Assert.Empty(graph.GetDerivedSourcesOf(1));
        Assert.Empty(graph.GetDerivedConsumersOf(1));
        Assert.Empty(graph.GetAuxiliariesFor(1));
        Assert.Null(graph.GetMasterOfAuxiliary(1));
        Assert.Empty(graph.GetItemsDescribedBy(1));
        Assert.Empty(graph.GetMetadataItemsFor(1));
    }

    [Fact]
    public void Thmb_Resolves_From_Thumbnail_To_Master_And_Inverse()
    {
        var graph = new HeifReferenceGraph(
        [
            new HeifReference("thmb", FromItemId: 2, ToItemIds: [1]),
        ]);

        Assert.Equal<uint>([2u], graph.GetThumbnailsFor(1));
        Assert.Equal(1u, graph.GetMasterOfThumbnail(2));
        Assert.Null(graph.GetMasterOfThumbnail(1));
    }

    [Fact]
    public void Multiple_Thumbnails_For_Same_Master_Are_All_Listed()
    {
        var graph = new HeifReferenceGraph(
        [
            new HeifReference("thmb", FromItemId: 2, ToItemIds: [1]),
            new HeifReference("thmb", FromItemId: 3, ToItemIds: [1]),
        ]);

        Assert.Equal<uint>([2u, 3u], graph.GetThumbnailsFor(1));
        Assert.Equal(1u, graph.GetMasterOfThumbnail(2));
        Assert.Equal(1u, graph.GetMasterOfThumbnail(3));
    }

    [Fact]
    public void Dimg_Resolves_Derived_Sources_And_Inverse_Consumers()
    {
        var graph = new HeifReferenceGraph(
        [
            new HeifReference("dimg", FromItemId: 10, ToItemIds: [20, 21, 22, 23]),
        ]);

        Assert.Equal<uint>([20u, 21u, 22u, 23u], graph.GetDerivedSourcesOf(10));
        Assert.Equal<uint>([10u], graph.GetDerivedConsumersOf(20));
        Assert.Equal<uint>([10u], graph.GetDerivedConsumersOf(23));
        Assert.Empty(graph.GetDerivedSourcesOf(20));
    }

    [Fact]
    public void Tile_Item_With_Multiple_Grid_Consumers_Is_Listed_Under_Each()
    {
        var graph = new HeifReferenceGraph(
        [
            new HeifReference("dimg", FromItemId: 10, ToItemIds: [20]),
            new HeifReference("dimg", FromItemId: 11, ToItemIds: [20]),
        ]);

        Assert.Equal<uint>([10u, 11u], graph.GetDerivedConsumersOf(20));
    }

    [Fact]
    public void Auxl_Resolves_Auxiliary_To_Master_And_Inverse()
    {
        var graph = new HeifReferenceGraph(
        [
            new HeifReference("auxl", FromItemId: 5, ToItemIds: [1]),
        ]);

        Assert.Equal<uint>([5u], graph.GetAuxiliariesFor(1));
        Assert.Equal(1u, graph.GetMasterOfAuxiliary(5));
        Assert.Null(graph.GetMasterOfAuxiliary(1));
    }

    [Fact]
    public void Cdsc_Resolves_Metadata_Item_To_Described_Items_And_Inverse()
    {
        var graph = new HeifReferenceGraph(
        [
            new HeifReference("cdsc", FromItemId: 100, ToItemIds: [1]),
            new HeifReference("cdsc", FromItemId: 101, ToItemIds: [1]),
        ]);

        Assert.Equal<uint>([1u], graph.GetItemsDescribedBy(100));
        Assert.Equal<uint>([1u], graph.GetItemsDescribedBy(101));
        Assert.Equal<uint>([100u, 101u], graph.GetMetadataItemsFor(1));
    }

    [Fact]
    public void Unknown_Reference_Type_Is_Ignored()
    {
        var graph = new HeifReferenceGraph(
        [
            new HeifReference("zzzz", FromItemId: 1, ToItemIds: [2]),
        ]);

        Assert.Empty(graph.GetThumbnailsFor(2));
        Assert.Empty(graph.GetDerivedSourcesOf(1));
        Assert.Empty(graph.GetAuxiliariesFor(2));
        Assert.Empty(graph.GetItemsDescribedBy(1));
    }

    [Fact]
    public void Full_Container_Graph_Resolves_All_Relationships()
    {
        // primary item 1 is a 4x4 grid composed of tile items 20..23,
        // primary has an alpha aux at 5, a thumbnail at 2,
        // and an Exif metadata item at 100.
        var refs = ImmutableArray.Create(
            new HeifReference("thmb", FromItemId: 2, ToItemIds: [1]),
            new HeifReference("dimg", FromItemId: 1, ToItemIds: [20, 21, 22, 23]),
            new HeifReference("auxl", FromItemId: 5, ToItemIds: [1]),
            new HeifReference("cdsc", FromItemId: 100, ToItemIds: [1]));

        var graph = new HeifReferenceGraph(refs);

        Assert.Equal<uint>([2u], graph.GetThumbnailsFor(1));
        Assert.Equal<uint>([20u, 21u, 22u, 23u], graph.GetDerivedSourcesOf(1));
        Assert.Equal<uint>([5u], graph.GetAuxiliariesFor(1));
        Assert.Equal<uint>([100u], graph.GetMetadataItemsFor(1));
        Assert.Equal(1u, graph.GetMasterOfThumbnail(2));
        Assert.Equal(1u, graph.GetMasterOfAuxiliary(5));
        Assert.Equal<uint>([1u], graph.GetItemsDescribedBy(100));
    }

    [Fact]
    public void HeifReader_Exposes_PrimaryThumbnailIds_From_Iref()
    {
        var bytes = BuildHeifWithIref(
            primaryItemId: 1,
            refs:
            [
                ("thmb", 2u, [1u]),
            ],
            extraInfeIds: [2]);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Equal(1u, r.PrimaryItemId);
        Assert.Equal<uint>([2u], r.PrimaryThumbnailIds);
        Assert.Equal<uint>([2u], r.ReferenceGraph.GetThumbnailsFor(1));
        Assert.Equal(1u, r.ReferenceGraph.GetMasterOfThumbnail(2));
    }

    [Fact]
    public void HeifReader_Exposes_PrimaryAuxiliaryIds_From_Iref()
    {
        var bytes = BuildHeifWithIref(
            primaryItemId: 1,
            refs:
            [
                ("auxl", 7u, [1u]),
            ],
            extraInfeIds: [7]);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Equal<uint>([7u], r.PrimaryAuxiliaryIds);
        Assert.Equal(1u, r.ReferenceGraph.GetMasterOfAuxiliary(7));
    }

    [Fact]
    public void HeifReader_Reference_Graph_Walks_Grid_Derivation()
    {
        var bytes = BuildHeifWithIref(
            primaryItemId: 1,
            refs:
            [
                ("dimg", 1u, [20u, 21u, 22u, 23u]),
                ("thmb", 2u, [1u]),
                ("cdsc", 100u, [1u]),
            ],
            extraInfeIds: [2, 20, 21, 22, 23, 100]);

        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Equal<uint>([20u, 21u, 22u, 23u], r.ReferenceGraph.GetDerivedSourcesOf(1));
        Assert.Equal<uint>([1u], r.ReferenceGraph.GetDerivedConsumersOf(20));
        Assert.Equal<uint>([2u], r.PrimaryThumbnailIds);
        Assert.Equal<uint>([100u], r.ReferenceGraph.GetMetadataItemsFor(1));
    }

    [Fact]
    public void HeifReader_With_No_Iref_Returns_Empty_Graph_Results()
    {
        var bytes = BuildHeifWithIref(primaryItemId: 1, refs: [], extraInfeIds: []);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Empty(r.References);
        Assert.Empty(r.PrimaryThumbnailIds);
        Assert.Empty(r.PrimaryAuxiliaryIds);
        Assert.Empty(r.ReferenceGraph.GetThumbnailsFor(1));
    }

    // ---- fixture builder ----
    private static byte[] BuildHeifWithIref(
        uint primaryItemId,
        IReadOnlyList<(string Type, uint From, uint[] To)> refs,
        IReadOnlyList<uint> extraInfeIds)
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
                BinaryPrimitives.WriteUInt16BigEndian(b.Slice(4, 2), (ushort)primaryItemId);
                h.Write(b);
            });

            WriteBox(meta, "iinf", h =>
            {
                var allIds = new List<uint> { primaryItemId };
                foreach (var id in extraInfeIds)
                    if (!allIds.Contains(id)) allIds.Add(id);

                Span<byte> hdr = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(4, 2), (ushort)allIds.Count);
                h.Write(hdr);
                foreach (var id in allIds)
                {
                    WriteBox(h, "infe", inf =>
                    {
                        Span<byte> data = stackalloc byte[15];
                        data[0] = 2;
                        BinaryPrimitives.WriteUInt16BigEndian(data.Slice(4, 2), (ushort)id);
                        Encoding.ASCII.GetBytes("hvc1").CopyTo(data.Slice(8));
                        inf.Write(data);
                    });
                }
            });

            if (refs.Count > 0)
            {
                WriteBox(meta, "iref", h =>
                {
                    Span<byte> vf2 = stackalloc byte[4];
                    h.Write(vf2);

                    foreach (var (type, from, to) in refs)
                    {
                        WriteBox(h, type, body =>
                        {
                            Span<byte> hdrBuf = stackalloc byte[2 + 2];
                            BinaryPrimitives.WriteUInt16BigEndian(hdrBuf.Slice(0, 2), (ushort)from);
                            BinaryPrimitives.WriteUInt16BigEndian(hdrBuf.Slice(2, 2), (ushort)to.Length);
                            body.Write(hdrBuf);
                            byte[] tb = new byte[2];
                            foreach (var t in to)
                            {
                                BinaryPrimitives.WriteUInt16BigEndian(tb, (ushort)t);
                                body.Write(tb);
                            }
                        });
                    }
                });
            }

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
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    Span<byte> entry = stackalloc byte[4];
                    BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), (ushort)primaryItemId);
                    entry[2] = 1;
                    entry[3] = 1;
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

