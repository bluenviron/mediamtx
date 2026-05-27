using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Psd;
using Xunit;

namespace Mediar.Tests;

public class PsdReaderTests
{
    [Fact]
    public void Parses_Rgb_8Bit_Header()
    {
        var psd = BuildPsd(width: 4, height: 3, channels: 3, depth: 8, mode: PsdColorMode.Rgb, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);

        Assert.Equal(ImageFormat.Psd, r.Format);
        Assert.Equal(4, r.Info.Width);
        Assert.Equal(3, r.Info.Height);
        Assert.Equal(PsdColorMode.Rgb, r.ColorMode);
        Assert.Equal(8, r.BitDepth);
        Assert.Equal(3, r.ChannelCount);
        Assert.False(r.IsPsb);
    }

    [Fact]
    public void Detects_Psb_Version_2()
    {
        var psb = BuildPsd(width: 8, height: 6, channels: 4, depth: 16, mode: PsdColorMode.Rgb, version: 2);
        using var r = PsdReader.Open(new MemoryStream(psb), ownsStream: true);

        Assert.Equal(ImageFormat.Psb, r.Format);
        Assert.True(r.IsPsb);
        Assert.Equal(16, r.BitDepth);
        Assert.Equal(4, r.ChannelCount);
        Assert.Equal(PixelFormat.Rgba64, r.Info.PixelFormat);
    }

    [Fact]
    public async Task Decodes_Raw_Rgb_8bit_Composite()
    {
        // Build a 2x2 RGB image with planar channels: R, G, B
        var psd = BuildPsd(2, 2, 3, 8, PsdColorMode.Rgb, 1, compression: 0,
            channelData: [
                [0xFF, 0x00, 0xFF, 0x00],  // R plane
                [0x00, 0xFF, 0x00, 0xFF],  // G plane
                [0x80, 0x80, 0x80, 0x80],  // B plane
            ]);

        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.True(r.CanDecodePixels);

        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync()) frames.Add(f);
        Assert.Single(frames);
        var px = frames[0].Pixels.Span;
        // Pixel 0: R=0xFF G=0x00 B=0x80
        Assert.Equal(0xFF, px[0]); Assert.Equal(0x00, px[1]); Assert.Equal(0x80, px[2]);
        // Pixel 1: R=0x00 G=0xFF B=0x80
        Assert.Equal(0x00, px[3]); Assert.Equal(0xFF, px[4]); Assert.Equal(0x80, px[5]);
        frames[0].Dispose();
    }

    [Fact]
    public void Rejects_Non_Psd_Bytes()
    {
        Assert.Throws<ImageFormatException>(() =>
            PsdReader.Open(new MemoryStream(new byte[] { 1, 2, 3, 4, 5, 6, 7, 8 }), ownsStream: true));
    }

    [Fact]
    public void Extracts_Iptc_Caption_From_Image_Resources()
    {
        var iptcBlock = BuildIptcBlock(byline: "Alice", caption: "Test photo");
        var resources = WrapImageResource(id: 1028, name: "", data: iptcBlock);
        var psd = BuildPsd(2, 2, 3, 8, PsdColorMode.Rgb, 1, compression: 0,
            channelData: [
                [0, 0, 0, 0],
                [0, 0, 0, 0],
                [0, 0, 0, 0],
            ],
            imageResources: resources);

        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal("Alice", r.Metadata.Author);
        Assert.Equal("Test photo", r.Metadata.Description);
    }

    private static byte[] BuildIptcBlock(string byline, string caption)
    {
        using var ms = new MemoryStream();
        // 2:80 By-line
        WriteIptcRecord(ms, 2, 80, Encoding.UTF8.GetBytes(byline));
        // 2:120 Caption
        WriteIptcRecord(ms, 2, 120, Encoding.UTF8.GetBytes(caption));
        return ms.ToArray();
    }

    private static void WriteIptcRecord(MemoryStream ms, byte record, byte dataset, byte[] data)
    {
        ms.WriteByte(0x1C);
        ms.WriteByte(record);
        ms.WriteByte(dataset);
        ms.WriteByte((byte)((data.Length >> 8) & 0xFF));
        ms.WriteByte((byte)(data.Length & 0xFF));
        ms.Write(data);
    }

    private static byte[] WrapImageResource(ushort id, string name, byte[] data)
    {
        using var ms = new MemoryStream();
        ms.Write([(byte)'8', (byte)'B', (byte)'I', (byte)'M']);
        Span<byte> idBuf = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(idBuf, id);
        ms.Write(idBuf);
        byte[] nameBytes = Encoding.ASCII.GetBytes(name);
        ms.WriteByte((byte)nameBytes.Length);
        ms.Write(nameBytes);
        if ((nameBytes.Length + 1) % 2 != 0) ms.WriteByte(0);
        Span<byte> lenBuf = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(lenBuf, (uint)data.Length);
        ms.Write(lenBuf);
        ms.Write(data);
        if ((data.Length & 1) == 1) ms.WriteByte(0);
        return ms.ToArray();
    }

    private static byte[] BuildPsd(int width, int height, int channels, int depth, PsdColorMode mode,
                                    int version, int compression = 0, byte[][]? channelData = null,
                                    byte[]? imageResources = null)
    {
        using var ms = new MemoryStream();
        ms.Write([(byte)'8', (byte)'B', (byte)'P', (byte)'S']);
        Span<byte> verBuf = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(verBuf, (ushort)version);
        ms.Write(verBuf);
        ms.Write(new byte[6]);
        Span<byte> chBuf = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(chBuf, (ushort)channels);
        ms.Write(chBuf);
        Span<byte> wBuf = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(wBuf, (uint)height);
        ms.Write(wBuf);
        BinaryPrimitives.WriteUInt32BigEndian(wBuf, (uint)width);
        ms.Write(wBuf);
        Span<byte> dBuf = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(dBuf, (ushort)depth);
        ms.Write(dBuf);
        Span<byte> mBuf = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(mBuf, (ushort)mode);
        ms.Write(mBuf);

        // Color mode data (length 0)
        ms.Write(new byte[4]);

        // Image resources
        Span<byte> resLen = stackalloc byte[4];
        if (imageResources is not null)
        {
            BinaryPrimitives.WriteUInt32BigEndian(resLen, (uint)imageResources.Length);
            ms.Write(resLen);
            ms.Write(imageResources);
        }
        else
        {
            BinaryPrimitives.WriteUInt32BigEndian(resLen, 0);
            ms.Write(resLen);
        }

        // Layer and mask: length 0 (4 bytes for PSD, 8 for PSB)
        if (version == 2)
            ms.Write(new byte[8]);
        else
            ms.Write(new byte[4]);

        // Image data: 2 bytes compression
        Span<byte> cBuf = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(cBuf, (ushort)compression);
        ms.Write(cBuf);

        if (channelData is not null)
        {
            foreach (var c in channelData) ms.Write(c);
        }
        else
        {
            int bps = Math.Max(1, depth / 8);
            ms.Write(new byte[width * height * channels * bps]);
        }
        return ms.ToArray();
    }
}
