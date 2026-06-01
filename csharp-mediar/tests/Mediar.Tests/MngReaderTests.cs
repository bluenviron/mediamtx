using System.Buffers.Binary;
using System.IO.Compression;
using Mediar.Imaging;
using Mediar.Imaging.Mng;
using Xunit;

namespace Mediar.Tests;

public class MngReaderTests
{
    [Fact]
    public void Parses_MHDR_And_Exposes_Embedded_Png()
    {
        var mng = BuildSimpleMng(width: 4, height: 4, frames: 1, ticks: 30);
        using var r = MngReader.Open(new MemoryStream(mng, writable: false), ownsStream: true);

        Assert.Equal(ImageFormat.Mng, r.Format);
        Assert.Equal(4, r.Info.Width);
        Assert.Equal(4, r.Info.Height);
        Assert.Equal(30, r.TicksPerSecond);
        Assert.Single(r.EmbeddedStreams);
        Assert.Equal(MngEmbeddedStreamKind.Png, r.EmbeddedStreams[0].Kind);
        Assert.True(r.CanDecodePixels);
    }

    [Fact]
    public async Task Decodes_Frames_Via_Embedded_Png()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        using var r = MngReader.Open(new MemoryStream(mng, writable: false), ownsStream: true);

        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync())
        {
            frames.Add(f);
        }

        Assert.Single(frames);
        Assert.Equal(2, frames[0].Width);
        Assert.Equal(2, frames[0].Height);
        frames[0].Dispose();
    }

    [Fact]
    public void Detects_Multiple_Embedded_Pngs()
    {
        var mng = BuildSimpleMng(width: 4, height: 4, frames: 3, ticks: 30, embedCount: 3);
        using var r = MngReader.Open(new MemoryStream(mng, writable: false), ownsStream: true);

        Assert.Equal(3, r.EmbeddedStreams.Count);
        Assert.Equal(3, r.NominalFrameCount);
        Assert.Equal(3, r.Info.FrameCount);
        Assert.True(r.Info.IsAnimated);
    }

    [Fact]
    public void Rejects_Non_Mng_Bytes()
    {
        var bytes = new byte[64];
        Assert.Throws<ImageFormatException>(() =>
            MngReader.Open(new MemoryStream(bytes), ownsStream: true));
    }

    [Fact]
    public void Open_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            MngReader.Open((Stream)null!, ownsStream: false));
    }

    [Fact]
    public void Open_Missing_Path_Throws_FileNotFound()
    {
        var path = Path.Combine(Path.GetTempPath(), "definitely-not-here-" + Guid.NewGuid().ToString("N") + ".mng");
        Assert.Throws<FileNotFoundException>(() => MngReader.Open(path));
    }

    [Fact]
    public void Empty_Stream_Throws()
    {
        // Strictly empty — signature mismatch surfaces as ImageFormatException.
        Assert.Throws<ImageFormatException>(() =>
            MngReader.Open(new MemoryStream(Array.Empty<byte>()), ownsStream: true));
    }

    [Fact]
    public void Signature_Only_Stream_Throws_For_Missing_MHDR()
    {
        // 8 magic bytes but no MHDR chunk -> reader must throw.
        byte[] just_sig = [0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];
        Assert.Throws<ImageFormatException>(() =>
            MngReader.Open(new MemoryStream(just_sig), ownsStream: true));
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        r.Dispose();
        r.Dispose(); // must not throw
    }

    [Fact]
    public void ReadFramesAsync_After_Dispose_Throws()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        r.Dispose();
        var ex = Assert.Throws<ObjectDisposedException>(() =>
        {
            // Materialise the IAsyncEnumerable to surface the disposal check.
            _ = r.ReadFramesAsync().GetAsyncEnumerator().MoveNextAsync().AsTask().GetAwaiter().GetResult();
        });
        Assert.NotNull(ex);
    }

    [Fact]
    public async Task ReadFramesAsync_Honours_Cancellation_Token()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
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

    [Fact]
    public void OwnsStream_False_Keeps_Underlying_Stream_Alive_After_Dispose()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        var inner = new MemoryStream(mng, writable: false);
        var r = MngReader.Open(inner, ownsStream: false);
        r.Dispose();
        // Stream not owned -> still readable.
        Assert.True(inner.CanRead);
        inner.Dispose();
    }

    [Fact]
    public void MHDR_Header_Fields_Surface_Through_Public_Properties()
    {
        var mng = BuildMngWithMhdr(width: 16, height: 8, ticks: 60, layers: 5,
                                   frames: 2, playTime: 1200, profile: 0x000A,
                                   embedCount: 2);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);

        Assert.Equal(60, r.TicksPerSecond);
        Assert.Equal(5, r.NominalLayerCount);
        Assert.Equal(2, r.NominalFrameCount);
        Assert.Equal(1200, r.NominalPlayTimeTicks);
        Assert.Equal(0x000Au, r.Profile);
    }

    [Fact]
    public void JNG_Substream_Is_Surfaced_But_Not_Decoded()
    {
        var mng = BuildMngWithJng(width: 8, height: 4);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        var jng = Assert.Single(r.EmbeddedStreams);
        Assert.Equal(MngEmbeddedStreamKind.Jng, jng.Kind);
        Assert.Equal(8, jng.Width);
        Assert.Equal(4, jng.Height);
        Assert.Null(jng.PngBytes);
        Assert.False(r.CanDecodePixels);
        Assert.Equal(0, r.Info.FrameCount);
    }

    [Fact]
    public async Task JNG_Only_Stream_Yields_No_ImageFrames()
    {
        var mng = BuildMngWithJng(width: 8, height: 4);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        int count = 0;
        await foreach (var f in r.ReadFramesAsync())
        {
            count++;
            f.Dispose();
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public void Embedded_Png_Bytes_Start_With_Png_Signature()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        var sub = Assert.Single(r.EmbeddedStreams);
        Assert.NotNull(sub.PngBytes);
        Assert.Equal(0x89, sub.PngBytes![0]);
        Assert.Equal((byte)'P', sub.PngBytes[1]);
        Assert.Equal((byte)'N', sub.PngBytes[2]);
        Assert.Equal((byte)'G', sub.PngBytes[3]);
    }

    [Fact]
    public void Embedded_Stream_Reports_Positive_Offset()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        var sub = Assert.Single(r.EmbeddedStreams);
        Assert.True(sub.Offset > 0, $"Expected offset > 0, got {sub.Offset}");
    }

    [Fact]
    public void Single_Frame_Mng_Is_Not_Marked_Animated()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 1, ticks: 10);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        Assert.False(r.Info.IsAnimated);
        Assert.Equal(1, r.Info.FrameCount);
    }

    [Fact]
    public void MEND_less_Stream_Still_Parses_Successfully()
    {
        // Same as BuildSimpleMng but without the trailing MEND chunk.
        using var ms = new MemoryStream();
        ms.Write([0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
        WriteChunk(ms, "MHDR", BuildMhdr(4, 4, 30, layers: 1, frames: 1, playTime: 0, profile: 0x49));
        AppendPng(ms, 4, 4);
        using var r = MngReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal(4, r.Info.Width);
        Assert.Single(r.EmbeddedStreams);
    }

    [Fact]
    public void EmbeddedStream_Width_Height_Match_IHDR()
    {
        var mng = BuildSimpleMng(width: 7, height: 5, frames: 1, ticks: 10);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        var sub = Assert.Single(r.EmbeddedStreams);
        Assert.Equal(7, sub.Width);
        Assert.Equal(5, sub.Height);
    }

    [Fact]
    public async Task Decodes_Three_Frames_From_Three_Embedded_Pngs()
    {
        var mng = BuildSimpleMng(width: 2, height: 2, frames: 3, ticks: 30, embedCount: 3);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync())
        {
            frames.Add(f);
        }
        Assert.Equal(3, frames.Count);
        foreach (var f in frames) f.Dispose();
    }

    [Fact]
    public void Format_Reports_Mng_Even_When_No_Frames()
    {
        var mng = BuildMngWithJng(width: 2, height: 2);
        using var r = MngReader.Open(new MemoryStream(mng), ownsStream: true);
        Assert.Equal(ImageFormat.Mng, r.Format);
    }

    [Fact]
    public void Metadata_Captures_tEXt_Title()
    {
        using var ms = new MemoryStream();
        ms.Write([0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
        WriteChunk(ms, "MHDR", BuildMhdr(2, 2, 30, layers: 1, frames: 1, playTime: 0, profile: 0x49));
        // tEXt: "Title\0Test Title"
        var keyAndVal = new List<byte>();
        keyAndVal.AddRange("Title"u8.ToArray());
        keyAndVal.Add(0);
        keyAndVal.AddRange("Test Title"u8.ToArray());
        WriteChunk(ms, "tEXt", keyAndVal.ToArray());
        AppendPng(ms, 2, 2);
        WriteChunk(ms, "MEND", []);

        using var r = MngReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal("Test Title", r.Metadata.Title);
        Assert.True(r.Metadata.Tags.ContainsKey("Title"));
    }

    // ---------- additional builders ----------

    private static byte[] BuildMngWithMhdr(int width, int height, int ticks, int layers,
                                           int frames, int playTime, uint profile, int embedCount)
    {
        using var ms = new MemoryStream();
        ms.Write([0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
        WriteChunk(ms, "MHDR", BuildMhdr(width, height, ticks, layers, frames, playTime, profile));
        for (int i = 0; i < embedCount; i++) AppendPng(ms, width, height);
        WriteChunk(ms, "MEND", []);
        return ms.ToArray();
    }

    private static byte[] BuildMngWithJng(int width, int height)
    {
        using var ms = new MemoryStream();
        ms.Write([0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
        WriteChunk(ms, "MHDR", BuildMhdr(width, height, 30, layers: 1, frames: 1, playTime: 0, profile: 0x49));
        // JHDR: only need 8 bytes (width + height); fill the rest as zeros.
        var jhdr = new byte[16];
        BinaryPrimitives.WriteUInt32BigEndian(jhdr.AsSpan(0), (uint)width);
        BinaryPrimitives.WriteUInt32BigEndian(jhdr.AsSpan(4), (uint)height);
        WriteChunk(ms, "JHDR", jhdr);
        WriteChunk(ms, "MEND", []);
        return ms.ToArray();
    }

    private static byte[] BuildSimpleMng(int width, int height, int frames, int ticks, int embedCount = 1)
    {
        using var ms = new MemoryStream();
        ms.Write([0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
        WriteChunk(ms, "MHDR", BuildMhdr(width, height, ticks, layers: 1, frames: frames, playTime: 0, profile: 0x0049));
        for (int i = 0; i < embedCount; i++) AppendPng(ms, width, height);
        WriteChunk(ms, "MEND", []);
        return ms.ToArray();
    }

    private static byte[] BuildMhdr(int w, int h, int ticks, int layers, int frames, int playTime, uint profile)
    {
        var b = new byte[28];
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(0), (uint)w);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(4), (uint)h);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(8), (uint)ticks);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(12), (uint)layers);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(16), (uint)frames);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(20), (uint)playTime);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(24), profile);
        return b;
    }

    private static void AppendPng(Stream output, int width, int height)
    {
        // IHDR
        var ihdr = new byte[13];
        BinaryPrimitives.WriteUInt32BigEndian(ihdr.AsSpan(0), (uint)width);
        BinaryPrimitives.WriteUInt32BigEndian(ihdr.AsSpan(4), (uint)height);
        ihdr[8] = 8;     // bit depth
        ihdr[9] = 2;     // color type RGB
        ihdr[10] = 0;    // compression
        ihdr[11] = 0;    // filter
        ihdr[12] = 0;    // interlace
        WriteChunk(output, "IHDR", ihdr);

        // IDAT: zlib-wrapped raw scanlines (filter 0 + RGB triplets)
        using var raw = new MemoryStream();
        for (int y = 0; y < height; y++)
        {
            raw.WriteByte(0); // filter "none"
            for (int x = 0; x < width; x++)
            {
                raw.WriteByte((byte)(x * 16));
                raw.WriteByte((byte)(y * 16));
                raw.WriteByte(0x80);
            }
        }
        var idat = ZlibCompress(raw.ToArray());
        WriteChunk(output, "IDAT", idat);
        WriteChunk(output, "IEND", []);
    }

    private static byte[] ZlibCompress(byte[] data)
    {
        using var ms = new MemoryStream();
        // CMF + FLG
        ms.WriteByte(0x78);
        ms.WriteByte(0x9C);
        using (var ds = new DeflateStream(ms, CompressionMode.Compress, leaveOpen: true))
        {
            ds.Write(data, 0, data.Length);
        }
        uint adler = Adler32(data);
        ms.WriteByte((byte)((adler >> 24) & 0xFF));
        ms.WriteByte((byte)((adler >> 16) & 0xFF));
        ms.WriteByte((byte)((adler >> 8) & 0xFF));
        ms.WriteByte((byte)(adler & 0xFF));
        return ms.ToArray();
    }

    private static uint Adler32(ReadOnlySpan<byte> data)
    {
        uint a = 1, b = 0;
        foreach (byte x in data)
        {
            a = (a + x) % 65521;
            b = (b + a) % 65521;
        }
        return (b << 16) | a;
    }

    private static void WriteChunk(Stream output, string type, byte[] data)
    {
        Span<byte> hdr = stackalloc byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(hdr[..4], (uint)data.Length);
        hdr[4] = (byte)type[0];
        hdr[5] = (byte)type[1];
        hdr[6] = (byte)type[2];
        hdr[7] = (byte)type[3];
        output.Write(hdr);
        output.Write(data);
        var crcBuf = new byte[4 + data.Length];
        crcBuf[0] = hdr[4]; crcBuf[1] = hdr[5]; crcBuf[2] = hdr[6]; crcBuf[3] = hdr[7];
        Buffer.BlockCopy(data, 0, crcBuf, 4, data.Length);
        Span<byte> crc = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(crc, Crc32(crcBuf));
        output.Write(crc);
    }

    private static uint Crc32(ReadOnlySpan<byte> data)
    {
        uint c = 0xFFFFFFFFu;
        foreach (byte b in data)
        {
            c ^= b;
            for (int i = 0; i < 8; i++)
                c = (c >> 1) ^ (0xEDB88320u & (uint)-(int)(c & 1));
        }
        return c ^ 0xFFFFFFFFu;
    }
}
