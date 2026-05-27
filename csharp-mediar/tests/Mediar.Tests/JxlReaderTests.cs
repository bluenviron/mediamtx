using Mediar.Imaging;
using Mediar.Imaging.Jxl;
using Xunit;

namespace Mediar.Tests;

public class JxlReaderTests
{
    [Fact]
    public void Parses_Bare_Codestream_Signature()
    {
        // Bare JXL: 0xFF 0x0A + SizeHeader. Small=1 (1 bit), height-table index 0 -> H=8, ratio=1 -> W=H=8.
        // LSB-first packing: bit0=small(1), bits1..5=height_index(0), bits6..8=ratio(1)
        // Byte after signature: small=1 in bit0, ratio bit0=1 in bit6 -> 0b01000001 = 0x41
        byte[] file = new byte[] { 0xFF, 0x0A, 0x41 };
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);

        Assert.Equal(ImageFormat.Jxl, r.Format);
        Assert.False(r.HasContainer);
        Assert.Equal(8, r.Info.Height);
        Assert.Equal(8, r.Info.Width);
    }

    [Fact]
    public void Recognises_Container_Signature_And_Boxes()
    {
        byte[] sig = new byte[]
        {
            0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A,
        };
        using var ms = new MemoryStream();
        ms.Write(sig);
        // jxlc box: 8-byte header + 3-byte payload (FF 0A 41 = bare codestream w/h)
        var payload = new byte[] { 0xFF, 0x0A, 0x41 };
        Span<byte> bh = stackalloc byte[8];
        bh[3] = (byte)(8 + payload.Length);
        bh[4] = (byte)'j'; bh[5] = (byte)'x'; bh[6] = (byte)'l'; bh[7] = (byte)'c';
        ms.Write(bh);
        ms.Write(payload);

        using var r = JxlReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.True(r.HasContainer);
        Assert.Contains(r.Boxes, b => b.Type == "jxlc");
    }

    [Fact]
    public void Rejects_Bytes_Without_Signature()
    {
        Assert.Throws<ImageFormatException>(() =>
            JxlReader.Open(new MemoryStream(new byte[] { 0x12, 0x34, 0x56 }), ownsStream: true));
    }

    [Fact]
    public async Task ReadFramesAsync_Throws()
    {
        byte[] file = new byte[] { 0xFF, 0x0A, 0x41 };
        using var r = JxlReader.Open(new MemoryStream(file), ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }
}
