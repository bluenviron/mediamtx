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
}
