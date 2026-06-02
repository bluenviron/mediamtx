using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifReaderTests
{
    [Fact]
    public void Parses_Ftyp_Major_Brand_And_Sets_Format()
    {
        var bytes = BuildMinimalHeif(brand: "heic", widthDim: 320, heightDim: 240);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Equal("heic", r.MajorBrand);
        Assert.Equal(ImageFormat.Heic, r.Format);
        Assert.Equal(320, r.Info.Width);
        Assert.Equal(240, r.Info.Height);
    }

    [Fact]
    public void Recognises_Avif_Brand()
    {
        var bytes = BuildMinimalHeif(brand: "avif", widthDim: 64, heightDim: 64);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Equal("avif", r.MajorBrand);
        Assert.Equal(ImageFormat.Avif, r.Format);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_For_Hevc_Decode()
    {
        var bytes = BuildMinimalHeif(brand: "heic", widthDim: 8, heightDim: 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    [Fact]
    public void Exposes_Primary_Item()
    {
        var bytes = BuildMinimalHeif("heic", 100, 100, primaryItemId: 1);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(1u, r.PrimaryItemId);
    }

    [Fact]
    public void Open_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => HeifReader.Open(null!, ImageFormat.Heif));
    }

    [Fact]
    public void Open_NonExistentFile_Throws()
    {
        string path = Path.Combine(Path.GetTempPath(), $"mediar-heif-missing-{Guid.NewGuid():N}.heic");
        Assert.Throws<FileNotFoundException>(() => HeifReader.Open(path));
    }

    [Fact]
    public void CanDecodePixels_Is_False()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Dispose_Idempotent()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void OwnsStreamFalse_Leaves_Source_Open()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        var src = new MemoryStream(bytes);
        var r = HeifReader.Open(src, ImageFormat.Heif, ownsStream: false);
        r.Dispose();
        Assert.True(src.CanRead);
        src.Dispose();
    }

    [Fact]
    public void CompatibleBrands_Populated()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        // Fixture writes major+minor then "mif1" then major again.
        Assert.Contains("mif1", r.CompatibleBrands);
        Assert.Contains("heic", r.CompatibleBrands);
    }

    [Fact]
    public void BrandInfo_Reflects_Major_Brand_Heic()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal("heic", r.BrandInfo.MajorBrand);
        Assert.Equal(HeifCodec.Hevc, r.BrandInfo.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, r.BrandInfo.ContainerKind);
        Assert.False(r.BrandInfo.IsImageSequence);
        Assert.True(r.BrandInfo.HasBrand("heic"));
        Assert.True(r.BrandInfo.HasBrand("mif1"));
        Assert.False(r.BrandInfo.HasBrand("xxxx"));
    }

    [Fact]
    public void BrandInfo_Reflects_Major_Brand_Avif()
    {
        var bytes = BuildMinimalHeif("avif", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal("avif", r.BrandInfo.MajorBrand);
        Assert.Equal(HeifCodec.Av1, r.BrandInfo.PrimaryCodec);
    }

    [Fact]
    public void Items_Contains_Primary_Item()
    {
        var bytes = BuildMinimalHeif("heic", 16, 16, primaryItemId: 1);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.NotEmpty(r.Items);
        Assert.Contains(r.Items, i => i.Id == 1);
    }

    [Fact]
    public void Properties_Contains_Ispe()
    {
        var bytes = BuildMinimalHeif("heic", 32, 24);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Contains(r.Properties, p => p.Type == "ispe");
    }

    [Fact]
    public void Associations_Maps_Primary_To_Ispe()
    {
        var bytes = BuildMinimalHeif("heic", 32, 24, primaryItemId: 1);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.Associations.ContainsKey(1u));
        var indices = r.Associations[1u];
        Assert.NotEmpty(indices);
    }

    [Fact]
    public void References_Empty_For_Minimal_Fixture()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Empty(r.References);
    }

    [Fact]
    public void Primary_Thumbnails_And_Auxiliaries_Empty_When_None()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8, primaryItemId: 1);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Empty(r.PrimaryThumbnailIds);
        Assert.Empty(r.PrimaryAuxiliaryIds);
    }

    [Fact]
    public void GetConstructionMethod_UnknownItem_ReturnsZero()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(0, r.GetConstructionMethod(9999u));
    }

    [Fact]
    public void TryGetItemData_UnknownItem_ReturnsFalse()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.TryGetItemData(9999u, out var data));
        Assert.True(data.IsEmpty);
    }

    [Fact]
    public void TryGetGridDerivation_UnknownItem_ReturnsFalse()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.TryGetGridDerivation(9999u, out _));
    }

    [Fact]
    public void TryGetOverlayDerivation_UnknownItem_ReturnsFalse()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.TryGetOverlayDerivation(9999u, out _));
    }

    [Fact]
    public void IsIdentityDerivation_UnknownItem_ReturnsFalse()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.IsIdentityDerivation(9999u));
    }

    [Fact]
    public void IsIdentityDerivation_HvcItem_ReturnsFalse()
    {
        // The minimal fixture's primary item is "hvc1", not "iden".
        var bytes = BuildMinimalHeif("heic", 8, 8, primaryItemId: 1);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.IsIdentityDerivation(1u));
    }

    [Fact]
    public void Property_Lookup_Helpers_Return_False_When_Property_Missing()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8, primaryItemId: 1);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        // None of these properties exist in the minimal fixture.
        Assert.False(r.TryGetContentLightLevel(1u, out _));
        Assert.False(r.TryGetMasteringDisplayColourVolume(1u, out _));
        Assert.False(r.TryGetCleanAperture(1u, out _));
        Assert.False(r.TryGetPixelInformation(1u, out _));
        Assert.False(r.TryGetAuxiliaryType(1u, out _));
        Assert.False(r.TryGetAv1OperatingPoint(1u, out _));
        Assert.False(r.TryGetAv1LayeredImageIndexing(1u, out _));
        Assert.False(r.TryGetAv1CodecConfiguration(1u, out _));
        Assert.False(r.TryGetHevcCodecConfiguration(1u, out _));
        Assert.False(r.TryGetVvcCodecConfiguration(1u, out _));
        Assert.False(r.TryGetUserDescription(1u, out _));
        Assert.False(r.TryGetContentColourVolume(1u, out _));
        Assert.False(r.TryGetLayerSelector(1u, out _));
        Assert.False(r.TryGetRequiredReference(1u, out _));
        Assert.False(r.TryGetJpegConfiguration(1u, out _));
        Assert.False(r.TryGetTargetOutputLayerSet(1u, out _));
        Assert.False(r.TryGetOperatingPointsInformation(1u, out _));
        Assert.False(r.TryGetImageRotation(1u, out var rot));
        Assert.Equal(HeifImageRotation.None, rot);
        Assert.False(r.TryGetImageMirror(1u, out _));
        Assert.False(r.TryGetPixelAspectRatio(1u, out _));
    }

    [Fact]
    public void Property_Lookup_Helpers_Return_False_For_Item_Without_Associations()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        // Unknown item has no associations row at all.
        Assert.False(r.TryGetImageRotation(9999u, out _));
        Assert.False(r.TryGetImageMirror(9999u, out _));
        Assert.False(r.TryGetPixelAspectRatio(9999u, out _));
    }

    [Fact]
    public void Refining_Mif1_Brand_Stays_Heif()
    {
        var bytes = BuildMinimalHeif("mif1", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(ImageFormat.Heif, r.Format);
        Assert.Equal("mif1", r.MajorBrand);
    }

    [Fact]
    public void Refining_Cr3_Brand_Sets_Cr3()
    {
        var bytes = BuildMinimalHeif("crx ", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(ImageFormat.Cr3, r.Format);
        Assert.Equal(HeifCodec.CanonRaw, r.BrandInfo.PrimaryCodec);
    }

    [Fact]
    public void Format_Preserved_When_Caller_Passes_Specific()
    {
        // Caller-supplied non-Heif format should NOT be refined.
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Avif, ownsStream: true);
        Assert.Equal(ImageFormat.Avif, r.Format);
    }

    [Fact]
    public void Open_NullPath_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => HeifReader.Open((string)null!));
    }

    [Fact]
    public void OwnsStreamTrue_Disposes_Underlying_Stream()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        var inner = new MemoryStream(bytes, writable: false);
        using (var r = HeifReader.Open(inner, ImageFormat.Heif, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Heic, r.Format);
        }
        Assert.False(inner.CanRead);
    }

    [Fact]
    public void Info_Format_Equals_Reader_Format()
    {
        var bytes = BuildMinimalHeif("heic", 8, 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(r.Format, r.Info.Format);
    }

    // ---- fixture builder ----
    private static byte[] BuildMinimalHeif(string brand, int widthDim, int heightDim, uint primaryItemId = 1)
    {
        using var ms = new MemoryStream();

        // ftyp box
        WriteBox(ms, "ftyp", w =>
        {
            w.Write(Encoding.ASCII.GetBytes(brand));
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write(Encoding.ASCII.GetBytes("mif1"));
            w.Write(Encoding.ASCII.GetBytes(brand));
        });

        // meta box (FullBox: version=0 flags=0)
        WriteBox(ms, "meta", meta =>
        {
            Span<byte> vf = stackalloc byte[4];
            meta.Write(vf);

            // hdlr
            WriteBox(meta, "hdlr", h =>
            {
                Span<byte> b = stackalloc byte[25];
                Encoding.ASCII.GetBytes("pict").CopyTo(b.Slice(8));
                h.Write(b);
            });

            // pitm version=0
            WriteBox(meta, "pitm", h =>
            {
                Span<byte> b = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(b.Slice(4, 2), (ushort)primaryItemId);
                h.Write(b);
            });

            // iinf v0 with one infe v2
            WriteBox(meta, "iinf", h =>
            {
                Span<byte> hdr = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(4, 2), 1);
                h.Write(hdr);
                WriteBox(h, "infe", inf =>
                {
                    Span<byte> data = stackalloc byte[15];
                    data[0] = 2;  // version
                    BinaryPrimitives.WriteUInt16BigEndian(data.Slice(4, 2), (ushort)primaryItemId);
                    Encoding.ASCII.GetBytes("hvc1").CopyTo(data.Slice(8));
                    // name: empty NUL
                    inf.Write(data);
                });
            });

            // iprp / ipco / ispe
            WriteBox(meta, "iprp", iprp =>
            {
                WriteBox(iprp, "ipco", ipco =>
                {
                    WriteBox(ipco, "ispe", isp =>
                    {
                        Span<byte> data = stackalloc byte[12];
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), (uint)widthDim);
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(8, 4), (uint)heightDim);
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
                    entry[2] = 1;  // assoc count
                    entry[3] = 1;  // property index 1
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
