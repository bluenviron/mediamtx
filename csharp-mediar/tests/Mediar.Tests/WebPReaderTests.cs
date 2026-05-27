using System.Buffers.Binary;
using Mediar.Imaging;
using Mediar.Imaging.WebP;
using Xunit;

namespace Mediar.Tests;

public class WebPReaderTests
{
    [Fact]
    public void Recognises_Riff_Webp_Signature_And_Vp8L_Dimensions()
    {
        var bytes = BuildSimpleVp8LContainer(width: 4, height: 3);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);

        Assert.Equal(ImageFormat.WebP, r.Format);
        Assert.Equal(4, r.Info.Width);
        Assert.Equal(3, r.Info.Height);
        Assert.Contains(r.Chunks, c => c.FourCC == "VP8L");
    }

    [Fact]
    public void Parses_Vp8X_Header_For_Animated_Image()
    {
        var bytes = BuildAnimatedVp8XContainer(width: 16, height: 12, frames: 2);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);

        Assert.Equal(16, r.Info.Width);
        Assert.Equal(12, r.Info.Height);
        Assert.True(r.Info.IsAnimated);
        Assert.Equal(2, r.Info.FrameCount);
    }

    [Fact]
    public void Rejects_Non_Riff_Bytes()
    {
        Assert.Throws<ImageFormatException>(() =>
            WebPReader.Open(new MemoryStream(new byte[] { 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12 }), ownsStream: true));
    }

    [Fact]
    public async Task Vp8_Lossy_Frames_Throw_On_Pixel_Decode()
    {
        var bytes = BuildVp8LossyContainer(width: 8, height: 6);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    private static byte[] BuildSimpleVp8LContainer(int width, int height)
    {
        // Minimal VP8L payload: signature + header. Pixel data is single-symbol Huffman
        // so the actual pixel decode would produce solid black; we test the container parse.
        var vp8l = new List<byte> { 0x2F };  // VP8L signature
        // 14b width-1 + 14b height-1 + 1b alpha + 3b version, LSB-first packed
        uint hdr = (uint)((width - 1) & 0x3FFF) | (((uint)(height - 1) & 0x3FFF) << 14);
        vp8l.Add((byte)(hdr & 0xFF));
        vp8l.Add((byte)((hdr >> 8) & 0xFF));
        vp8l.Add((byte)((hdr >> 16) & 0xFF));
        vp8l.Add((byte)((hdr >> 24) & 0xFF));

        return BuildRiffWebp(new[] { ("VP8L", vp8l.ToArray()) });
    }

    private static byte[] BuildAnimatedVp8XContainer(int width, int height, int frames)
    {
        var vp8x = new byte[10];
        vp8x[0] = 0x02;  // animation flag
        vp8x[4] = (byte)((width - 1) & 0xFF);
        vp8x[5] = (byte)(((width - 1) >> 8) & 0xFF);
        vp8x[6] = (byte)(((width - 1) >> 16) & 0xFF);
        vp8x[7] = (byte)((height - 1) & 0xFF);
        vp8x[8] = (byte)(((height - 1) >> 8) & 0xFF);
        vp8x[9] = (byte)(((height - 1) >> 16) & 0xFF);

        var anim = new byte[6];

        var chunks = new List<(string, byte[])>
        {
            ("VP8X", vp8x),
            ("ANIM", anim),
        };
        for (int i = 0; i < frames; i++)
        {
            chunks.Add(("ANMF", BuildAnmf(width, height)));
        }
        return BuildRiffWebp(chunks);
    }

    private static byte[] BuildAnmf(int width, int height)
    {
        var data = new byte[16 + 5];
        // 3 bytes x>>1, 3 bytes y>>1, 3 bytes width-1, 3 bytes height-1, 3 bytes duration, flags
        data[6] = (byte)((width - 1) & 0xFF);
        data[7] = (byte)(((width - 1) >> 8) & 0xFF);
        data[9] = (byte)((height - 1) & 0xFF);
        data[10] = (byte)(((height - 1) >> 8) & 0xFF);
        data[12] = 100;  // 100ms duration
        // sub-chunk VP8L (5 bytes payload)
        data[16] = (byte)'V';
        data[17] = (byte)'P';
        data[18] = (byte)'8';
        data[19] = (byte)'L';
        return data;
    }

    private static byte[] BuildVp8LossyContainer(int width, int height)
    {
        // VP8 keyframe header: 3-byte frame tag + 3-byte start magic + 14b w + 14b h
        var vp8 = new byte[3 + 3 + 4];
        vp8[3] = 0x9D; vp8[4] = 0x01; vp8[5] = 0x2A;
        BinaryPrimitives.WriteUInt16LittleEndian(vp8.AsSpan(6), (ushort)(width & 0x3FFF));
        BinaryPrimitives.WriteUInt16LittleEndian(vp8.AsSpan(8), (ushort)(height & 0x3FFF));
        return BuildRiffWebp(new[] { ("VP8 ", vp8) });
    }

    private static byte[] BuildRiffWebp(IEnumerable<(string FourCC, byte[] Data)> chunks)
    {
        using var ms = new MemoryStream();
        ms.WriteByte((byte)'R'); ms.WriteByte((byte)'I'); ms.WriteByte((byte)'F'); ms.WriteByte((byte)'F');
        long sizeSlot = ms.Position;
        ms.Write([0, 0, 0, 0]);
        ms.WriteByte((byte)'W'); ms.WriteByte((byte)'E'); ms.WriteByte((byte)'B'); ms.WriteByte((byte)'P');
        Span<byte> len = stackalloc byte[4];
        foreach (var (fcc, data) in chunks)
        {
            ms.WriteByte((byte)fcc[0]);
            ms.WriteByte((byte)fcc[1]);
            ms.WriteByte((byte)fcc[2]);
            ms.WriteByte((byte)fcc[3]);
            BinaryPrimitives.WriteUInt32LittleEndian(len, (uint)data.Length);
            ms.Write(len);
            ms.Write(data);
            if ((data.Length & 1) == 1) ms.WriteByte(0);
        }
        long fileEnd = ms.Position;
        ms.Position = sizeSlot;
        Span<byte> sz = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(sz, (uint)(fileEnd - 8));
        ms.Write(sz);
        return ms.ToArray();
    }
}
