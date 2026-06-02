using Mediar.Imaging.Ktx;
using Xunit;

namespace Mediar.Tests.Ktx;

/// <summary>
/// Tests for <see cref="DfdColorSpace"/>, the helper that translates a
/// parsed KTX2 Data Format Descriptor's colour primaries + transfer function
/// into a short human-readable <see cref="Mediar.Imaging.ImageInfo.ColorSpace"/>
/// label.
/// </summary>
public sealed class DfdColorSpaceTests
{
    [Fact]
    public void Describe_Returns_Null_When_Dfd_Is_Null()
    {
        Assert.Null(DfdColorSpace.Describe(null));
    }

    [Fact]
    public void Describe_Returns_Null_When_Both_Primaries_And_Transfer_Are_Unspecified()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.Unspecified,
            TransferFunction = KhrTransferFunction.Unspecified,
        };
        builder.AddSample(0, 8, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.NotNull(dfd);
        Assert.Null(DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_sRGB_For_Bt709_With_SRgb_Transfer()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.SRgb,
        };
        builder.AddSample(0, 8, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Equal("sRGB", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_Linear_sRGB_For_Bt709_With_Linear_Transfer()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.Linear,
        };
        builder.AddSample(0, 8, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Equal("Linear sRGB", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_BT2020_PQ_For_HDR_Combination()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.Bt2020,
            TransferFunction = KhrTransferFunction.PqEotf,
        };
        builder.AddSample(0, 10, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Equal("BT.2020 PQ", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_BT2020_HLG_For_HDR_Broadcast_Combination()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.Bt2020,
            TransferFunction = KhrTransferFunction.HlgOetf,
        };
        builder.AddSample(0, 10, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Equal("BT.2020 HLG", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_Display_P3_For_DisplayP3_With_SRgb_Transfer()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.DisplayP3,
            TransferFunction = KhrTransferFunction.SRgb,
        };
        builder.AddSample(0, 8, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Equal("Display P3", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Falls_Back_To_Primaries_Only_When_Transfer_Unknown()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.AdobeRgb,
            TransferFunction = KhrTransferFunction.Unspecified,
        };
        builder.AddSample(0, 8, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Equal("Adobe RGB", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Falls_Back_To_Transfer_Only_When_Primaries_Unknown()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = KhrColorPrimaries.Unspecified,
            TransferFunction = KhrTransferFunction.SRgb,
        };
        builder.AddSample(0, 8, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Equal("sRGB", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void IsSrgbVkFormat_Returns_True_For_Known_SRgb_VkFormats()
    {
        // VK_FORMAT_R8G8B8A8_SRGB == 43
        Assert.True(KtxFormat.IsSrgbVkFormat(43));
        // VK_FORMAT_BC1_RGBA_SRGB_BLOCK == 134
        Assert.True(KtxFormat.IsSrgbVkFormat(134));
        // VK_FORMAT_BC7_SRGB_BLOCK == 146
        Assert.True(KtxFormat.IsSrgbVkFormat(146));
        // VK_FORMAT_ETC2_R8G8B8A8_SRGB_BLOCK == 152
        Assert.True(KtxFormat.IsSrgbVkFormat(152));
    }

    [Fact]
    public void IsSrgbVkFormat_Returns_False_For_Unorm_VkFormats()
    {
        // VK_FORMAT_R8G8B8A8_UNORM == 37
        Assert.False(KtxFormat.IsSrgbVkFormat(37));
        // VK_FORMAT_BC1_RGBA_UNORM_BLOCK == 133
        Assert.False(KtxFormat.IsSrgbVkFormat(133));
        // VK_FORMAT_BC6H_UFLOAT_BLOCK == 143
        Assert.False(KtxFormat.IsSrgbVkFormat(143));
    }

    [Fact]
    public void IsSrgbGlInternalFormat_Returns_True_For_Known_SRgb_Tokens()
    {
        // GL_SRGB8
        Assert.True(KtxFormat.IsSrgbGlInternalFormat(0x8C41));
        // GL_SRGB8_ALPHA8
        Assert.True(KtxFormat.IsSrgbGlInternalFormat(0x8C43));
        // GL_COMPRESSED_SRGB_S3TC_DXT1_EXT
        Assert.True(KtxFormat.IsSrgbGlInternalFormat(0x8C4C));
        // GL_COMPRESSED_SRGB_ALPHA_BPTC_UNORM
        Assert.True(KtxFormat.IsSrgbGlInternalFormat(0x8E8D));
        // GL_COMPRESSED_SRGB8_ETC2
        Assert.True(KtxFormat.IsSrgbGlInternalFormat(0x9275));
    }

    [Fact]
    public void IsSrgbGlInternalFormat_Returns_False_For_Linear_Tokens()
    {
        // GL_RGB8
        Assert.False(KtxFormat.IsSrgbGlInternalFormat(0x8051));
        // GL_RGBA8
        Assert.False(KtxFormat.IsSrgbGlInternalFormat(0x8058));
        // GL_COMPRESSED_RGB_S3TC_DXT1_EXT
        Assert.False(KtxFormat.IsSrgbGlInternalFormat(0x83F0));
        // GL_COMPRESSED_RGB8_ETC2 (linear)
        Assert.False(KtxFormat.IsSrgbGlInternalFormat(0x9274));
    }

    [Fact]
    public void Describe_Returns_Bt601Pal_With_Linear_Transfer()
    {
        var dfd = BuildDfd(KhrColorPrimaries.Bt601Ebu, KhrTransferFunction.Linear);
        Assert.Equal("BT.601 PAL Linear", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_CieXyz_When_Transfer_Unknown()
    {
        var dfd = BuildDfd(KhrColorPrimaries.CieXyz, KhrTransferFunction.Unspecified);
        Assert.Equal("CIE XYZ", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_Aces_With_AcesCc_Transfer()
    {
        var dfd = BuildDfd(KhrColorPrimaries.Aces, KhrTransferFunction.AcesCc);
        Assert.Equal("ACES ACEScc", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_Bt2020_HLG_For_HlgEotf_Variant()
    {
        var dfd = BuildDfd(KhrColorPrimaries.Bt2020, KhrTransferFunction.HlgEotf);
        Assert.Equal("BT.2020 HLG", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void Describe_Returns_Bt2020_PQ_For_PqOetf_Variant()
    {
        var dfd = BuildDfd(KhrColorPrimaries.Bt2020, KhrTransferFunction.PqOetf);
        Assert.Equal("BT.2020 PQ", DfdColorSpace.Describe(dfd));
    }

    [Fact]
    public void DescribeBlock_Can_Be_Called_Directly()
    {
        var dfd = BuildDfd(KhrColorPrimaries.AdobeRgb, KhrTransferFunction.AdobeRgb);
        Assert.NotNull(dfd!.Basic);
        Assert.Equal("Adobe RGB Adobe RGB", DfdColorSpace.DescribeBlock(dfd.Basic!));
    }

    [Fact]
    public void Describe_Returns_Null_When_Both_Primaries_And_Transfer_Are_Unknown_Enum_Values()
    {
        // Out-of-spec enum values land in the `_ => null` branches of both
        // PrimariesName and TransferName, leaving Describe with nothing to say.
        var dfd = BuildDfd((KhrColorPrimaries)0xFE, (KhrTransferFunction)0xFE);
        Assert.Null(DfdColorSpace.Describe(dfd));
    }

    private static KtxDfd BuildDfd(KhrColorPrimaries primaries, KhrTransferFunction transfer)
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorPrimaries = primaries,
            TransferFunction = transfer,
        };
        builder.AddSample(0, 8, 0);
        var bytes = builder.Build();
        var dfd = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.NotNull(dfd);
        return dfd!;
    }
}
