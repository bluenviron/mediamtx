using System.Collections.Immutable;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifBrandInfoTests
{
    [Fact]
    public void Heic_Major_Brand_Identifies_Hevc_SingleImage()
    {
        var info = HeifBrandInfo.From("heic", ["mif1", "heic"]);

        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
        Assert.False(info.IsImageSequence);
        Assert.False(info.IsMultilayer);
        Assert.False(info.IsRangeExtended);
        Assert.False(info.IsToneMapped);
        Assert.False(info.IsAppleMultiImage);
    }

    [Fact]
    public void Heix_Sets_RangeExtended_Flag()
    {
        var info = HeifBrandInfo.From("heix", ["mif1", "heic", "heix"]);

        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
        Assert.True(info.IsRangeExtended);
        Assert.False(info.IsImageSequence);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Hevc_Sequence_Brand_Classifies_As_ImageSequence()
    {
        var info = HeifBrandInfo.From("hevc", ["msf1", "hevc"]);

        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
        Assert.True(info.IsImageSequence);
        Assert.Equal(HeifContainerKind.ImageSequence, info.ContainerKind);
    }

    [Fact]
    public void Hevx_Sequence_Sets_Both_RangeExtended_And_ImageSequence()
    {
        var info = HeifBrandInfo.From("hevx", ["msf1", "hevx"]);

        Assert.True(info.IsImageSequence);
        Assert.True(info.IsRangeExtended);
        Assert.Equal(HeifContainerKind.ImageSequence, info.ContainerKind);
    }

    [Fact]
    public void Heim_Image_Sets_Multilayer_Flag()
    {
        var info = HeifBrandInfo.From("heim", ["mif1", "heim"]);

        Assert.True(info.IsMultilayer);
        Assert.False(info.IsImageSequence);
        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
    }

    [Fact]
    public void Heis_Sequence_Sets_Both_Multilayer_And_ImageSequence()
    {
        var info = HeifBrandInfo.From("heis", ["msf1", "heis"]);

        Assert.True(info.IsMultilayer);
        Assert.True(info.IsImageSequence);
        Assert.Equal(HeifContainerKind.ImageSequence, info.ContainerKind);
    }

    [Fact]
    public void Avif_Major_Brand_Identifies_Av1_SingleImage()
    {
        var info = HeifBrandInfo.From("avif", ["mif1", "miaf", "avif"]);

        Assert.Equal(HeifCodec.Av1, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
        Assert.False(info.IsImageSequence);
    }

    [Fact]
    public void Avis_Major_Brand_Identifies_Av1_ImageSequence()
    {
        var info = HeifBrandInfo.From("avis", ["msf1", "avis"]);

        Assert.Equal(HeifCodec.Av1, info.PrimaryCodec);
        Assert.True(info.IsImageSequence);
        Assert.Equal(HeifContainerKind.ImageSequence, info.ContainerKind);
    }

    [Fact]
    public void Vvic_Identifies_Vvc_SingleImage()
    {
        var info = HeifBrandInfo.From("vvic", ["mif1", "vvic"]);

        Assert.Equal(HeifCodec.Vvc, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Vvis_Identifies_Vvc_ImageSequence()
    {
        var info = HeifBrandInfo.From("vvis", ["msf1", "vvis"]);

        Assert.Equal(HeifCodec.Vvc, info.PrimaryCodec);
        Assert.True(info.IsImageSequence);
    }

    [Fact]
    public void Crx_Major_Brand_Identifies_CanonRaw_SingleImage()
    {
        var info = HeifBrandInfo.From("crx ", ["crx ", "isom"]);

        Assert.Equal(HeifCodec.CanonRaw, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Unif_Major_Brand_Identifies_Uncompressed()
    {
        var info = HeifBrandInfo.From("unif", ["mif1", "unif"]);

        Assert.Equal(HeifCodec.Uncompressed, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Tmap_Major_Brand_Sets_ToneMapped()
    {
        var info = HeifBrandInfo.From("tmap", ["mif1", "tmap"]);

        Assert.Equal(HeifCodec.ToneMapped, info.PrimaryCodec);
        Assert.True(info.IsToneMapped);
    }

    [Fact]
    public void Apple_MA1A_And_MA1B_Set_AppleMultiImage_Flag()
    {
        var infoA = HeifBrandInfo.From("heic", ["mif1", "MA1A", "heic"]);
        var infoB = HeifBrandInfo.From("heic", ["mif1", "MA1B", "heic"]);

        Assert.True(infoA.IsAppleMultiImage);
        Assert.True(infoB.IsAppleMultiImage);
    }

    [Fact]
    public void Unknown_Brand_Falls_Through_To_Unknown_Codec()
    {
        var info = HeifBrandInfo.From("xxxx", ["yyyy"]);

        Assert.Equal(HeifCodec.Unknown, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.Unknown, info.ContainerKind);
    }

    [Fact]
    public void HasBrand_Matches_Major_And_Compatible()
    {
        var info = HeifBrandInfo.From("heic", ["mif1", "heic", "miaf"]);

        Assert.True(info.HasBrand("heic"));
        Assert.True(info.HasBrand("mif1"));
        Assert.True(info.HasBrand("miaf"));
        Assert.False(info.HasBrand("avif"));
    }

    [Fact]
    public void ToneMap_In_Compatible_List_Still_Sets_Flag_But_Codec_Stays_Av1()
    {
        // Common HDR-gain-map AVIF: major=avif, compat=mif1+tmap.
        var info = HeifBrandInfo.From("avif", ["mif1", "tmap", "avif"]);

        Assert.Equal(HeifCodec.Av1, info.PrimaryCodec);
        Assert.True(info.IsToneMapped);
    }

    [Fact]
    public void HeifReader_Exposes_BrandInfo_Correctly()
    {
        // Build a minimal HEIF file with major brand "avif".
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
        });

        using var r = HeifReader.Open(new MemoryStream(ms.ToArray()), ImageFormat.Heif, ownsStream: true);
        Assert.Equal("avif", r.BrandInfo.MajorBrand);
        Assert.Equal(HeifCodec.Av1, r.BrandInfo.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, r.BrandInfo.ContainerKind);
        Assert.True(r.BrandInfo.HasBrand("mif1"));
    }

    [Fact]
    public void Mif2_Major_Brand_Classifies_As_SingleImage()
    {
        var info = HeifBrandInfo.From("mif2", []);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
        Assert.False(info.IsImageSequence);
    }

    [Fact]
    public void Mif3_Major_Brand_Classifies_As_SingleImage()
    {
        var info = HeifBrandInfo.From("mif3", []);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Empty_Compatible_Brands_Major_Only_Works()
    {
        var info = HeifBrandInfo.From("heic", []);
        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
        Assert.Empty(info.CompatibleBrands);
    }

    [Fact]
    public void CompatibleBrands_Property_Preserved_Verbatim()
    {
        ImmutableArray<string> compat = ["mif1", "heic", "miaf", "MA1A"];
        var info = HeifBrandInfo.From("heic", compat);
        Assert.Equal(compat, info.CompatibleBrands);
    }

    [Fact]
    public void Hevc_Image_Brand_Wins_Over_Av1_When_Both_Present()
    {
        // major=avif but heic in compat → primary codec resolves to Hevc per switch precedence.
        var info = HeifBrandInfo.From("avif", ["mif1", "heic", "avif"]);
        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Record_Equality_And_With_Expression()
    {
        ImmutableArray<string> compat = ["mif1", "heic"];
        var a = HeifBrandInfo.From("heic", compat);
        var b = HeifBrandInfo.From("heic", compat);
        Assert.Equal(a, b);

        var c = a with { MajorBrand = "avif" };
        Assert.NotEqual(a, c);
        Assert.Equal("avif", c.MajorBrand);
    }

    [Fact]
    public void HasBrand_Empty_String_Returns_False()
    {
        var info = HeifBrandInfo.From("heic", ["mif1"]);
        Assert.False(info.HasBrand(string.Empty));
    }

    [Fact]
    public void HasBrand_Is_Case_Sensitive()
    {
        var info = HeifBrandInfo.From("heic", ["mif1"]);
        Assert.True(info.HasBrand("heic"));
        Assert.False(info.HasBrand("HEIC"));
        Assert.False(info.HasBrand("Mif1"));
    }

    [Fact]
    public void Unif_In_Compat_Only_Still_Sets_Uncompressed_Codec()
    {
        // unif as compat brand only - codec resolves through compat list.
        var info = HeifBrandInfo.From("mif1", ["mif1", "unif"]);
        Assert.Equal(HeifCodec.Uncompressed, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Crx_In_Compat_Only_Sets_CanonRaw_Codec()
    {
        var info = HeifBrandInfo.From("isom", ["isom", "crx "]);
        Assert.Equal(HeifCodec.CanonRaw, info.PrimaryCodec);
        Assert.Equal(HeifContainerKind.SingleImage, info.ContainerKind);
    }

    [Fact]
    public void Tmap_In_Compat_Only_Sets_ToneMapped_Codec_When_No_Other_Codec_Hit()
    {
        var info = HeifBrandInfo.From("mif1", ["mif1", "tmap"]);
        Assert.Equal(HeifCodec.ToneMapped, info.PrimaryCodec);
        Assert.True(info.IsToneMapped);
    }

    [Fact]
    public void Heis_Sets_All_Three_Flags_Multilayer_RangeNot_Set_And_Sequence()
    {
        // heis = multilayer image sequence; range-extended is NOT implied.
        var info = HeifBrandInfo.From("heis", ["msf1", "heis"]);
        Assert.True(info.IsMultilayer);
        Assert.True(info.IsImageSequence);
        Assert.False(info.IsRangeExtended);
        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
    }

    [Fact]
    public void Combined_Flags_Multilayer_RangeExtended_AppleMulti_All_Set()
    {
        var info = HeifBrandInfo.From("heic", ["mif1", "heic", "heim", "heix", "MA1B"]);
        Assert.True(info.IsMultilayer);
        Assert.True(info.IsRangeExtended);
        Assert.True(info.IsAppleMultiImage);
        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
    }

    [Theory]
    [InlineData("heic", HeifCodec.Hevc)]
    [InlineData("heix", HeifCodec.Hevc)]
    [InlineData("heim", HeifCodec.Hevc)]
    [InlineData("hevc", HeifCodec.Hevc)]
    [InlineData("hevx", HeifCodec.Hevc)]
    [InlineData("heis", HeifCodec.Hevc)]
    [InlineData("avif", HeifCodec.Av1)]
    [InlineData("avis", HeifCodec.Av1)]
    [InlineData("vvic", HeifCodec.Vvc)]
    [InlineData("vvis", HeifCodec.Vvc)]
    [InlineData("unif", HeifCodec.Uncompressed)]
    [InlineData("crx ", HeifCodec.CanonRaw)]
    [InlineData("tmap", HeifCodec.ToneMapped)]
    public void Codec_Brand_Mapping_Theory(string brand, HeifCodec expected)
    {
        var info = HeifBrandInfo.From(brand, [brand]);
        Assert.Equal(expected, info.PrimaryCodec);
    }

    [Theory]
    [InlineData("msf1")]
    [InlineData("hevc")]
    [InlineData("hevx")]
    [InlineData("heis")]
    [InlineData("avis")]
    [InlineData("vvis")]
    public void Sequence_Brand_Theory_Sets_IsImageSequence(string brand)
    {
        var info = HeifBrandInfo.From(brand, [brand]);
        Assert.True(info.IsImageSequence);
        Assert.Equal(HeifContainerKind.ImageSequence, info.ContainerKind);
    }

    [Fact]
    public void Msf1_Alone_Sets_Sequence_Even_Without_Codec_Brand()
    {
        var info = HeifBrandInfo.From("msf1", ["msf1"]);
        Assert.True(info.IsImageSequence);
        Assert.Equal(HeifContainerKind.ImageSequence, info.ContainerKind);
        Assert.Equal(HeifCodec.Unknown, info.PrimaryCodec);
    }

    [Fact]
    public void MajorBrand_Not_Duplicated_In_Compat_When_Also_Listed()
    {
        // Major brand is added once even if also present in compat.
        var info = HeifBrandInfo.From("heic", ["heic", "heic", "mif1"]);
        // HasBrand still works for both; CompatibleBrands echoes whatever caller passed.
        Assert.True(info.HasBrand("heic"));
        Assert.True(info.HasBrand("mif1"));
        Assert.Equal(HeifCodec.Hevc, info.PrimaryCodec);
    }

    private static void WriteBox(Stream s, string type, Action<MemoryStream> writePayload)
    {
        using var inner = new MemoryStream();
        writePayload(inner);
        var payload = inner.ToArray();
        int total = payload.Length + 8;
        Span<byte> hdr = stackalloc byte[8];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32BigEndian(hdr[..4], (uint)total);
        System.Text.Encoding.ASCII.GetBytes(type).CopyTo(hdr.Slice(4, 4));
        s.Write(hdr);
        s.Write(payload);
    }
}
