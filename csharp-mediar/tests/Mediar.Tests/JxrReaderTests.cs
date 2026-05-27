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
    public async Task ReadFramesAsync_Throws()
    {
        byte[] file = BuildJxr(16, 16);
        using var r = JxrReader.Open(new MemoryStream(file), ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    private static byte[] BuildJxr(int width, int height)
    {
        // II 0xBC 0x01, then IFD offset, then IFD with width + height tags.
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'I', (byte)'I', 0xBC, 0x01 });
        Span<byte> ifdOff = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(ifdOff, 8);
        ms.Write(ifdOff);

        // IFD at offset 8: count(2) + 4 entries(12 each)
        Span<byte> count = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(count, 4);
        ms.Write(count);
        WriteIfdEntry(ms, 0xBC80, (uint)width);   // ImageWidth
        WriteIfdEntry(ms, 0xBC81, (uint)height);  // ImageHeight
        WriteIfdEntry(ms, 0xBCC0, 0x12345678);    // ImageOffset
        WriteIfdEntry(ms, 0xBCC1, 0x1234);        // ImageByteCount
        return ms.ToArray();
    }

    private static void WriteIfdEntry(Stream s, ushort tag, uint value)
    {
        Span<byte> entry = stackalloc byte[12];
        BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(0, 2), tag);
        BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(2, 2), 4);  // type LONG
        BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(4, 4), 1);
        BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(8, 4), value);
        s.Write(entry);
    }
}
