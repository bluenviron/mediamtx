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
