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

    [Fact]
    public async Task ANNO_And_COPY_Map_To_Comment_And_Copyright()
    {
        byte[] pcm = new byte[16];
        byte[] aiff = BuildAiffWithChunks(8000, 1, 16, pcm,
            new (string id, byte[] body)[]
            {
                ("ANNO", Encoding.UTF8.GetBytes("a comment")),
                ("COPY", Encoding.UTF8.GetBytes("(c) 2024")),
            });

        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        Assert.Equal("a comment", dx.Metadata.Comment);
        Assert.Equal("(c) 2024", dx.Metadata.Copyright);
    }

    [Fact]
    public async Task FormatName_Is_Aiff()
    {
        byte[] aiff = BuildAiff(8000, 1, 16, new byte[16], "", "");
        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        Assert.Equal("aiff", dx.FormatName);
    }

    [Theory]
    [InlineData(8000)]
    [InlineData(11025)]
    [InlineData(22050)]
    [InlineData(44100)]
    [InlineData(48000)]
    [InlineData(96000)]
    public async Task ExtendedFloat_Decodes_Common_SampleRates(int sr)
    {
        byte[] pcm = new byte[sr * 2 / 100]; // 10 ms
        byte[] aiff = BuildAiff(sr, 1, 16, pcm, "", "");
        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(sr, a.SampleRate);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    public async Task Channel_Count_RoundTrips(int channels)
    {
        byte[] pcm = new byte[128 * channels * 2];
        byte[] aiff = BuildAiff(44100, channels, 16, pcm, "", "");
        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(channels, a.Channels);
    }

    [Fact]
    public async Task Duration_Reflects_PCM_Length()
    {
        // 8000 Hz × 0.5 s = 4000 mono 16-bit frames = 8000 bytes.
        byte[] pcm = new byte[8000];
        byte[] aiff = BuildAiff(8000, 1, 16, pcm, "", "");
        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        Assert.InRange((dx.Duration - TimeSpan.FromSeconds(0.5)).TotalMilliseconds, -5, 5);
    }

    [Fact]
    public async Task ReadSamples_Pts_Increments_By_Packet_Frames()
    {
        const int sr = 8000;
        // 5 packets × (sr/100) frames = 50 ms at 100 Hz packet rate.
        byte[] pcm = new byte[5 * (sr / 100) * 2];
        byte[] aiff = BuildAiff(sr, 1, 16, pcm, "", "");
        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        var pts = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { pts.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(5, pts.Count);
        long step = pts[1] - pts[0];
        Assert.All(Enumerable.Range(0, pts.Count - 1), i =>
            Assert.Equal(step, pts[i + 1] - pts[i]));
    }

    [Fact]
    public async Task Seek_Negative_Reads_All_Samples()
    {
        byte[] pcm = new byte[80 * 2];
        byte[] aiff = BuildAiff(8000, 1, 16, pcm, "", "");
        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(-1));
        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Seek_Past_End_Yields_No_Samples()
    {
        byte[] pcm = new byte[16 * 2];
        byte[] aiff = BuildAiff(8000, 1, 16, pcm, "", "");
        using var src = new IO.MemoryRandomAccessSource(aiff);
        using var dx = AiffDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromHours(1));
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { count++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public void Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AiffDemuxer.Open((IO.IRandomAccessSource)null!));
    }

    [Fact]
    public void Open_Missing_Path_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-aiff-{Guid.NewGuid():N}.aiff");
        Assert.Throws<FileNotFoundException>(() => AiffDemuxer.Open(path));
    }

    [Fact]
    public void Open_Too_Small_Throws()
    {
        using var src = new IO.MemoryRandomAccessSource(new byte[4]);
        Assert.Throws<InvalidDataException>(() => AiffDemuxer.Open(src));
    }

    [Fact]
    public void Open_Missing_FORM_Throws()
    {
        var b = new byte[12];
        b[0] = (byte)'X'; b[1] = (byte)'O'; b[2] = (byte)'R'; b[3] = (byte)'M';
        using var src = new IO.MemoryRandomAccessSource(b);
        Assert.Throws<InvalidDataException>(() => AiffDemuxer.Open(src));
    }

    [Fact]
    public void Open_Wrong_Form_Type_Throws()
    {
        var b = new byte[12];
        b[0] = (byte)'F'; b[1] = (byte)'O'; b[2] = (byte)'R'; b[3] = (byte)'M';
        b[8] = (byte)'B'; b[9] = (byte)'A'; b[10] = (byte)'D'; b[11] = (byte)'!';
        using var src = new IO.MemoryRandomAccessSource(b);
        Assert.Throws<InvalidDataException>(() => AiffDemuxer.Open(src));
    }

    [Fact]
    public void Open_Missing_COMM_Throws()
    {
        // Only SSND chunk.
        using var ms = new MemoryStream();
        WriteAscii(ms, "FORM");
        WriteBeUInt32(ms, 4 + 8 + 8); // form size
        WriteAscii(ms, "AIFF");
        WriteAscii(ms, "SSND");
        WriteBeUInt32(ms, 8);
        ms.Write(new byte[8]);
        using var src = new IO.MemoryRandomAccessSource(ms.ToArray());
        Assert.Throws<InvalidDataException>(() => AiffDemuxer.Open(src));
    }

    [Fact]
    public async Task Aifc_Sowt_Maps_To_PcmS16Le()
    {
        byte[] pcm = new byte[16];
        byte[] aifc = BuildAifc(8000, 1, 16, pcm, "sowt");
        using var src = new IO.MemoryRandomAccessSource(aifc);
        using var dx = AiffDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.PcmS16Le, a.Codec);
    }

    [Fact]
    public async Task Aifc_fl32_Maps_To_PcmF32Le()
    {
        byte[] pcm = new byte[32];
        byte[] aifc = BuildAifc(48000, 2, 32, pcm, "fl32");
        using var src = new IO.MemoryRandomAccessSource(aifc);
        using var dx = AiffDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.PcmF32Le, a.Codec);
    }

    [Fact]
    public async Task Aifc_ulaw_Maps_To_G711MuLaw()
    {
        byte[] pcm = new byte[16];
        byte[] aifc = BuildAifc(8000, 1, 8, pcm, "ulaw");
        using var src = new IO.MemoryRandomAccessSource(aifc);
        using var dx = AiffDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.G711MuLaw, a.Codec);
    }

    [Fact]
    public async Task Aifc_alaw_Maps_To_G711ALaw()
    {
        byte[] pcm = new byte[16];
        byte[] aifc = BuildAifc(8000, 1, 8, pcm, "alaw");
        using var src = new IO.MemoryRandomAccessSource(aifc);
        using var dx = AiffDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.G711ALaw, a.Codec);
    }

    [Fact]
    public async Task Dispose_Idempotent()
    {
        byte[] aiff = BuildAiff(8000, 1, 16, new byte[8], "", "");
        var src = new IO.MemoryRandomAccessSource(aiff);
        var dx = AiffDemuxer.Open(src, ownsSource: true);
        dx.Dispose();
        dx.Dispose();
    }

    [Fact]
    public async Task DisposeAsync_Works()
    {
        byte[] aiff = BuildAiff(8000, 1, 16, new byte[8], "", "");
        var src = new IO.MemoryRandomAccessSource(aiff);
        var dx = AiffDemuxer.Open(src, ownsSource: true);
        await dx.DisposeAsync();
    }

    // ---------- helpers ----------

    private static byte[] BuildAiffWithChunks(int sr, int ch, int bits, byte[] pcm, (string id, byte[] body)[] extras)
    {
        using var ms = new MemoryStream();
        byte[] comm = new byte[18];
        BinaryPrimitives.WriteUInt16BigEndian(comm.AsSpan(0, 2), (ushort)ch);
        BinaryPrimitives.WriteUInt32BigEndian(comm.AsSpan(2, 4), (uint)(pcm.Length / (bits / 8 * ch)));
        BinaryPrimitives.WriteUInt16BigEndian(comm.AsSpan(6, 2), (ushort)bits);
        WriteExtendedFloat(comm.AsSpan(8, 10), sr);

        byte[] ssnd = new byte[8 + pcm.Length];
        pcm.CopyTo(ssnd.AsSpan(8));

        long bodySize = 4 + 8 + comm.Length + 8 + ssnd.Length;
        foreach (var (_, b) in extras) bodySize += 8 + b.Length + (b.Length & 1);

        WriteAscii(ms, "FORM");
        WriteBeUInt32(ms, (uint)bodySize);
        WriteAscii(ms, "AIFF");
        WriteChunk(ms, "COMM", comm);
        WriteChunk(ms, "SSND", ssnd);
        foreach (var (id, body) in extras) WriteChunk(ms, id, body);
        return ms.ToArray();
    }

    private static byte[] BuildAifc(int sr, int ch, int bits, byte[] pcm, string compFourCc)
    {
        using var ms = new MemoryStream();
        // Pascal string after fourcc.
        byte[] name = Encoding.ASCII.GetBytes("Test");
        int commBodyLen = 18 + 4 + 1 + name.Length;
        if ((commBodyLen & 1) != 0) commBodyLen++; // pad to even
        byte[] comm = new byte[commBodyLen];
        BinaryPrimitives.WriteUInt16BigEndian(comm.AsSpan(0, 2), (ushort)ch);
        BinaryPrimitives.WriteUInt32BigEndian(comm.AsSpan(2, 4), (uint)(pcm.Length / (bits / 8 * ch)));
        BinaryPrimitives.WriteUInt16BigEndian(comm.AsSpan(6, 2), (ushort)bits);
        WriteExtendedFloat(comm.AsSpan(8, 10), sr);
        comm[18] = (byte)compFourCc[0];
        comm[19] = (byte)compFourCc[1];
        comm[20] = (byte)compFourCc[2];
        comm[21] = (byte)compFourCc[3];
        comm[22] = (byte)name.Length;
        Array.Copy(name, 0, comm, 23, name.Length);

        byte[] ssnd = new byte[8 + pcm.Length];
        pcm.CopyTo(ssnd.AsSpan(8));

        long bodySize = 4 + 8 + comm.Length + 8 + ssnd.Length;

        WriteAscii(ms, "FORM");
        WriteBeUInt32(ms, (uint)bodySize);
        WriteAscii(ms, "AIFC");
        WriteChunk(ms, "COMM", comm);
        WriteChunk(ms, "SSND", ssnd);
        return ms.ToArray();
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
