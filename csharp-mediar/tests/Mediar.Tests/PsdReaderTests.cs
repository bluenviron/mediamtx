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

    [Fact]
    public void Open_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            PsdReader.Open((Stream)null!, ownsStream: false));
    }

    [Fact]
    public void Open_Missing_Path_Throws_FileNotFound()
    {
        string p = Path.Combine(Path.GetTempPath(),
            "missing-psd-" + Guid.NewGuid().ToString("N") + ".psd");
        Assert.Throws<FileNotFoundException>(() => PsdReader.Open(p));
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        Assert.Throws<ImageFormatException>(() =>
            PsdReader.Open(new MemoryStream(Array.Empty<byte>()), ownsStream: true));
    }

    [Fact]
    public void Stream_Too_Short_For_Header_Throws()
    {
        // Less than 26 bytes -> rejected.
        byte[] tooShort = [(byte)'8', (byte)'B', (byte)'P', (byte)'S', 0, 1];
        Assert.Throws<ImageFormatException>(() =>
            PsdReader.Open(new MemoryStream(tooShort), ownsStream: true));
    }

    [Fact]
    public void Unsupported_Version_Throws()
    {
        // 8BPS signature, version 0xABCD (not 1 or 2) -> rejected.
        var bytes = new byte[26];
        bytes[0] = (byte)'8'; bytes[1] = (byte)'B'; bytes[2] = (byte)'P'; bytes[3] = (byte)'S';
        bytes[4] = 0xAB; bytes[5] = 0xCD;
        Assert.Throws<ImageFormatException>(() =>
            PsdReader.Open(new MemoryStream(bytes), ownsStream: true));
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var psd = BuildPsd(2, 2, 3, 8, PsdColorMode.Rgb, 1);
        var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        r.Dispose();
        r.Dispose(); // must not throw
    }

    [Fact]
    public void OwnsStream_False_Keeps_Underlying_Stream_Alive()
    {
        var psd = BuildPsd(2, 2, 3, 8, PsdColorMode.Rgb, 1);
        var inner = new MemoryStream(psd, writable: false);
        var r = PsdReader.Open(inner, ownsStream: false);
        r.Dispose();
        Assert.True(inner.CanRead);
        inner.Dispose();
    }

    [Fact]
    public void Grayscale_8Bit_Single_Channel_Header()
    {
        var psd = BuildPsd(width: 4, height: 2, channels: 1, depth: 8,
                            mode: PsdColorMode.Grayscale, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(PsdColorMode.Grayscale, r.ColorMode);
        Assert.Equal(1, r.ChannelCount);
        Assert.Equal(8, r.BitDepth);
        Assert.Equal(PixelFormat.Gray8, r.Info.PixelFormat);
    }

    [Fact]
    public void Grayscale_16Bit_Single_Channel_Header()
    {
        var psd = BuildPsd(width: 4, height: 2, channels: 1, depth: 16,
                            mode: PsdColorMode.Grayscale, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(PixelFormat.Gray16, r.Info.PixelFormat);
        Assert.Equal(16, r.BitDepth);
    }

    [Fact]
    public void Rgba_8Bit_Four_Channels_Header_HasAlpha()
    {
        var psd = BuildPsd(width: 2, height: 2, channels: 4, depth: 8,
                            mode: PsdColorMode.Rgb, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(PixelFormat.Rgba32, r.Info.PixelFormat);
        Assert.True(r.Info.HasAlpha);
        Assert.Equal(4, r.ChannelCount);
    }

    [Fact]
    public void Cmyk_8Bit_Four_Channels_Header()
    {
        var psd = BuildPsd(width: 2, height: 2, channels: 4, depth: 8,
                            mode: PsdColorMode.Cmyk, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(PsdColorMode.Cmyk, r.ColorMode);
        Assert.Equal(PixelFormat.Cmyk32, r.Info.PixelFormat);
    }

    [Fact]
    public void Indexed_8Bit_Header()
    {
        var psd = BuildPsd(width: 4, height: 4, channels: 1, depth: 8,
                            mode: PsdColorMode.Indexed, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(PixelFormat.Indexed8, r.Info.PixelFormat);
    }

    [Fact]
    public void Unknown_Combination_Sets_PixelFormat_Unknown_And_CannotDecode()
    {
        // Multichannel with depth 32 -> falls into wildcard arm.
        var psd = BuildPsd(width: 2, height: 2, channels: 5, depth: 32,
                            mode: PsdColorMode.Multichannel, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(PixelFormat.Unknown, r.Info.PixelFormat);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Info_FrameCount_Is_One()
    {
        var psd = BuildPsd(width: 4, height: 4, channels: 3, depth: 8,
                            mode: PsdColorMode.Rgb, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(1, r.Info.FrameCount);
        Assert.False(r.Info.IsAnimated);
    }

    [Fact]
    public void Info_BitsPerPixel_Reflects_Depth_Times_Channels()
    {
        var psd = BuildPsd(width: 4, height: 4, channels: 4, depth: 16,
                            mode: PsdColorMode.Rgb, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(64, r.Info.BitsPerPixel);
    }

    [Fact]
    public async Task ReadFramesAsync_Honours_Cancellation_Token()
    {
        var psd = BuildPsd(2, 2, 3, 8, PsdColorMode.Rgb, 1, compression: 0,
            channelData: [
                [0, 0, 0, 0],
                [0, 0, 0, 0],
                [0, 0, 0, 0],
            ]);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token))
            {
                f.Dispose();
            }
        });
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

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => PsdReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        var psd = BuildPsd(width: 2, height: 2, channels: 3, depth: 8, mode: PsdColorMode.Rgb, version: 1);
        var ms = new MemoryStream(psd);
        using (var r = PsdReader.Open(ms, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Psd, r.Format);
        }
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        var psd = BuildPsd(width: 2, height: 2, channels: 3, depth: 8, mode: PsdColorMode.Rgb, version: 1);
        using var ms = new MemoryStream(psd);
        using (var r = PsdReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Psd, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'8', (byte)ms.ReadByte());
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        var psd = BuildPsd(width: 2, height: 2, channels: 3, depth: 8, mode: PsdColorMode.Rgb, version: 1);
        var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Info_Format_Equals_Psd()
    {
        var psd = BuildPsd(width: 2, height: 2, channels: 3, depth: 8, mode: PsdColorMode.Rgb, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        Assert.Equal(ImageFormat.Psd, r.Info.Format);
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        var psd = BuildPsd(width: 2, height: 2, channels: 3, depth: 8, mode: PsdColorMode.Rgb, version: 1);
        using var r = PsdReader.Open(new MemoryStream(psd), ownsStream: true);
        if (!r.CanDecodePixels) return;
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
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

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => PsdReader.Open((string)null!));
    }
}
