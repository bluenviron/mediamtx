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

    [Fact]
    public async Task ReadComposedFramesAsync_Falls_Through_For_Non_Animated_WebP()
    {
        var bytes = BuildSimpleVp8LContainer(width: 4, height: 3);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);

        int count = 0;
        await foreach (var f in r.ReadComposedFramesAsync())
        {
            using (f)
            {
                Assert.Equal(4, f.Width);
                Assert.Equal(3, f.Height);
                count++;
            }
        }
        Assert.Equal(1, count);
    }

    [Fact]
    public async Task ReadComposedFramesAsync_Throws_For_Animated_Vp8_Lossy_Frames()
    {
        // Built ANMF entries reference VP8 ("VP8 ") sub-chunks, which the
        // composed iterator must reject for now (no VP8 codec yet).
        var bytes = BuildAnimatedVp8XLossyContainer(width: 4, height: 4, frames: 2);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadComposedFramesAsync()) { f.Dispose(); }
        });
    }

    [Fact]
    public void Open_NullStream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => WebPReader.Open((Stream)null!, ownsStream: true));
    }

    [Fact]
    public void Open_TooShortInput_Throws_ImageFormatException()
    {
        Assert.Throws<ImageFormatException>(() =>
            WebPReader.Open(new MemoryStream(new byte[] { (byte)'R', (byte)'I' }), ownsStream: true));
    }

    [Fact]
    public void Open_RiffButWrongMagic_Throws_ImageFormatException()
    {
        // First 4 bytes "RIFF" but bytes 8..11 are not "WEBP" → reject.
        byte[] bytes = new byte[16];
        bytes[0] = (byte)'R'; bytes[1] = (byte)'I'; bytes[2] = (byte)'F'; bytes[3] = (byte)'F';
        bytes[8] = (byte)'A'; bytes[9] = (byte)'V'; bytes[10] = (byte)'I'; bytes[11] = (byte)' ';
        Assert.Throws<ImageFormatException>(() => WebPReader.Open(new MemoryStream(bytes), ownsStream: true));
    }

    [Fact]
    public void Open_FromFilePath_Works()
    {
        var bytes = BuildSimpleVp8LContainer(width: 5, height: 7);
        string tempPath = Path.Combine(Path.GetTempPath(), $"webp-test-{Guid.NewGuid():N}.webp");
        try
        {
            File.WriteAllBytes(tempPath, bytes);
            using var r = WebPReader.Open(tempPath);
            Assert.Equal(5, r.Info.Width);
            Assert.Equal(7, r.Info.Height);
            Assert.Equal(ImageFormat.WebP, r.Format);
        }
        finally
        {
            if (File.Exists(tempPath)) File.Delete(tempPath);
        }
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var bytes = BuildSimpleVp8LContainer(width: 2, height: 2);
        var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose(); // should not throw
    }

    [Fact]
    public async Task ReadFramesAsync_AfterDispose_Throws_ObjectDisposedException()
    {
        var bytes = BuildSimpleVp8LContainer(width: 2, height: 2);
        var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        await Assert.ThrowsAsync<ObjectDisposedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    [Fact]
    public async Task ReadComposedFramesAsync_AfterDispose_Throws_ObjectDisposedException()
    {
        var bytes = BuildSimpleVp8LContainer(width: 2, height: 2);
        var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        await Assert.ThrowsAsync<ObjectDisposedException>(async () =>
        {
            await foreach (var f in r.ReadComposedFramesAsync()) { f.Dispose(); }
        });
    }

    [Fact]
    public async Task ReadFramesAsync_Honours_Cancellation()
    {
        var bytes = BuildAnimatedVp8XContainer(width: 4, height: 4, frames: 3);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Animated_VP8X_Reader_Has_Anim_Chunk_And_LoopCount()
    {
        var bytes = BuildAnimatedVp8XContainer(width: 8, height: 8, frames: 1);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Contains(r.Chunks, c => c.FourCC == "ANIM");
        Assert.Contains(r.Chunks, c => c.FourCC == "VP8X");
        // Loop count = first 2 bytes after BG in ANIM; our fixture leaves all zeros.
        Assert.Equal(0, r.LoopCount);
        Assert.Equal(0u, r.BackgroundColor);
    }

    [Fact]
    public void Vp8X_HasAlpha_Flag_Reflected_In_Info()
    {
        var bytes = BuildVp8XWithAlphaContainer(width: 8, height: 8);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.HasAlpha);
        Assert.True(r.Info.HasAlpha);
        Assert.Equal(4, r.Info.ChannelCount);
    }

    [Fact]
    public void Iccp_Chunk_Populates_IccProfile()
    {
        byte[] icc = { 0xCA, 0xFE, 0xBA, 0xBE, 0x01, 0x02 };
        var bytes = BuildWithExtraChunks(width: 4, height: 4, extras: new[] { ("ICCP", icc) });
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(icc.Length, r.Info.IccProfile.Length);
        Assert.Equal(icc, r.Info.IccProfile.ToArray());
    }

    [Fact]
    public void Xmp_Chunk_Surfaces_In_Metadata_Tags()
    {
        byte[] xmp = System.Text.Encoding.UTF8.GetBytes("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\"/>");
        var bytes = BuildWithExtraChunks(width: 4, height: 4, extras: new[] { ("XMP ", xmp) });
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.Metadata.Tags.ContainsKey("XMP"));
        Assert.Contains("xmpmeta", r.Metadata.Tags["XMP"]);
    }

    [Fact]
    public void Exif_Chunk_Marks_Presence_In_Metadata_Tags()
    {
        byte[] exif = new byte[42];
        var bytes = BuildWithExtraChunks(width: 4, height: 4, extras: new[] { ("EXIF", exif) });
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.Metadata.Tags.ContainsKey("EXIF"));
        Assert.Contains("42", r.Metadata.Tags["EXIF"]);
    }

    [Fact]
    public void Vp8_Lossy_Container_CanDecodePixels_Is_False()
    {
        var bytes = BuildVp8LossyContainer(width: 4, height: 4);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Vp8L_Container_CanDecodePixels_Is_True()
    {
        var bytes = BuildSimpleVp8LContainer(width: 4, height: 4);
        using var r = WebPReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.CanDecodePixels);
    }

    private static byte[] BuildVp8XWithAlphaContainer(int width, int height)
    {
        var vp8x = new byte[10];
        vp8x[0] = 0x10; // alpha flag
        vp8x[4] = (byte)((width - 1) & 0xFF);
        vp8x[5] = (byte)(((width - 1) >> 8) & 0xFF);
        vp8x[6] = (byte)(((width - 1) >> 16) & 0xFF);
        vp8x[7] = (byte)((height - 1) & 0xFF);
        vp8x[8] = (byte)(((height - 1) >> 8) & 0xFF);
        vp8x[9] = (byte)(((height - 1) >> 16) & 0xFF);
        return BuildRiffWebp(new[] { ("VP8X", vp8x) });
    }

    private static byte[] BuildWithExtraChunks(int width, int height, IEnumerable<(string, byte[])> extras)
    {
        var vp8l = new List<byte> { 0x2F };
        uint hdr = (uint)((width - 1) & 0x3FFF) | (((uint)(height - 1) & 0x3FFF) << 14);
        vp8l.Add((byte)(hdr & 0xFF));
        vp8l.Add((byte)((hdr >> 8) & 0xFF));
        vp8l.Add((byte)((hdr >> 16) & 0xFF));
        vp8l.Add((byte)((hdr >> 24) & 0xFF));
        var chunks = new List<(string, byte[])> { ("VP8L", vp8l.ToArray()) };
        chunks.AddRange(extras);
        return BuildRiffWebp(chunks);
    }

    private static byte[] BuildAnimatedVp8XLossyContainer(int width, int height, int frames)
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
            chunks.Add(("ANMF", BuildAnmfWithVp8(width, height)));
        }
        return BuildRiffWebp(chunks);
    }

    private static byte[] BuildAnmfWithVp8(int width, int height)
    {
        var data = new byte[16 + 5];
        data[6] = (byte)((width - 1) & 0xFF);
        data[7] = (byte)(((width - 1) >> 8) & 0xFF);
        data[9] = (byte)((height - 1) & 0xFF);
        data[10] = (byte)(((height - 1) >> 8) & 0xFF);
        data[12] = 100;
        // sub-chunk VP8 (lossy) - 5 bytes payload
        data[16] = (byte)'V';
        data[17] = (byte)'P';
        data[18] = (byte)'8';
        data[19] = (byte)' ';
        return data;
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
