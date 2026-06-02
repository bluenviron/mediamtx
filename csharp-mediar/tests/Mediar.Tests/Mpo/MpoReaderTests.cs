using Mediar.Imaging;
using Mediar.Imaging.Mpo;
using Xunit;

namespace Mediar.Tests.Mpo;

/// <summary>
/// Tests for <see cref="MpoReader"/>, covering MPF segment validation,
/// MP Index IFD parsing, MPEntry table decoding, sub-image discovery,
/// and per-sub-image JPEG decode delegation.
/// </summary>
public sealed class MpoReaderTests
{
    // Tiny 16x16 solid-red baseline JPEG (the same fixture used by
    // RafReaderTests and TiffReaderTests).
    private const string RedJpegBase64 =
        "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAQCAwMDAgQDAwMEBAQEBQkGBQUFBQsICAYJDQsNDQ0LDAwOEBQRDg8TDwwMEhgSExUWFxcXDhEZGxkWGhQWFxb/" +
        "2wBDAQQEBAUFBQoGBgoWDwwPFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhb/wAARCAAQABADASIAAhEBAxEB/8QA" +
        "HwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2Jyggk" +
        "KFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMX" +
        "Gx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAEC" +
        "AxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOE" +
        "hYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDxeiiivyk/" +
        "v4//2Q==";

    private static byte[] LoadRedJpeg() => Convert.FromBase64String(RedJpegBase64);

    [Fact]
    public void Rejects_File_Without_Jpeg_SOI()
    {
        byte[] bytes = [0x00, 0x00, 0x00, 0x00];
        Assert.Throws<ImageFormatException>(() => MpoReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Rejects_File_Without_MPF_Segment()
    {
        // A plain baseline JPEG has no MPF APP2 -> reject.
        byte[] bytes = LoadRedJpeg();
        Assert.Throws<ImageFormatException>(() => MpoReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Rejects_When_MPF_Segment_Omitted_By_Builder()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary | 0x80000000,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiFrameDisparity | 0x40000000,
                },
            ],
            OmitMpfSegment = true,
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => MpoReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Parses_MPF_Version_And_Two_SubImages()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary | 0x80000000, // DependentParent
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiFrameDisparity | 0x40000000, // DependentChild
                    DependentImage1 = 1,
                },
            ],
            OverrideVersion = "0100",
        };
        byte[] bytes = TestMpoBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var mpo = MpoReader.Open(ms);

        Assert.Equal("0100", mpo.Mpo.Version);
        Assert.Equal(2u, mpo.Mpo.NumberOfImages);
        Assert.Equal("II", mpo.Mpo.ByteOrder);
        Assert.Equal(2, mpo.SubImages.Count);

        Assert.Equal(MpoImageKind.BaselineMpPrimary, mpo.SubImages[0].Kind);
        Assert.True(mpo.SubImages[0].IsDependentParent);
        Assert.Equal(0, mpo.SubImages[0].Offset);
        Assert.True(mpo.SubImages[0].Length > 0);
        Assert.Equal(16, mpo.SubImages[0].Width);
        Assert.Equal(16, mpo.SubImages[0].Height);
        Assert.True(mpo.SubImages[0].CanDecodePixels);

        Assert.Equal(MpoImageKind.MultiFrameDisparity, mpo.SubImages[1].Kind);
        Assert.True(mpo.SubImages[1].IsDependentChild);
        Assert.True(mpo.SubImages[1].Offset > 0);
        Assert.Equal((ushort)1, mpo.SubImages[1].DependentImage1);

        Assert.Equal(ImageFormat.Mpo, mpo.Format);
        Assert.Equal(2, mpo.Info.FrameCount);
        Assert.Equal(16, mpo.Info.Width);
        Assert.Equal(16, mpo.Info.Height);
        Assert.True(mpo.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Yields_One_Frame_Per_SubImage()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiFrameDisparity,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiAngle,
                },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var mpo = MpoReader.Open(ms);

        int count = 0;
        await foreach (var frame in mpo.ReadFramesAsync())
        {
            using (frame)
            {
                Assert.Equal(16, frame.Width);
                Assert.Equal(16, frame.Height);
                Assert.Equal(PixelFormat.Rgb24, frame.PixelFormat);
            }
            count++;
        }

        Assert.Equal(3, count);
    }

    [Fact]
    public void Decodes_ImageUidList_When_Present()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiFrameDisparity,
                },
            ],
            ImageUids =
            [
                "FUJIFILM-FINEPIX-3DW3-PARENT-0001",
                "FUJIFILM-FINEPIX-3DW3-CHILD--0001",
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var mpo = MpoReader.Open(ms);

        Assert.Equal(2, mpo.Mpo.ImageUids.Count);
        Assert.Equal("FUJIFILM-FINEPIX-3DW3-PARENT-0001", mpo.Mpo.ImageUids[0]);
        Assert.Contains("MPF:ImageUID[0]", mpo.Metadata.Tags.Keys);
    }

    [Fact]
    public void Rejects_When_NumberOfImages_Disagrees_With_MPEntry_Length()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiFrameDisparity,
                },
            ],
            DeclareMismatchedImageCount = true,
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => MpoReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Records_Raw_Attribute_Bits_Correctly()
    {
        const uint Attr = 0xE0000000u | (uint)MpoImageKind.MultiAngle;
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = Attr,
                },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var mpo = MpoReader.Open(ms);

        Assert.Equal(Attr, mpo.SubImages[1].RawAttribute);
        Assert.True(mpo.SubImages[1].IsDependentParent);
        Assert.True(mpo.SubImages[1].IsDependentChild);
        Assert.True(mpo.SubImages[1].IsRepresentative);
        Assert.Equal(MpoImageKind.MultiAngle, mpo.SubImages[1].Kind);
    }

    [Fact]
    public void LargeThumbnail_Kinds_Parsed_Correctly()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.LargeThumbnailClass1,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.LargeThumbnailClass2,
                },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));

        Assert.Equal(MpoImageKind.LargeThumbnailClass1, mpo.SubImages[1].Kind);
        Assert.Equal(MpoImageKind.LargeThumbnailClass2, mpo.SubImages[2].Kind);
    }

    [Fact]
    public void MultiFramePanorama_Kind_Parsed_Correctly()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiFramePanorama,
                },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));

        Assert.Equal(MpoImageKind.MultiFramePanorama, mpo.SubImages[1].Kind);
    }

    [Fact]
    public void DependentImage2_Field_Is_Preserved()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                    DependentImage1 = 2,
                    DependentImage2 = 7,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.MultiFrameDisparity,
                },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));

        Assert.Equal((ushort)2, mpo.SubImages[0].DependentImage1);
        Assert.Equal((ushort)7, mpo.SubImages[0].DependentImage2);
    }

    [Fact]
    public void SubImage_Indexes_Increment_Sequentially_From_Zero()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.BaselineMpPrimary },
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiFrameDisparity },
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiAngle },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));

        for (int i = 0; i < mpo.SubImages.Count; i++)
        {
            Assert.Equal(i, mpo.SubImages[i].Index);
        }
    }

    [Fact]
    public void Unknown_Kind_Code_Falls_Back_To_Unknown_Enum()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = (uint)MpoImageKind.BaselineMpPrimary,
                },
                new TestMpoBuilder.MpoEntrySpec
                {
                    JpegBytes = LoadRedJpeg(),
                    Attribute = 0xAB1234u, // unknown image-kind code in low 24 bits
                },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));

        // 0xAB1234 isn't defined in the MpoImageKind enum - cast to a value
        // not in the enum is still legal but won't match any named member.
        Assert.False(Enum.IsDefined<MpoImageKind>(mpo.SubImages[1].Kind));
    }

    [Fact]
    public void Mpo_Info_Dimensions_Match_Primary_SubImage_Width_Height()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.BaselineMpPrimary },
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiFrameDisparity },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));

        Assert.Equal(mpo.SubImages[0].Width, mpo.Info.Width);
        Assert.Equal(mpo.SubImages[0].Height, mpo.Info.Height);
    }

    [Fact]
    public void First_SubImage_Offset_Is_Zero()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.BaselineMpPrimary },
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiFrameDisparity },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));

        Assert.Equal(0L, mpo.SubImages[0].Offset);
        Assert.True(mpo.SubImages[1].Offset > 0);
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => MpoReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        var spec = MinimalTwoEntrySpec();
        byte[] bytes = TestMpoBuilder.Build(spec);
        var ms = new MemoryStream(bytes, writable: false);
        using (var r = MpoReader.Open(ms, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Mpo, r.Format);
        }
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        var spec = MinimalTwoEntrySpec();
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = MpoReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Mpo, r.Format);
        }
        ms.Position = 0;
        Assert.Equal(0xFF, ms.ReadByte());
    }

    [Fact]
    public void ByteOrder_Defaults_To_II_LittleEndian()
    {
        var spec = MinimalTwoEntrySpec();
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal("II", mpo.Mpo.ByteOrder);
    }

    [Fact]
    public void Default_Version_Is_0100_When_Not_Overridden()
    {
        var spec = MinimalTwoEntrySpec();
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal("0100", mpo.Mpo.Version);
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        var spec = MinimalTwoEntrySpec();
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in mpo.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Sub_Image_Count_Matches_Builder_Entries()
    {
        var spec = new TestMpoBuilder.MpoSpec
        {
            Entries =
            [
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.BaselineMpPrimary },
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiFrameDisparity },
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiAngle },
                new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiFramePanorama },
            ],
        };
        byte[] bytes = TestMpoBuilder.Build(spec);
        using var mpo = MpoReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(4u, mpo.Mpo.NumberOfImages);
        Assert.Equal(4, mpo.SubImages.Count);
    }

    private static TestMpoBuilder.MpoSpec MinimalTwoEntrySpec() => new()
    {
        Entries =
        [
            new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.BaselineMpPrimary },
            new TestMpoBuilder.MpoEntrySpec { JpegBytes = LoadRedJpeg(), Attribute = (uint)MpoImageKind.MultiFrameDisparity },
        ],
    };

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => MpoReader.Open((string)null!));
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestMpoBuilder.Build(MinimalTwoEntrySpec());
        var r = MpoReader.Open(new MemoryStream(bytes, writable: false), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Format_Is_Mpo_And_Info_Format_Matches()
    {
        byte[] bytes = TestMpoBuilder.Build(MinimalTwoEntrySpec());
        using var r = MpoReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Mpo, r.Format);
        Assert.Equal(ImageFormat.Mpo, r.Info.Format);
    }
}
