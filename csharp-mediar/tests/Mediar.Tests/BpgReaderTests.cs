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
        WriteUe7(ms, 0);  // picture_data_length
        return ms.ToArray();
    }

    private static void WriteUe7(Stream s, uint v)
    {
        // Encode in 1..5 7-bit big-endian groups.
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
}
