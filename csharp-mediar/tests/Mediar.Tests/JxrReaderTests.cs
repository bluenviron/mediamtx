using System.Buffers.Binary;
using Mediar.Imaging;
using Mediar.Imaging.Jxr;
using Xunit;

namespace Mediar.Tests;

public class JxrReaderTests
{
    [Fact]
    public void Parses_Jxr_Tiff_Container_With_Width_Height_Tags()
    {
        byte[] file = BuildJxr(width: 800, height: 600);
        using var r = JxrReader.Open(new MemoryStream(file), ownsStream: true);

        Assert.Equal(ImageFormat.Jxr, r.Format);
        Assert.Equal(800, r.Info.Width);
        Assert.Equal(600, r.Info.Height);
        Assert.Equal(0x12345678u, r.ImageOffset);
        Assert.Equal(0x1234u, r.ImageByteCount);
    }

    [Fact]
    public void Rejects_Non_Jxr_Bytes()
    {
        Assert.Throws<ImageFormatException>(() =>
            JxrReader.Open(new MemoryStream(new byte[] { 0, 1, 2, 3, 4, 5, 6, 7 }), ownsStream: true));
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_NotSupported_With_Message()
    {
        byte[] file = BuildJxr(16, 16);
        using var r = JxrReader.Open(new MemoryStream(file), ownsStream: true);
        var ex = await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
        Assert.Contains("JPEG XR", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_Stream_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => JxrReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Path_Works()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-jxr-{Guid.NewGuid():N}.jxr");
        File.WriteAllBytes(path, BuildJxr(40, 30));
        try
        {
            using var r = JxrReader.Open(path);
            Assert.Equal(40, r.Info.Width);
            Assert.Equal(30, r.Info.Height);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-jxr-missing-{Guid.NewGuid():N}.jxr");
        Assert.Throws<FileNotFoundException>(() => JxrReader.Open(path));
    }

    [Fact]
    public void Rejects_Stream_Too_Short()
    {
        Assert.Throws<ImageFormatException>(() =>
            JxrReader.Open(new MemoryStream(new byte[] { (byte)'I', (byte)'I', 0xBC }), ownsStream: true));
    }

    [Theory]
    [InlineData((byte)'X', (byte)'I', 0xBC)] // wrong byte 0
    [InlineData((byte)'I', (byte)'X', 0xBC)] // wrong byte 1
    [InlineData((byte)'I', (byte)'I', 0x12)] // wrong byte 2
    public void Rejects_Wrong_Magic(byte b0, byte b1, byte b2)
    {
        byte[] bad = new byte[] { b0, b1, b2, 0x01, 0, 0, 0, 0 };
        Assert.Throws<ImageFormatException>(() => JxrReader.Open(new MemoryStream(bad), ownsStream: true));
    }

    [Fact]
    public void Format_Is_Jxr()
    {
        using var r = JxrReader.Open(new MemoryStream(BuildJxr(8, 8)), ownsStream: true);
        Assert.Equal(ImageFormat.Jxr, r.Format);
        Assert.Equal(ImageFormat.Jxr, r.Info.Format);
    }

    [Fact]
    public void Cannot_Decode_Pixels()
    {
        using var r = JxrReader.Open(new MemoryStream(BuildJxr(8, 8)), ownsStream: true);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Metadata_Is_Empty()
    {
        using var r = JxrReader.Open(new MemoryStream(BuildJxr(8, 8)), ownsStream: true);
        Assert.Same(ImageMetadata.Empty, r.Metadata);
    }

    [Fact]
    public void Info_Frame_Count_Is_One()
    {
        using var r = JxrReader.Open(new MemoryStream(BuildJxr(8, 8)), ownsStream: true);
        Assert.Equal(1, r.Info.FrameCount);
    }

    [Fact]
    public void Tags_Are_Exposed_In_Source_Order()
    {
        using var r = JxrReader.Open(new MemoryStream(BuildJxr(64, 48)), ownsStream: true);
        Assert.Equal(4, r.Tags.Length);
        Assert.Equal((ushort)0xBC80, r.Tags[0].Tag);
        Assert.Equal((ushort)0xBC81, r.Tags[1].Tag);
        Assert.Equal((ushort)0xBCC0, r.Tags[2].Tag);
        Assert.Equal((ushort)0xBCC1, r.Tags[3].Tag);
    }

    [Fact]
    public void BitsPerPixel_Tag_Is_Decoded()
    {
        var file = BuildJxrWithEntries(new (ushort Tag, uint Value)[]
        {
            (0xBC80, 100),  // width
            (0xBC81, 80),   // height
            (0xBC83, 32),   // bits per pixel
        });
        using var r = JxrReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(32, r.Info.BitsPerPixel);
    }

    [Fact]
    public void PixelFormatGuid_Is_Decoded_From_Tag_BC00()
    {
        var guid = new Guid("12345678-1234-5678-9abc-def012345678");
        var guidBytes = guid.ToByteArray();

        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'I', (byte)'I', 0xBC, 0x01 });
        Span<byte> ifdOff = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(ifdOff, 8);
        ms.Write(ifdOff);
        // IFD at 8: 1 entry → ifd ends at 8 + 2 + 12 = 22; guid lives at 22
        Span<byte> count = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(count, 1);
        ms.Write(count);

        Span<byte> entry = stackalloc byte[12];
        BinaryPrimitives.WriteUInt16LittleEndian(entry[..2], 0xBC00);     // Tag
        BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(2, 2), 1);   // Type BYTE
        BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(4, 4), 16);  // Count = 16
        BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(8, 4), 22u); // ValueOrOffset
        ms.Write(entry);
        ms.Write(guidBytes);

        using var r = JxrReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal(guid, r.PixelFormatGuid);
    }

    [Fact]
    public void PixelFormatGuid_Defaults_To_Empty_When_Absent()
    {
        using var r = JxrReader.Open(new MemoryStream(BuildJxr(8, 8)), ownsStream: true);
        Assert.Equal(Guid.Empty, r.PixelFormatGuid);
    }

    [Fact]
    public void Empty_Ifd_Yields_Zero_Dimensions_And_No_Tags()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'I', (byte)'I', 0xBC, 0x01 });
        Span<byte> ifdOff = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(ifdOff, 8);
        ms.Write(ifdOff);
        Span<byte> zero = stackalloc byte[2]; // 0 entries
        ms.Write(zero);

        using var r = JxrReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal(0, r.Info.Width);
        Assert.Equal(0, r.Info.Height);
        Assert.Empty(r.Tags);
    }

    [Fact]
    public void Ifd_Offset_Past_End_Yields_No_Tags()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'I', (byte)'I', 0xBC, 0x01 });
        Span<byte> ifdOff = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(ifdOff, 100); // file is only 8 bytes
        ms.Write(ifdOff);

        using var r = JxrReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Empty(r.Tags);
        Assert.Equal(0, r.Info.Width);
    }

    [Fact]
    public void Ifd_With_Truncated_Entry_List_Stops_Cleanly()
    {
        // Claim 4 entries but only provide bytes for 2 → parser stops at p + 12 > b.Length.
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'I', (byte)'I', 0xBC, 0x01 });
        Span<byte> ifdOff = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(ifdOff, 8);
        ms.Write(ifdOff);
        Span<byte> count = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(count, 4); // claim 4
        ms.Write(count);
        WriteIfdEntry(ms, 0xBC80, 11); // width
        WriteIfdEntry(ms, 0xBC81, 22); // height
        // ...truncated; missing 2 more entries

        using var r = JxrReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal(2, r.Tags.Length);
        Assert.Equal(11, r.Info.Width);
        Assert.Equal(22, r.Info.Height);
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var r = JxrReader.Open(new MemoryStream(BuildJxr(8, 8)), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Dispose_With_OwnsStream_True_Closes_Underlying_Stream()
    {
        var stream = new MemoryStream(BuildJxr(8, 8));
        var r = JxrReader.Open(stream, ownsStream: true);
        r.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = stream.Length);
    }

    [Fact]
    public void Dispose_With_OwnsStream_False_Leaves_Stream_Open()
    {
        var stream = new MemoryStream(BuildJxr(8, 8));
        var r = JxrReader.Open(stream, ownsStream: false);
        r.Dispose();
        _ = stream.Length;
        stream.Dispose();
    }

    [Fact]
    public void JxrTag_Record_Members_And_Equality()
    {
        var a = new JxrTag(0xBC80, 4, 1, 800);
        var b = new JxrTag(0xBC80, 4, 1, 800);
        Assert.Equal((ushort)0xBC80, a.Tag);
        Assert.Equal((ushort)4, a.Type);
        Assert.Equal(1u, a.Count);
        Assert.Equal(800u, a.ValueOrOffset);
        Assert.Equal(a, b);
    }

    // -------------------- helpers --------------------

    private static byte[] BuildJxr(int width, int height)
    {
        return BuildJxrWithEntries(new (ushort, uint)[]
        {
            (0xBC80, (uint)width),
            (0xBC81, (uint)height),
            (0xBCC0, 0x12345678),
            (0xBCC1, 0x1234),
        });
    }

    private static byte[] BuildJxrWithEntries((ushort Tag, uint Value)[] entries)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'I', (byte)'I', 0xBC, 0x01 });
        Span<byte> ifdOff = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(ifdOff, 8);
        ms.Write(ifdOff);

        Span<byte> count = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(count, (ushort)entries.Length);
        ms.Write(count);
        foreach (var (tag, value) in entries)
        {
            WriteIfdEntry(ms, tag, value);
        }
        return ms.ToArray();
    }

    private static void WriteIfdEntry(Stream s, ushort tag, uint value)
    {
        Span<byte> entry = stackalloc byte[12];
        BinaryPrimitives.WriteUInt16LittleEndian(entry[..2], tag);
        BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(2, 2), 4);  // type LONG
        BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(4, 4), 1);
        BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(8, 4), value);
        s.Write(entry);
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => JxrReader.Open((string)null!));
    }
}
