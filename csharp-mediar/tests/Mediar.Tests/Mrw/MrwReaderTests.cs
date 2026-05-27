using Mediar.Imaging;
using Mediar.Imaging.Mrw;
using Mediar.Tests.Srw;
using Xunit;

namespace Mediar.Tests.Mrw;

public sealed class MrwReaderTests
{
    [Fact]
    public void Open_RejectsTruncatedHeader()
    {
        var bytes = new byte[] { 0x00, 0x4D, 0x52 };
        using var ms = new MemoryStream(bytes);
        var ex = Assert.Throws<ImageFormatException>(() => MrwReader.Open(ms));
        Assert.Contains("Truncated", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_RejectsWithoutMrmMagic()
    {
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            OverrideMagic = [(byte)'A', (byte)'B', (byte)'C', (byte)'D'],
            Prd = new TestMrwBuilder.PrdSpec(),
        });
        using var ms = new MemoryStream(bytes);
        var ex = Assert.Throws<ImageFormatException>(() => MrwReader.Open(ms));
        Assert.Contains("MRM", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_RejectsWhenEnvelopeOverrunsFile()
    {
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec(),
            OverrideEnvelopeLength = 1_000_000,
        });
        using var ms = new MemoryStream(bytes);
        var ex = Assert.Throws<ImageFormatException>(() => MrwReader.Open(ms));
        Assert.Contains("envelope", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_RejectsWithoutPrdSubBlock()
    {
        var ttw = BuildKonicaMinoltaTtw("MINOLTA CO.,LTD.", "DiMAGE A2", emitStrip: false);
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            // Prd intentionally omitted
            TtwBytes = ttw,
        });
        using var ms = new MemoryStream(bytes);
        var ex = Assert.Throws<ImageFormatException>(() => MrwReader.Open(ms));
        Assert.Contains("PRD", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_RejectsWhenSubBlockTagFirstByteIsNonZero()
    {
        var ttw = BuildKonicaMinoltaTtw("MINOLTA CO.,LTD.", "DiMAGE A2", emitStrip: false);
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec(),
            TtwBytes = ttw,
            InsertBogusSubBlockFirstByte = 0x42,
        });
        using var ms = new MemoryStream(bytes);
        var ex = Assert.Throws<ImageFormatException>(() => MrwReader.Open(ms));
        Assert.Contains("sub-block", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_RejectsWhenMakeIsNotKonicaMinolta()
    {
        var ttw = BuildKonicaMinoltaTtw("Canon Inc.", "EOS 5D", emitStrip: false);
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec(),
            TtwBytes = ttw,
        });
        using var ms = new MemoryStream(bytes);
        var ex = Assert.Throws<ImageFormatException>(() => MrwReader.Open(ms));
        Assert.Contains("Konica Minolta", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("MINOLTA CO.,LTD.")]
    [InlineData("Minolta Co., Ltd.")]
    [InlineData("KONICA MINOLTA CAMERA, INC.")]
    [InlineData("KONICA MINOLTA")]
    [InlineData("Konica Minolta Camera, Inc.")]
    public void Open_AcceptsAll_KonicaMinoltaMakeVariants(string make)
    {
        var ttw = BuildKonicaMinoltaTtw(make, "DYNAX 7D", emitStrip: false);
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec(),
            TtwBytes = ttw,
        });
        using var ms = new MemoryStream(bytes);
        using var reader = MrwReader.Open(ms);
        Assert.Equal(make, reader.Mrw.Make);
        Assert.Equal("DYNAX 7D", reader.Mrw.Model);
    }

    [Fact]
    public void Open_Parses_PrdGeometry_AndMetadata()
    {
        var ttw = BuildKonicaMinoltaTtw(
            make: "KONICA MINOLTA CAMERA, INC.",
            model: "DYNAX 7D",
            software: "v1.10",
            dateTime: "2005:07:01 12:34:56",
            artist: "Tester",
            copyright: "(C) Test",
            emitStrip: false);

        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec
            {
                VersionNumber = "27WB0002",
                SensorHeight = 2160,
                SensorWidth = 3008,
                ImageHeight = 2136,
                ImageWidth = 2868,
                DataSize = 12,
                PixelSize = 12,
                StorageMethod = 0x52,
                BayerPattern = 0x01,
            },
            TtwBytes = ttw,
        });

        using var ms = new MemoryStream(bytes);
        using var reader = MrwReader.Open(ms);

        Assert.Equal(ImageFormat.Mrw, reader.Format);
        Assert.Equal("27WB0002", reader.Mrw.VersionNumber);
        Assert.Equal(2160, reader.Mrw.SensorHeight);
        Assert.Equal(3008, reader.Mrw.SensorWidth);
        Assert.Equal(2136, reader.Mrw.ImageHeight);
        Assert.Equal(2868, reader.Mrw.ImageWidth);
        Assert.Equal(12, reader.Mrw.DataSize);
        Assert.Equal(12, reader.Mrw.PixelSize);
        Assert.Equal(0x52, reader.Mrw.StorageMethod);
        Assert.Equal(0x01, reader.Mrw.BayerPattern);
        Assert.Equal("KONICA MINOLTA CAMERA, INC.", reader.Mrw.Make);
        Assert.Equal("DYNAX 7D", reader.Mrw.Model);
        Assert.Equal("v1.10", reader.Mrw.Software);
        Assert.Equal("2005:07:01 12:34:56", reader.Mrw.DateTime);
        Assert.Equal("Tester", reader.Mrw.Artist);
        Assert.Equal("(C) Test", reader.Mrw.Copyright);
    }

    [Fact]
    public void Open_Records_Wbg_And_Rif_Lengths_WhenPresent()
    {
        var ttw = BuildKonicaMinoltaTtw("MINOLTA CO.,LTD.", "DiMAGE A2", emitStrip: false);
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec(),
            TtwBytes = ttw,
            WbgBytes = new byte[] { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08 },
            RifBytes = new byte[] { 0x10, 0x11, 0x12 },
        });

        using var ms = new MemoryStream(bytes);
        using var reader = MrwReader.Open(ms);

        Assert.Equal(8, reader.Mrw.WhiteBalanceGainsLength);
        Assert.Equal(3, reader.Mrw.RawInformationFileLength);
    }

    [Fact]
    public void Open_Discovers_Cfa_SubImage_When_RawPayloadAppended()
    {
        var ttw = BuildKonicaMinoltaTtw("MINOLTA CO.,LTD.", "DiMAGE A2", emitStrip: false);
        var cfa = new byte[1024];
        for (int i = 0; i < cfa.Length; i++) cfa[i] = (byte)(i & 0xFF);

        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec
            {
                ImageWidth = 2868,
                ImageHeight = 2136,
            },
            TtwBytes = ttw,
            CfaBytes = cfa,
        });

        using var ms = new MemoryStream(bytes);
        using var reader = MrwReader.Open(ms);

        var cfaSub = reader.SubImages.Single(s => s.Kind == MrwSubImageKind.Cfa);
        Assert.Equal(1024u, cfaSub.Length);
        Assert.Equal(2868, cfaSub.Width);
        Assert.Equal(2136, cfaSub.Height);
        Assert.False(cfaSub.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_TtwEmbeddedTiff_When_StripPresent()
    {
        // 4x2 RGB strip TIFF embedded in TTW.
        var ttw = BuildKonicaMinoltaTtw("MINOLTA CO.,LTD.", "DiMAGE 7Hi", emitStrip: true);
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec(),
            TtwBytes = ttw,
        });

        using var ms = new MemoryStream(bytes);
        using var reader = MrwReader.Open(ms);
        Assert.True(reader.CanDecodePixels);

        int frames = 0;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frames++;
            Assert.Equal(4, f.Width);
            Assert.Equal(2, f.Height);
            Assert.Equal(PixelFormat.Rgb24, f.PixelFormat);
            f.Dispose();
        }
        Assert.Equal(1, frames);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_TtwNotDecodable()
    {
        // TTW with Make but no decodable strip (typical of real-world MRW metadata-only TTWs).
        var ttw = BuildKonicaMinoltaTtw("MINOLTA CO.,LTD.", "DiMAGE A2", emitStrip: false);
        var bytes = TestMrwBuilder.Build(new TestMrwBuilder.MrwSpec
        {
            Prd = new TestMrwBuilder.PrdSpec(),
            TtwBytes = ttw,
        });

        using var ms = new MemoryStream(bytes);
        using var reader = MrwReader.Open(ms);
        Assert.False(reader.CanDecodePixels);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in reader.ReadFramesAsync())
            {
            }
        });
    }

    [Fact]
    public void Detector_Recognises_MrmMagic()
    {
        ReadOnlySpan<byte> header = stackalloc byte[] { 0x00, 0x4D, 0x52, 0x4D, 0x00, 0x00, 0x00, 0x08 };
        Assert.Equal(ImageFormat.Mrw, ImageFormatDetector.Detect(header));
    }

    /// <summary>
    /// Builds a small TIFF byte stream suitable for embedding into the MRW
    /// TTW sub-block. When <paramref name="emitStrip"/> is true, the TIFF
    /// carries a real 4x2 RGB uncompressed strip that TiffReader can decode.
    /// When false the TIFF has metadata only (no usable strip dimensions),
    /// matching the typical real-world MRW TTW layout.
    /// </summary>
    private static byte[] BuildKonicaMinoltaTtw(string make, string model,
                                                bool emitStrip,
                                                string? software = null,
                                                string? dateTime = null,
                                                string? artist = null,
                                                string? copyright = null)
    {
        byte[] strip;
        int w, h, bps, spp, photometric;
        if (emitStrip)
        {
            // 4x2 RGB pixels: 24 bytes
            strip = new byte[]
            {
                0xFF, 0x00, 0x00,  0x00, 0xFF, 0x00,  0x00, 0x00, 0xFF,  0x80, 0x80, 0x80,
                0x10, 0x20, 0x30,  0x40, 0x50, 0x60,  0x70, 0x80, 0x90,  0xA0, 0xB0, 0xC0,
            };
            w = 4; h = 2; bps = 8; spp = 3; photometric = 2;
        }
        else
        {
            // 1 byte of dummy strip data; TiffReader will reject the 1x1 IFD or
            // ignore it as undecodable depending on parameters.
            strip = new byte[] { 0x00 };
            w = 0; h = 0; bps = 8; spp = 1; photometric = 1;
        }

        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = w,
            Height = h,
            BitsPerSample = bps,
            SamplesPerPixel = spp,
            Compression = 1,
            Photometric = photometric,
            NewSubFileType = 0,
            StripPayload = strip,
            Make = make,
            Model = model,
            Software = software,
            DateTime = dateTime,
            Artist = artist,
            Copyright = copyright,
        };
        return TestSrwBuilder.Build(spec);
    }
}
