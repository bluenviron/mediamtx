using Mediar.Imaging;
using Mediar.Imaging.Bpg;
using Xunit;

namespace Mediar.Tests;

public class BpgReaderTests
{
    [Fact]
    public void Parses_Bpg_Header_Without_Extensions()
    {
        byte[] file = BuildBpg(width: 320, height: 240, pixelFormat: 3 /* 4:4:4 */,
                                alpha1: false, bitDepth: 8, colorSpace: 1, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);

        Assert.Equal(ImageFormat.Bpg, r.Format);
        Assert.Equal(320, r.Info.Width);
        Assert.Equal(240, r.Info.Height);
        Assert.Equal(8, r.BitDepth);
        Assert.False(r.HasAlphaChannel);
        Assert.Equal(1, r.ColorSpaceCode);
        Assert.Empty(r.Extensions);
    }

    [Fact]
    public void Rejects_Non_Bpg_Bytes()
    {
        Assert.Throws<ImageFormatException>(() =>
            BpgReader.Open(new MemoryStream(new byte[] { 0, 0, 0, 0, 0, 0 }), ownsStream: true));
    }

    [Fact]
    public async Task ReadFramesAsync_Throws()
    {
        byte[] file = BuildBpg(8, 8, 0, false, 8, 0, false, false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    [Fact]
    public async Task ReadFramesAsync_Message_Mentions_Hevc()
    {
        byte[] file = BuildBpg(8, 8, 0, false, 8, 0, false, false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        var ex = await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
        Assert.Contains("HEVC", ex.Message);
    }

    [Fact]
    public void Open_Stream_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => BpgReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Path_Works()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-bpg-{Guid.NewGuid():N}.bpg");
        File.WriteAllBytes(path, BuildBpg(64, 48, 3, false, 8, 1, false, false));
        try
        {
            using var r = BpgReader.Open(path);
            Assert.Equal(64, r.Info.Width);
            Assert.Equal(48, r.Info.Height);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-bpg-missing-{Guid.NewGuid():N}.bpg");
        Assert.Throws<FileNotFoundException>(() => BpgReader.Open(path));
    }

    [Fact]
    public void Rejects_Stream_Too_Short()
    {
        Assert.Throws<ImageFormatException>(() =>
            BpgReader.Open(new MemoryStream(new byte[] { (byte)'B', (byte)'P', (byte)'G', 0xFB, 0 }), ownsStream: true));
    }

    [Theory]
    [InlineData((byte)'X', (byte)'P', (byte)'G', (byte)0xFB)]
    [InlineData((byte)'B', (byte)'X', (byte)'G', (byte)0xFB)]
    [InlineData((byte)'B', (byte)'P', (byte)'X', (byte)0xFB)]
    [InlineData((byte)'B', (byte)'P', (byte)'G', (byte)0x00)]
    public void Rejects_Wrong_Signature(byte b0, byte b1, byte b2, byte b3)
    {
        byte[] bad = new byte[] { b0, b1, b2, b3, 0, 0, 0, 0 };
        Assert.Throws<ImageFormatException>(() => BpgReader.Open(new MemoryStream(bad), ownsStream: true));
    }

    [Theory]
    [InlineData(0, 1)]  // Grayscale → 1 channel
    [InlineData(1, 3)]  // 4:2:0 → 3 channels
    [InlineData(2, 3)]  // 4:2:2 → 3 channels
    [InlineData(3, 3)]  // 4:4:4 → 3 channels
    public void PixelFormat_Drives_ChannelCount(int pf, int expectedChannels)
    {
        byte[] file = BuildBpg(16, 16, pf, alpha1: false, bitDepth: 8, colorSpace: 0, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(pf, r.PixelFormatCode);
        Assert.Equal(expectedChannels, r.Info.ChannelCount);
    }

    [Theory]
    [InlineData(8)]
    [InlineData(10)]
    [InlineData(12)]
    [InlineData(14)]
    public void BitDepth_Roundtrips(int bd)
    {
        byte[] file = BuildBpg(16, 16, 3, false, bd, 1, false, false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(bd, r.BitDepth);
    }

    [Fact]
    public void BitsPerPixel_Grayscale_Has_No_Triple()
    {
        byte[] file = BuildBpg(16, 16, 0 /* Gray */, alpha1: false, bitDepth: 10, colorSpace: 0, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(10, r.Info.BitsPerPixel); // bd*1 + 0
    }

    [Fact]
    public void BitsPerPixel_Color_Multiplies_By_Three()
    {
        byte[] file = BuildBpg(16, 16, 3 /* 4:4:4 */, alpha1: false, bitDepth: 10, colorSpace: 1, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(30, r.Info.BitsPerPixel); // 10*3
    }

    [Fact]
    public void Alpha1_Flag_Adds_BitDepth_To_BitsPerPixel()
    {
        byte[] file = BuildBpg(16, 16, 3, alpha1: true, bitDepth: 8, colorSpace: 1, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.True(r.HasAlphaChannel);
        Assert.True(r.Info.HasAlpha);
        Assert.Equal(32, r.Info.BitsPerPixel); // 8*3 + 8
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    [InlineData(5)]
    public void ColorSpace_Code_Is_Preserved(int cs)
    {
        byte[] file = BuildBpg(16, 16, 3, false, 8, cs, false, false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(cs, r.ColorSpaceCode);
    }

    [Fact]
    public void Animated_Flag_Sets_Info_And_Zero_FrameCount()
    {
        byte[] file = BuildBpg(16, 16, 3, false, 8, 1, hasExt: false, animated: true);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.True(r.IsAnimated);
        Assert.True(r.Info.IsAnimated);
        Assert.Equal(0, r.Info.FrameCount);
    }

    [Fact]
    public void Static_Image_Has_Frame_Count_One()
    {
        byte[] file = BuildBpg(16, 16, 3, false, 8, 1, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.False(r.IsAnimated);
        Assert.Equal(1, r.Info.FrameCount);
    }

    [Fact]
    public void Format_Is_Bpg()
    {
        byte[] file = BuildBpg(16, 16, 3, false, 8, 1, false, false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(ImageFormat.Bpg, r.Format);
        Assert.Equal(ImageFormat.Bpg, r.Info.Format);
    }

    [Fact]
    public void Cannot_Decode_Pixels()
    {
        byte[] file = BuildBpg(16, 16, 3, false, 8, 1, false, false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Metadata_Is_Empty()
    {
        byte[] file = BuildBpg(16, 16, 3, false, 8, 1, false, false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Same(ImageMetadata.Empty, r.Metadata);
    }

    [Theory]
    [InlineData(1, "Exif")]
    [InlineData(2, "ICC")]
    [InlineData(3, "Xmp")]
    [InlineData(4, "Thumbnail")]
    [InlineData(5, "Animation")]
    public void Known_Extension_Tag_Is_Mapped(int tag, string expectedType)
    {
        byte[] file = BuildBpgWithExtensions(new (int Tag, byte[] Data)[] { (tag, new byte[] { 0xAA, 0xBB, 0xCC }) });
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Single(r.Extensions);
        Assert.Equal(expectedType, r.Extensions[0].Type);
        Assert.Equal(tag, r.Extensions[0].Tag);
        Assert.Equal(new byte[] { 0xAA, 0xBB, 0xCC }, r.Extensions[0].Data);
    }

    [Fact]
    public void Unknown_Extension_Tag_Uses_TagN_Naming()
    {
        byte[] file = BuildBpgWithExtensions(new (int Tag, byte[] Data)[] { (42, new byte[] { 1, 2, 3 }) });
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Single(r.Extensions);
        Assert.Equal("Tag42", r.Extensions[0].Type);
        Assert.Equal(42, r.Extensions[0].Tag);
    }

    [Fact]
    public void Multiple_Extensions_Are_Decoded_In_Order()
    {
        byte[] file = BuildBpgWithExtensions(new (int Tag, byte[] Data)[]
        {
            (1, new byte[] { 0x11 }),
            (2, new byte[] { 0x22, 0x22 }),
            (3, new byte[] { 0x33, 0x33, 0x33 }),
        });
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(3, r.Extensions.Length);
        Assert.Equal("Exif", r.Extensions[0].Type);
        Assert.Equal("ICC", r.Extensions[1].Type);
        Assert.Equal("Xmp", r.Extensions[2].Type);
    }

    [Fact]
    public void Hevc_Codestream_Offset_Advances_Past_Header_When_No_Extensions()
    {
        byte[] file = BuildBpg(width: 320, height: 240, pixelFormat: 3,
                                alpha1: false, bitDepth: 8, colorSpace: 1, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(file.Length, r.HevcCodestreamOffset);
    }

    [Fact]
    public void Hevc_Codestream_Offset_Skips_Extensions_Block()
    {
        byte[] file = BuildBpgWithExtensions(new (int Tag, byte[] Data)[] { (1, new byte[] { 0xAA, 0xBB }) });
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(file.Length, r.HevcCodestreamOffset);
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var r = BpgReader.Open(new MemoryStream(BuildBpg(16, 16, 3, false, 8, 1, false, false)), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Dispose_With_OwnsStream_True_Closes_Underlying()
    {
        var s = new MemoryStream(BuildBpg(16, 16, 3, false, 8, 1, false, false));
        var r = BpgReader.Open(s, ownsStream: true);
        r.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = s.Length);
    }

    [Fact]
    public void Dispose_With_OwnsStream_False_Leaves_Open()
    {
        var s = new MemoryStream(BuildBpg(16, 16, 3, false, 8, 1, false, false));
        var r = BpgReader.Open(s, ownsStream: false);
        r.Dispose();
        _ = s.Length;
        s.Dispose();
    }

    [Fact]
    public void BpgExtension_Record_Exposes_Properties()
    {
        var ext = new BpgExtension("Exif", 1, new byte[] { 0xAA });
        Assert.Equal("Exif", ext.Type);
        Assert.Equal(1, ext.Tag);
        Assert.Equal(new byte[] { 0xAA }, ext.Data);
    }

    [Fact]
    public void Large_Dimensions_Use_Multi_Byte_Ue7()
    {
        byte[] file = BuildBpg(width: 4096, height: 8192, pixelFormat: 3,
                                alpha1: false, bitDepth: 8, colorSpace: 1, hasExt: false, animated: false);
        using var r = BpgReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(4096, r.Info.Width);
        Assert.Equal(8192, r.Info.Height);
    }

    // -------------------- helpers --------------------

    private static byte[] BuildBpg(int width, int height, int pixelFormat, bool alpha1,
                                    int bitDepth, int colorSpace, bool hasExt, bool animated)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'B', (byte)'P', (byte)'G', 0xFB });
        byte b4 = (byte)(((pixelFormat & 7) << 5) | (alpha1 ? 0x10 : 0) | ((bitDepth - 8) & 0xF));
        ms.WriteByte(b4);
        byte b5 = (byte)(((colorSpace & 0xF) << 4) | (hasExt ? 0x08 : 0) | (animated ? 0x01 : 0));
        ms.WriteByte(b5);
        WriteUe7(ms, (uint)width);
        WriteUe7(ms, (uint)height);
        WriteUe7(ms, 0);
        return ms.ToArray();
    }

    private static byte[] BuildBpgWithExtensions((int Tag, byte[] Data)[] extensions)
    {
        // First serialize the extension records to compute extLen.
        using var extMs = new MemoryStream();
        foreach (var (tag, data) in extensions)
        {
            WriteUe7(extMs, (uint)tag);
            WriteUe7(extMs, (uint)data.Length);
            extMs.Write(data);
        }
        byte[] extPayload = extMs.ToArray();

        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'B', (byte)'P', (byte)'G', 0xFB });
        ms.WriteByte((byte)((3 << 5) | 0)); // pf=4:4:4, no alpha1, bd=8
        ms.WriteByte((byte)((1 << 4) | 0x08)); // cs=RGB, hasExt=1
        WriteUe7(ms, 320);
        WriteUe7(ms, 240);
        WriteUe7(ms, 0);
        WriteUe7(ms, (uint)extPayload.Length);
        ms.Write(extPayload);
        return ms.ToArray();
    }

    private static void WriteUe7(Stream s, uint v)
    {
        Span<byte> stack = stackalloc byte[5];
        int n = 0;
        do
        {
            stack[n++] = (byte)(v & 0x7F);
            v >>= 7;
        } while (v != 0);
        for (int i = n - 1; i >= 0; i--)
        {
            byte by = stack[i];
            if (i > 0) by |= 0x80;
            s.WriteByte(by);
        }
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => BpgReader.Open((string)null!));
    }
}
