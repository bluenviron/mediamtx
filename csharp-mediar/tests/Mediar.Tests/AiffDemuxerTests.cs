using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.Aiff;
using Xunit;

namespace Mediar.Tests;

public sealed class AiffDemuxerTests
{
    [Fact]
    public async Task Reads_Pcm16Be_Stream_And_Metadata()
    {
        const int sr = 44100;
        const int ch = 1;
        const int frames = 256;
        byte[] pcm = new byte[frames * 2];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)(i * 10);
            pcm[i * 2 + 0] = (byte)(v >> 8);
            pcm[i * 2 + 1] = (byte)v;
        }

        byte[] aiff = BuildAiff(sr, ch, bits: 16, pcm, title: "Hello", author: "Author");

        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);

        Assert.Equal("aiff", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Be, audio.Codec);
        Assert.Equal(sr, audio.SampleRate);
        Assert.Equal(ch, audio.Channels);

        Assert.Equal("Hello", dx.Metadata.Title);
        Assert.Equal("Author", dx.Metadata.Artist);

        int totalBytes = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { totalBytes += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, totalBytes);
    }

    private static byte[] BuildAiff(int sampleRate, int channels, int bits, ReadOnlySpan<byte> ssndData, string title, string author)
    {
        using var ms = new MemoryStream();

        // COMM chunk body: channels(2) + numSampleFrames(4) + sampleSize(2) + sampleRate(10 ext)
        byte[] comm = new byte[18];
        BinaryPrimitives.WriteUInt16BigEndian(comm.AsSpan(0, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32BigEndian(comm.AsSpan(2, 4), (uint)(ssndData.Length / (bits / 8 * channels)));
        BinaryPrimitives.WriteUInt16BigEndian(comm.AsSpan(6, 2), (ushort)bits);
        WriteExtendedFloat(comm.AsSpan(8, 10), sampleRate);

        byte[] ssnd = new byte[8 + ssndData.Length];
        // offset=0, blockSize=0
        ssndData.CopyTo(ssnd.AsSpan(8));

        // Compute total FORM size: 4 (AIFF) + chunks
        long bodySize = 4
            + 8 + comm.Length
            + 8 + ssnd.Length
            + 8 + Encoding.UTF8.GetByteCount(title) + (Encoding.UTF8.GetByteCount(title) & 1)
            + 8 + Encoding.UTF8.GetByteCount(author) + (Encoding.UTF8.GetByteCount(author) & 1);

        WriteAscii(ms, "FORM");
        WriteBeUInt32(ms, (uint)bodySize);
        WriteAscii(ms, "AIFF");
        WriteChunk(ms, "COMM", comm);
        WriteChunk(ms, "SSND", ssnd);
        WriteChunk(ms, "NAME", Encoding.UTF8.GetBytes(title));
        WriteChunk(ms, "AUTH", Encoding.UTF8.GetBytes(author));
        return ms.ToArray();
    }

    private static void WriteChunk(MemoryStream ms, string id, ReadOnlySpan<byte> data)
    {
        WriteAscii(ms, id);
        WriteBeUInt32(ms, (uint)data.Length);
        ms.Write(data);
        if ((data.Length & 1) != 0) ms.WriteByte(0);
    }

    private static void WriteAscii(MemoryStream ms, string s)
    {
        for (int i = 0; i < s.Length; i++) ms.WriteByte((byte)s[i]);
    }

    private static void WriteBeUInt32(MemoryStream ms, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(b, v);
        ms.Write(b);
    }

    private static void WriteExtendedFloat(Span<byte> dest, double value)
    {
        if (value == 0) { dest.Clear(); return; }
        int sign = value < 0 ? 1 : 0;
        value = Math.Abs(value);
        int exponent = (int)Math.Floor(Math.Log2(value));
        double mantissa = value / Math.Pow(2, exponent);
        ulong mantissaBits = (ulong)Math.Round(mantissa * (1UL << 63));
        ushort sExp = (ushort)((sign << 15) | (exponent + 16383));
        BinaryPrimitives.WriteUInt16BigEndian(dest[..2], sExp);
        BinaryPrimitives.WriteUInt64BigEndian(dest[2..10], mantissaBits);
    }
}
