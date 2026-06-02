using System.Buffers.Binary;
using Mediar.Imaging;
using Mediar.Imaging.Jxl;
using Xunit;

namespace Mediar.Tests;

public class JxlReaderTests
{
    // Bit-packing helper for the JXL SizeHeader (LSB-first within each byte).
    private static byte[] BareCodestream(IEnumerable<(int Value, int Bits)> fields)
    {
        var bytes = new List<byte> { 0xFF, 0x0A };
        int bitPos = 0;
        int current = 0;
        foreach (var (value, bits) in fields)
        {
            for (int i = 0; i < bits; i++)
            {
                int bit = (value >> i) & 1;
                current |= bit << (bitPos & 7);
                bitPos++;
                if ((bitPos & 7) == 0)
                {
                    bytes.Add((byte)current);
                    current = 0;
                }
            }
        }
        if ((bitPos & 7) != 0) bytes.Add((byte)current);
        return bytes.ToArray();
    }

    private static byte[] SmallCodestream(int heightIdx, int ratio)
        => BareCodestream(new[] { (1, 1), (heightIdx, 5), (ratio, 3) });

    private static byte[] BoxHeader(int totalSize, string type)
    {
        var hdr = new byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(hdr, (uint)totalSize);
        for (int i = 0; i < 4; i++) hdr[4 + i] = (byte)type[i];
        return hdr;
    }

    private static byte[] BoxHeaderLarge(long totalSize, string type)
    {
        var hdr = new byte[16];
        BinaryPrimitives.WriteUInt32BigEndian(hdr, 1u);
        for (int i = 0; i < 4; i++) hdr[4 + i] = (byte)type[i];
        BinaryPrimitives.WriteUInt64BigEndian(hdr.AsSpan(8), (ulong)totalSize);
        return hdr;
    }

    private static byte[] WithContainer(params (string Type, byte[] Payload)[] boxes)
    {
        var sig = new byte[]
        {
            0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
        };
        using var ms = new MemoryStream();
        ms.Write(sig);
        foreach (var (type, payload) in boxes)
        {
            ms.Write(BoxHeader(8 + payload.Length, type));
            ms.Write(payload);
        }
        return ms.ToArray();
    }

    [Fact]
    public void Parses_Bare_Codestream_Signature()
    {
        // small=1, heightIdx=0 → H=8, ratio=1 → W=H=8
        byte[] file = SmallCodestream(heightIdx: 0, ratio: 1);
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);

        Assert.Equal(ImageFormat.Jxl, r.Format);
        Assert.False(r.HasContainer);
        Assert.Equal(8, r.Info.Height);
        Assert.Equal(8, r.Info.Width);
    }

    [Theory]
    [InlineData(0, 1, 8, 8)]    // H=8, ratio=1 → W=H
    [InlineData(0, 2, 9, 8)]    // ratio=1.2
    [InlineData(0, 3, 10, 8)]   // ratio=4/3 → 10.66 → 10
    [InlineData(0, 4, 12, 8)]   // ratio=1.5
    [InlineData(0, 5, 14, 8)]   // ratio=16/9 → 14.22 → 14
    [InlineData(0, 6, 16, 8)]   // ratio=2
    [InlineData(0, 7, 24, 8)]   // ratio=3
    [InlineData(31, 1, 256, 256)] // H=(31+1)*8=256, ratio=1
    public void Small_SizeHeader_All_Ratios_Are_Decoded(int heightIdx, int ratio, int expectedW, int expectedH)
    {
        var file = SmallCodestream(heightIdx, ratio);
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(expectedW, r.Info.Width);
        Assert.Equal(expectedH, r.Info.Height);
    }

    [Fact]
    public void Large_SizeHeader_With_Explicit_Width()
    {
        // small=0, sizeClass=0 → 9 bits for H-1; height-1=99 → H=100
        // ratio=0 → explicit width; sizeClassW=0 → 9 bits for W-1; width-1=199 → W=200
        var file = BareCodestream(new[]
        {
            (0, 1), (0, 2), (99, 9), (0, 3), (0, 2), (199, 9),
        });
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(100, r.Info.Height);
        Assert.Equal(200, r.Info.Width);
    }

    [Fact]
    public void Large_SizeHeader_With_Ratio()
    {
        // small=0, sizeClass=0 → H-1 9 bits; H-1=49 → H=50
        // ratio=4 → W = (int)(H * 1.5) = 75
        var file = BareCodestream(new[]
        {
            (0, 1), (0, 2), (49, 9), (4, 3),
        });
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(50, r.Info.Height);
        Assert.Equal(75, r.Info.Width);
    }

    [Fact]
    public void Recognises_Container_Signature_And_Boxes()
    {
        var payload = SmallCodestream(0, 1);
        var file = WithContainer(("jxlc", payload));
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.True(r.HasContainer);
        Assert.Contains(r.Boxes, b => b.Type == "jxlc");
        Assert.Equal(ImageFormat.Jxl, r.Format);
    }

    [Fact]
    public void Container_With_Jxlc_Box_Sets_Codestream_Offset_And_Length()
    {
        var payload = SmallCodestream(0, 1);
        var file = WithContainer(("jxlc", payload));
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        // signature(12) + box header(8) = 20
        Assert.Equal(20, r.CodestreamOffset);
        Assert.Equal(payload.Length, r.CodestreamLength);
    }

    [Fact]
    public void Container_With_Jxlp_Box_Skips_4Byte_Partial_Index()
    {
        // jxlp: 4-byte partial-codestream index then codestream bytes.
        var bare = SmallCodestream(0, 1);
        var payload = new byte[4 + bare.Length];
        Array.Copy(bare, 0, payload, 4, bare.Length);
        var file = WithContainer(("jxlp", payload));
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.True(r.HasContainer);
        Assert.Equal(20 + 4, r.CodestreamOffset);
        Assert.Equal(payload.Length - 4, r.CodestreamLength);
        Assert.Equal(8, r.Info.Width);
    }

    [Fact]
    public void Container_With_Multiple_Boxes_First_Jxlc_Wins()
    {
        var payload = SmallCodestream(0, 1);
        var file = WithContainer(
            ("jbrd", new byte[] { 0x00, 0x01 }),
            ("jxlc", payload),
            ("jxlp", new byte[8]));
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(3, r.Boxes.Length);
        // First codestream wins; jxlp's later override doesn't apply (csLen != b.Length after jxlc).
        Assert.Equal(payload.Length, r.CodestreamLength);
    }

    [Fact]
    public void Container_With_Box_Size_Zero_Extends_To_End()
    {
        // Box with size==0 extends to end of file
        var sig = new byte[]
        {
            0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
        };
        using var ms = new MemoryStream();
        ms.Write(sig);
        var bare = SmallCodestream(0, 1);
        var hdr = BoxHeader(0, "jxlc"); // size = 0 → extend to end
        ms.Write(hdr);
        ms.Write(bare);
        var file = ms.ToArray();
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.True(r.HasContainer);
        Assert.Single(r.Boxes);
        Assert.Equal("jxlc", r.Boxes[0].Type);
    }

    [Fact]
    public void Container_With_Box_Size_One_Uses_64Bit_Large_Size()
    {
        var bare = SmallCodestream(0, 1);
        int totalSize = 16 + bare.Length; // 16-byte large header + payload
        var sig = new byte[]
        {
            0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
        };
        using var ms = new MemoryStream();
        ms.Write(sig);
        ms.Write(BoxHeaderLarge(totalSize, "jxlc"));
        ms.Write(bare);
        var file = ms.ToArray();
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Single(r.Boxes);
        Assert.Equal(12 + 16, r.CodestreamOffset);
        Assert.Equal(bare.Length, r.CodestreamLength);
    }

    [Fact]
    public void Container_With_Malformed_Box_Header_Breaks_Parsing()
    {
        // Box header with size < 8 should break out of parser, leaving no boxes.
        var sig = new byte[]
        {
            0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
        };
        using var ms = new MemoryStream();
        ms.Write(sig);
        ms.Write(BoxHeader(4, "jxlc")); // invalid: too small
        ms.Write(new byte[8]);
        var file = ms.ToArray();
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.True(r.HasContainer);
        Assert.Empty(r.Boxes);
    }

    [Fact]
    public void Container_Without_Any_Boxes_Has_Empty_Boxes()
    {
        var sig = new byte[]
        {
            0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
        };
        using var r = JxlReader.Open(new MemoryStream(sig), ownsStream: true);
        Assert.True(r.HasContainer);
        Assert.Empty(r.Boxes);
    }

    [Fact]
    public void Rejects_Bytes_Without_Signature()
    {
        Assert.Throws<ImageFormatException>(() =>
            JxlReader.Open(new MemoryStream(new byte[] { 0x12, 0x34, 0x56 }), ownsStream: true));
    }

    [Fact]
    public void Rejects_Empty_Stream()
    {
        Assert.Throws<ImageFormatException>(() =>
            JxlReader.Open(new MemoryStream(Array.Empty<byte>()), ownsStream: true));
    }

    [Fact]
    public void Rejects_Single_Byte_Stream()
    {
        Assert.Throws<ImageFormatException>(() =>
            JxlReader.Open(new MemoryStream(new byte[] { 0xFF }), ownsStream: true));
    }

    [Fact]
    public void Open_Stream_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => JxlReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Path_Works()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-jxl-{Guid.NewGuid():N}.jxl");
        File.WriteAllBytes(path, SmallCodestream(0, 1));
        try
        {
            using var r = JxlReader.Open(path);
            Assert.Equal(8, r.Info.Width);
            Assert.Equal(8, r.Info.Height);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_Throws_And_Does_Not_Leak_Handle()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-jxl-missing-{Guid.NewGuid():N}.jxl");
        Assert.Throws<FileNotFoundException>(() => JxlReader.Open(path));
    }

    [Fact]
    public void Metadata_Is_Empty()
    {
        using var r = JxlReader.Open(new MemoryStream(SmallCodestream(0, 1)), ownsStream: true);
        Assert.Same(ImageMetadata.Empty, r.Metadata);
    }

    [Fact]
    public void Cannot_Decode_Pixels()
    {
        using var r = JxlReader.Open(new MemoryStream(SmallCodestream(0, 1)), ownsStream: true);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Info_Frame_Count_Is_One()
    {
        using var r = JxlReader.Open(new MemoryStream(SmallCodestream(0, 1)), ownsStream: true);
        Assert.Equal(1, r.Info.FrameCount);
    }

    [Fact]
    public void Bare_Codestream_Codestream_Offset_Is_After_Signature()
    {
        var data = SmallCodestream(0, 1);
        using var r = JxlReader.Open(new MemoryStream(data), ownsStream: true);
        Assert.False(r.HasContainer);
        Assert.Equal(2, r.CodestreamOffset);
        Assert.Equal(data.Length - 2, r.CodestreamLength);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_NotSupportedException_With_Message()
    {
        using var r = JxlReader.Open(new MemoryStream(SmallCodestream(0, 1)), ownsStream: true);
        var ex = await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
        Assert.Contains("JPEG XL", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var r = JxlReader.Open(new MemoryStream(SmallCodestream(0, 1)), ownsStream: true);
        r.Dispose();
        r.Dispose(); // no throw
    }

    [Fact]
    public void Dispose_With_OwnsStream_True_Closes_Underlying_Stream()
    {
        var stream = new MemoryStream(SmallCodestream(0, 1));
        var r = JxlReader.Open(stream, ownsStream: true);
        r.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = stream.Length);
    }

    [Fact]
    public void Dispose_With_OwnsStream_False_Leaves_Stream_Open()
    {
        var stream = new MemoryStream(SmallCodestream(0, 1));
        var r = JxlReader.Open(stream, ownsStream: false);
        r.Dispose();
        _ = stream.Length; // still usable
        stream.Dispose();
    }

    [Fact]
    public void JxlBox_Record_Has_Expected_Members()
    {
        var b = new JxlBox("jxlc", 20, 5);
        Assert.Equal("jxlc", b.Type);
        Assert.Equal(20, b.Offset);
        Assert.Equal(5, b.PayloadLength);
        Assert.Equal(b, new JxlBox("jxlc", 20, 5));
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => JxlReader.Open((string)null!));
    }

    [Fact]
    public void Info_Format_Equals_Jxl()
    {
        byte[] file = SmallCodestream(heightIdx: 0, ratio: 1);
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(ImageFormat.Jxl, r.Info.Format);
    }
}
