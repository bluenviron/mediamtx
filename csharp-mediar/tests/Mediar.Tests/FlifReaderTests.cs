using Mediar.Imaging;
using Mediar.Imaging.Flif;
using Xunit;

namespace Mediar.Tests;

public class FlifReaderTests
{
    [Fact]
    public void Parses_Flif_Header()
    {
        // Build minimal FLIF: signature + flags byte + bit-depth char + width-1 + height-1 + frames-1
        byte[] file = BuildFlif(width: 100, height: 80, channels: 4, bitDepth8: true, frames: 1);
        using var r = FlifReader.Open(new MemoryStream(file), ownsStream: true);

        Assert.Equal(ImageFormat.Flif, r.Format);
        Assert.Equal(100, r.Info.Width);
        Assert.Equal(80, r.Info.Height);
        Assert.Equal(4, r.Channels);
        Assert.True(r.Info.HasAlpha);
        Assert.Equal(1, r.NumFrames);
    }

    [Fact]
    public void Parses_Animated_Flif()
    {
        byte[] file = BuildFlif(64, 48, 3, true, 16);
        using var r = FlifReader.Open(new MemoryStream(file), ownsStream: true);

        Assert.Equal(16, r.NumFrames);
        Assert.True(r.Info.IsAnimated);
    }

    [Fact]
    public void Rejects_Non_Flif_Bytes()
    {
        Assert.Throws<ImageFormatException>(() =>
            FlifReader.Open(new MemoryStream(new byte[] { 0, 1, 2, 3, 4, 5 }), ownsStream: true));
    }

    [Fact]
    public async Task ReadFramesAsync_Throws()
    {
        byte[] file = BuildFlif(8, 8, 3, true, 1);
        using var r = FlifReader.Open(new MemoryStream(file), ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    private static byte[] BuildFlif(int width, int height, int channels, bool bitDepth8, int frames)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F' });
        // flags: high nibble 'A' (interlaced non-animated) + low nibble channels
        byte flags = (byte)((0x40) | (channels & 0xF));
        ms.WriteByte(flags);
        ms.WriteByte((byte)(bitDepth8 ? '1' : '2'));
        WriteVarInt(ms, (uint)(width - 1));
        WriteVarInt(ms, (uint)(height - 1));
        WriteVarInt(ms, (uint)(frames - 1));
        return ms.ToArray();
    }

    private static void WriteVarInt(Stream s, uint v)
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
}
