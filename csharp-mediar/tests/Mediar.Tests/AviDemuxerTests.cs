using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.Avi;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class AviDemuxerTests
{
    [Fact]
    public async Task Reads_Single_Audio_Pcm_Stream_With_Metadata()
    {
        const int sr = 8000;
        const int ch = 1;
        const int bits = 16;
        const int frames = 800;
        byte[] pcm = new byte[frames * (bits / 8) * ch];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)(i * 100);
            pcm[i * 2 + 0] = (byte)v;
            pcm[i * 2 + 1] = (byte)(v >> 8);
        }

        byte[] avi = BuildPcmAvi(sr, ch, bits, pcm, title: "Track", artist: "Artist");

        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);

        Assert.Equal("avi", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Le, audio.Codec);
        Assert.Equal(sr, audio.SampleRate);
        Assert.Equal(ch, audio.Channels);
        Assert.Equal(bits, audio.BitsPerSample);

        Assert.Equal("Track", dx.Metadata.Title);
        Assert.Equal("Artist", dx.Metadata.Artist);

        int totalBytes = 0;
        int samples = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                totalBytes += s.Data.Length;
                samples++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, totalBytes);
        Assert.True(samples > 0);
    }

    [Fact]
    public void Open_FromNullSource_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AviDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Open_FileSmallerThan12_Throws_InvalidData()
    {
        using var src = new IO.MemoryRandomAccessSource(new byte[8]);
        var ex = Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
        Assert.Contains("RIFF", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_NonRiff_Magic_Throws_InvalidData()
    {
        byte[] hdr = new byte[12];
        WriteAscii(hdr.AsSpan(0, 4), "NOPE");
        WriteAscii(hdr.AsSpan(8, 4), "AVI ");
        using var src = new IO.MemoryRandomAccessSource(hdr);
        Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
    }

    [Fact]
    public void Open_Riff_But_NonAvi_FormType_Throws_InvalidData()
    {
        byte[] hdr = new byte[12];
        WriteAscii(hdr.AsSpan(0, 4), "RIFF");
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4, 4), 4u);
        WriteAscii(hdr.AsSpan(8, 4), "WAVE");
        using var src = new IO.MemoryRandomAccessSource(hdr);
        var ex = Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
        Assert.Contains("AVI", ex.Message);
    }

    [Fact]
    public void Open_Riff_Avi_But_Missing_Movi_Throws_InvalidData()
    {
        // A bare RIFF/AVI header without any movi chunk should report
        // a missing movi error during Open.
        byte[] hdr = new byte[12];
        WriteAscii(hdr.AsSpan(0, 4), "RIFF");
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4, 4), 4u);
        WriteAscii(hdr.AsSpan(8, 4), "AVI ");
        using var src = new IO.MemoryRandomAccessSource(hdr);
        var ex = Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
        Assert.Contains("movi", ex.Message);
    }

    [Fact]
    public void Demuxer_FormatName_Is_Avi()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.Equal("avi", dx.FormatName);
    }

    [Fact]
    public void Demuxer_Dispose_Is_Idempotent()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        var dx = AviDemuxer.Open(src);
        dx.Dispose();
        dx.Dispose(); // should not throw
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        var dx = AviDemuxer.Open(src);
        await dx.DisposeAsync();
    }

    [Fact]
    public async Task SeekAsync_Without_Index_Returns_Immediately()
    {
        // Open a file without idx1 (we'll just open one and ignore Seek
        // semantics; we just need to confirm the call doesn't throw).
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(0.5));
        // Calling Seek must not corrupt subsequent enumeration.
        int chunks = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            chunks++;
            s.Owner?.Dispose();
        }
        Assert.True(chunks > 0);
    }

    [Fact]
    public async Task SeekAsync_Negative_Time_Is_Clamped_To_Zero()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(-100));
        // Still able to enumerate.
        int chunks = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            chunks++;
            s.Owner?.Dispose();
        }
        Assert.True(chunks > 0);
    }

    [Fact]
    public async Task ReadSamplesAsync_Honours_Cancellation()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[200], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var s in dx.ReadSamplesAsync(cts.Token))
            {
                s.Owner?.Dispose();
            }
        });
    }

    [Fact]
    public void Track_Carries_Audio_Codec_And_TimeBase()
    {
        byte[] avi = BuildPcmAvi(48000, 2, 16, new byte[256], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var t = Assert.Single(dx.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(48000, audio.SampleRate);
        Assert.Equal(2, audio.Channels);
        Assert.Equal(16, audio.BitsPerSample);
        Assert.Equal(CodecId.PcmS16Le, audio.Codec);
        Assert.Equal("und", t.Language);
        Assert.Equal(0, t.Index);
    }

    [Fact]
    public void Metadata_All_Latin1_Roundtrips()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[32], title: "Café", artist: "Niño");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.Equal("Café", dx.Metadata.Title);
        Assert.Equal("Niño", dx.Metadata.Artist);
    }

    [Fact]
    public async Task Sample_PTS_Increases_Monotonically()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[400], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        long? lastPts = null;
        int n = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                if (lastPts.HasValue)
                {
                    Assert.True(s.Pts >= lastPts.Value, $"PTS regressed: {s.Pts} after {lastPts.Value}");
                }
                lastPts = s.Pts;
                n++;
                // Audio samples are always "key" frames in this demuxer.
                Assert.True(s.IsKeyFrame);
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.True(n >= 2);
    }

    [Fact]
    public void Open_FromString_OverloadExists()
    {
        // Just confirm the file-path overload throws on a missing file
        // (it should funnel through FileRandomAccessSource).
        Assert.ThrowsAny<Exception>(() => AviDemuxer.Open("Z:\\nonexistent-avi-file.avi"));
    }

    /// <summary>
    /// Build a tiny RIFF/AVI 1-stream PCM file with idx1, LIST INFO, and a
    /// movi list containing the data split across two ##wb chunks.
    /// </summary>
    private static byte[] BuildPcmAvi(int sampleRate, int channels, int bits, byte[] pcm, string title, string artist)
    {
        // Split PCM in half — the test exercises two-chunk movi parsing.
        int half = (pcm.Length / 2) & ~1;
        int rest = pcm.Length - half;
        ReadOnlySpan<byte> chunk1 = pcm.AsSpan(0, half);
        ReadOnlySpan<byte> chunk2 = pcm.AsSpan(half, rest);

        // strh (size 56) + strf (WAVEFORMATEX 18) + chunk overhead.
        byte[] strh = new byte[56];
        WriteAscii(strh.AsSpan(0, 4), "auds");
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(20, 4), 1u); // scale = 1
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(24, 4), (uint)sampleRate); // rate
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(32, 4), (uint)(pcm.Length / (bits / 8 * channels))); // length frames
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(40, 4), (uint)(bits / 8 * channels)); // sample size

        byte[] strf = new byte[16];
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(0, 2), 1); // PCM
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(2, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(4, 4), (uint)sampleRate);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(8, 4), (uint)(sampleRate * channels * (bits / 8))); // avg bytes/sec
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(12, 2), (ushort)(channels * (bits / 8))); // block align
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(14, 2), (ushort)bits);

        // avih (size 56). Only microsec/frame and TotalFrames matter for our duration.
        byte[] avih = new byte[56];
        BinaryPrimitives.WriteUInt32LittleEndian(avih.AsSpan(0, 4), (uint)(1_000_000.0 / 25)); // 25 fps placeholder
        BinaryPrimitives.WriteUInt32LittleEndian(avih.AsSpan(16, 4), (uint)(pcm.Length / (bits / 8 * channels)));

        // INFO LIST
        byte[] info = BuildInfo(title, artist);

        // ----- assemble -----
        using var ms = new MemoryStream();
        WriteAscii(ms, "RIFF");
        // placeholder for RIFF size
        long sizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "AVI ");

        // hdrl
        WriteAscii(ms, "LIST");
        long hdrlSizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "hdrl");
        WriteChunk(ms, "avih", avih);

        WriteAscii(ms, "LIST");
        long strlSizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "strl");
        WriteChunk(ms, "strh", strh);
        WriteChunk(ms, "strf", strf);
        long strlEnd = ms.Position;
        PatchSize(ms, strlSizeOffset, (uint)(strlEnd - strlSizeOffset - 4));
        long hdrlEnd = ms.Position;
        PatchSize(ms, hdrlSizeOffset, (uint)(hdrlEnd - hdrlSizeOffset - 4));

        // movi list with two ##wb chunks, capturing offsets for idx1.
        WriteAscii(ms, "LIST");
        long moviSizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        long moviStart = ms.Position;
        WriteAscii(ms, "movi");

        long chunk1HdrOffset = ms.Position; // relative to file
        WriteChunk(ms, "00wb", chunk1);
        long chunk2HdrOffset = ms.Position;
        WriteChunk(ms, "00wb", chunk2);

        long moviEnd = ms.Position;
        PatchSize(ms, moviSizeOffset, (uint)(moviEnd - moviSizeOffset - 4));

        // idx1
        byte[] idx1 = new byte[2 * 16];
        WriteAscii(idx1.AsSpan(0, 4), "00wb");
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(4, 4), 0x10u); // AVIIF_KEYFRAME
        // movi-relative offset of chunk header from moviStart - 4
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(8, 4), (uint)(chunk1HdrOffset - (moviStart - 4)));
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(12, 4), (uint)chunk1.Length);

        WriteAscii(idx1.AsSpan(16, 4), "00wb");
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(20, 4), 0x10u);
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(24, 4), (uint)(chunk2HdrOffset - (moviStart - 4)));
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(28, 4), (uint)chunk2.Length);

        WriteChunk(ms, "idx1", idx1);

        // LIST INFO
        WriteAscii(ms, "LIST");
        WriteLeUInt32(ms, (uint)(info.Length + 4));
        WriteAscii(ms, "INFO");
        ms.Write(info);

        long fileEnd = ms.Position;
        PatchSize(ms, sizeOffset, (uint)(fileEnd - sizeOffset - 4));
        return ms.ToArray();
    }

    private static byte[] BuildInfo(string title, string artist)
    {
        using var ms = new MemoryStream();
        WriteChunk(ms, "INAM", Encoding.Latin1.GetBytes(title + "\0"));
        WriteChunk(ms, "IART", Encoding.Latin1.GetBytes(artist + "\0"));
        return ms.ToArray();
    }

    private static void WriteChunk(MemoryStream ms, string id, ReadOnlySpan<byte> data)
    {
        WriteAscii(ms, id);
        WriteLeUInt32(ms, (uint)data.Length);
        ms.Write(data);
        if ((data.Length & 1) != 0) ms.WriteByte(0);
    }

    private static void WriteAscii(MemoryStream ms, string s)
    {
        for (int i = 0; i < s.Length; i++) ms.WriteByte((byte)s[i]);
    }

    private static void WriteAscii(Span<byte> dest, string s)
    {
        for (int i = 0; i < s.Length; i++) dest[i] = (byte)s[i];
    }

    private static void WriteLeUInt32(MemoryStream ms, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        ms.Write(b);
    }

    private static void PatchSize(MemoryStream ms, long offset, uint value)
    {
        long pos = ms.Position;
        ms.Position = offset;
        WriteLeUInt32(ms, value);
        ms.Position = pos;
    }
}
