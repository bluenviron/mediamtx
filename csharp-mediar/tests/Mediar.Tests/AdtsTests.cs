using Mediar.Containers.Adts;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class AdtsTests
{
    [Fact]
    public async Task RoundTrip_SingleFrame()
    {
        byte[] bytes = await MuxAsync(44100, 2, new byte[][] { Payload(64, 0) });
        Assert.True(AdtsHeader.TryParse(bytes, out var hdr));
        Assert.Equal(44100, hdr.SampleRate);
        Assert.Equal(2, hdr.ChannelConfig);
        Assert.Equal(7 + 64, hdr.FrameSize);
        Assert.False(hdr.HasCrc);
        Assert.Equal(7, hdr.HeaderSize);
        Assert.Equal(0, hdr.NumberOfRawDataBlocks);
        Assert.True(hdr.IsMpeg4);
        Assert.Equal(2, hdr.AudioObjectType); // profile=1 -> AOT-2 (AAC-LC)
    }

    [Fact]
    public async Task Demuxer_Reads_Frames()
    {
        byte[] payload = Payload(128, 0xAA);
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { payload, payload, payload });
        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes));
        Assert.Single(dx.Tracks);
        Assert.Equal(48000, ((AudioCodecParameters)dx.Tracks[0].Codec).SampleRate);

        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(128, s.Data.Length);
                Assert.Equal(payload, s.Data.ToArray());
            }
            finally { s.Owner?.Dispose(); }
            count++;
        }
        Assert.Equal(3, count);
    }

    [Theory]
    [InlineData(96000, 0)]
    [InlineData(88200, 1)]
    [InlineData(64000, 2)]
    [InlineData(48000, 3)]
    [InlineData(44100, 4)]
    [InlineData(32000, 5)]
    [InlineData(24000, 6)]
    [InlineData(22050, 7)]
    [InlineData(16000, 8)]
    [InlineData(12000, 9)]
    [InlineData(11025, 10)]
    [InlineData(8000, 11)]
    [InlineData(7350, 12)]
    public void IndexForSampleRate_Returns_Spec_Table(int sr, int expected)
    {
        Assert.Equal(expected, AdtsHeader.IndexForSampleRate(sr));
    }

    [Fact]
    public void IndexForSampleRate_Unknown_Returns_Minus1()
    {
        Assert.Equal(-1, AdtsHeader.IndexForSampleRate(192000));
        Assert.Equal(-1, AdtsHeader.IndexForSampleRate(123));
    }

    [Fact]
    public void TryParse_Empty_Returns_False()
    {
        Assert.False(AdtsHeader.TryParse(ReadOnlySpan<byte>.Empty, out var _));
    }

    [Fact]
    public void TryParse_Less_Than_Seven_Bytes_Returns_False()
    {
        Assert.False(AdtsHeader.TryParse(new byte[6], out _));
    }

    [Fact]
    public void TryParse_Wrong_Sync_Returns_False()
    {
        byte[] bad = new byte[7] { 0xFE, 0xF1, 0, 0, 0, 0, 0 };
        Assert.False(AdtsHeader.TryParse(bad, out _));
    }

    [Fact]
    public void TryParse_Non_Zero_Layer_Returns_False()
    {
        // layer bits = 01.
        byte[] bad = new byte[7] { 0xFF, 0xF3, 0x40, 0x40, 0, 0x10, 0 };
        Assert.False(AdtsHeader.TryParse(bad, out _));
    }

    [Fact]
    public void TryParse_Reserved_SampleRate_Index_Returns_False()
    {
        // sample_rate_idx = 13 (reserved).
        byte[] bad = new byte[7] { 0xFF, 0xF1, (1 << 6) | (13 << 2), 0x40, 0, 0x10, 0 };
        Assert.False(AdtsHeader.TryParse(bad, out _));
    }

    [Fact]
    public void TryParse_Frame_Length_Below_Header_Returns_False()
    {
        byte[] bad = new byte[7] { 0xFF, 0xF1, (1 << 6) | (4 << 2), 0x40, 0, 0x00, 0 };
        Assert.False(AdtsHeader.TryParse(bad, out _));
    }

    [Fact]
    public async Task TryParse_With_Crc_Header_Size_Is_Nine()
    {
        // Mux a normal protectionAbsent=1 frame, then flip the bit.
        byte[] bytes = await MuxAsync(44100, 2, new byte[][] { Payload(8, 0) });
        bytes[1] &= 0xFE; // clear protection_absent -> HasCrc=true
        Assert.True(AdtsHeader.TryParse(bytes, out var h));
        Assert.Equal(9, h.HeaderSize);
        Assert.True(h.HasCrc);
    }

    [Fact]
    public async Task TryParse_Mpeg2_Bit_Inverts_IsMpeg4()
    {
        byte[] bytes = await MuxAsync(44100, 2, new byte[][] { Payload(8, 0) });
        bytes[1] |= 0x08; // set MPEG-2 bit
        Assert.True(AdtsHeader.TryParse(bytes, out var h));
        Assert.False(h.IsMpeg4);
    }

    [Theory]
    [InlineData(96000)]
    [InlineData(48000)]
    [InlineData(11025)]
    [InlineData(7350)]
    public async Task RoundTrip_All_Spec_Sample_Rates(int sr)
    {
        byte[] payload = Payload(32, 0);
        byte[] bytes = await MuxAsync(sr, 1, new byte[][] { payload });
        Assert.True(AdtsHeader.TryParse(bytes, out var h));
        Assert.Equal(sr, h.SampleRate);
        Assert.Equal(1, h.ChannelConfig);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(6)]
    [InlineData(7)]
    public async Task RoundTrip_All_Channel_Configs(int channels)
    {
        byte[] bytes = await MuxAsync(48000, channels, new byte[][] { Payload(16, 0) });
        Assert.True(AdtsHeader.TryParse(bytes, out var h));
        Assert.Equal(channels, h.ChannelConfig);
    }

    // ---------- AdtsMuxer guards ----------

    [Fact]
    public void Muxer_Constructor_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AdtsMuxer(null!));
    }

    [Fact]
    public void Muxer_Constructor_Non_Writable_Throws()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new AdtsMuxer(ms));
    }

    [Fact]
    public void Muxer_FormatName_Is_Aac()
    {
        using var ms = new MemoryStream();
        using var mux = new AdtsMuxer(ms, leaveOpen: true);
        Assert.Equal("aac", mux.FormatName);
    }

    [Fact]
    public void Muxer_AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AdtsMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void Muxer_AddTrack_Non_Audio_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AdtsMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1, Codec = new VideoCodecParameters { Codec = CodecId.H264 },
            TimeBase = new Rational(1, 90000),
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Wrong_Codec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AdtsMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(BuildTrack(CodecId.Mp3, 44100, 2)));
    }

    [Fact]
    public void Muxer_AddTrack_Bad_SampleRate_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AdtsMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(BuildTrack(CodecId.Aac, 5500, 1)));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(8)]
    public void Muxer_AddTrack_Bad_Channel_Throws(int channels)
    {
        using var ms = new MemoryStream();
        using var mux = new AdtsMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(BuildTrack(CodecId.Aac, 48000, channels)));
    }

    [Fact]
    public void Muxer_AddTrack_Second_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AdtsMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2)));
    }

    [Fact]
    public async Task Muxer_AddTrack_After_Start_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AdtsMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
        await mux.StartAsync();
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2)));
    }

    [Fact]
    public async Task Muxer_StartAsync_Without_Track_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AdtsMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_WriteSample_Before_Start_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AdtsMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.WriteSampleAsync(BuildSample(0, new byte[8])));
    }

    [Fact]
    public async Task Muxer_WriteSample_After_Finish_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AdtsMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
        await mux.StartAsync();
        await mux.FinishAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.WriteSampleAsync(BuildSample(0, new byte[8])));
    }

    [Fact]
    public async Task Muxer_WriteSample_Wrong_TrackIndex_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AdtsMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentException>(async () => await mux.WriteSampleAsync(BuildSample(99, new byte[8])));
    }

    [Fact]
    public async Task Muxer_WriteSample_Oversized_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AdtsMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
        await mux.StartAsync();
        // Frame length field is 13 bits; max payload = 8191 - 7 = 8184.
        byte[] huge = new byte[8200];
        await Assert.ThrowsAsync<InvalidDataException>(async () => await mux.WriteSampleAsync(BuildSample(0, huge)));
    }

    [Fact]
    public async Task Muxer_FinishAsync_Idempotent()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AdtsMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync();
    }

    [Fact]
    public async Task Muxer_LeaveOpen_True_Keeps_Stream()
    {
        var ms = new MemoryStream();
        await using (var mux = new AdtsMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        _ = ms.Length;
        ms.Dispose();
    }

    [Fact]
    public async Task Muxer_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        await using (var mux = new AdtsMuxer(ms, leaveOpen: false))
        {
            mux.AddTrack(BuildTrack(CodecId.Aac, 48000, 2));
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    // ---------- AdtsDemuxer ----------

    [Fact]
    public void Demuxer_Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AdtsDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Demuxer_Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-adts-missing-{Guid.NewGuid():N}.aac");
        Assert.Throws<FileNotFoundException>(() => AdtsDemuxer.Open(path));
    }

    [Fact]
    public void Demuxer_Open_No_Sync_Throws()
    {
        byte[] junk = new byte[32];
        Assert.Throws<InvalidDataException>(() => AdtsDemuxer.Open(new MemoryRandomAccessSource(junk)));
    }

    [Fact]
    public async Task Demuxer_Pts_Increments_By_1024()
    {
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { Payload(16, 0), Payload(16, 0), Payload(16, 0) });
        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes));
        var pts = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { pts.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(new long[] { 0, 1024, 2048 }, pts);
    }

    [Fact]
    public async Task Demuxer_Seek_Skips_Earlier_Frames()
    {
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { Payload(8, 0), Payload(8, 1), Payload(8, 2), Payload(8, 3) });
        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes));
        // Each frame = 1024 samples = ~21.3 ms; seek to 25 ms -> skip frame 0 (1024 samples ends before target).
        await dx.SeekAsync(TimeSpan.FromMilliseconds(25));
        var pts = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { pts.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(new long[] { 1024, 2048, 3072 }, pts);
    }

    [Fact]
    public async Task Demuxer_Seek_Negative_Clamps_To_Zero()
    {
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { Payload(8, 0), Payload(8, 1) });
        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes));
        await dx.SeekAsync(TimeSpan.FromSeconds(-1));
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { count++; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public async Task Demuxer_Skips_Leading_Id3v2()
    {
        byte[] aac = await MuxAsync(48000, 2, new byte[][] { Payload(8, 0xCC), Payload(8, 0xDD) });
        // Build ID3v2 header + body of 5 bytes; synchsafe size 0x05 -> bytes 0,0,0,5.
        byte[] id3 = new byte[10 + 5];
        id3[0] = (byte)'I'; id3[1] = (byte)'D'; id3[2] = (byte)'3';
        id3[3] = 0x04; id3[4] = 0x00; id3[5] = 0x00;
        id3[9] = 0x05;
        byte[] full = id3.Concat(aac).ToArray();

        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(full));
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { count++; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public async Task Demuxer_FormatName_Is_Aac()
    {
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { Payload(8, 0) });
        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes));
        Assert.Equal("aac", dx.FormatName);
    }

    [Fact]
    public async Task Demuxer_Duration_Is_Zero()
    {
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { Payload(8, 0), Payload(8, 0) });
        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes));
        Assert.Equal(TimeSpan.Zero, dx.Duration);
    }

    [Fact]
    public async Task Demuxer_Channel_Config_7_Maps_To_8()
    {
        byte[] bytes = await MuxAsync(48000, 7, new byte[][] { Payload(8, 0) });
        using var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes));
        Assert.Equal(8, ((AudioCodecParameters)dx.Tracks[0].Codec).Channels);
    }

    [Fact]
    public async Task Demuxer_Dispose_Idempotent()
    {
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { Payload(8, 0) });
        var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes), ownsSource: true);
        dx.Dispose();
        dx.Dispose();
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        byte[] bytes = await MuxAsync(48000, 2, new byte[][] { Payload(8, 0) });
        var dx = AdtsDemuxer.Open(new MemoryRandomAccessSource(bytes), ownsSource: true);
        await dx.DisposeAsync();
    }

    // ---------- helpers ----------

    private static byte[] Payload(int size, int seed)
    {
        byte[] p = new byte[size];
        for (int i = 0; i < size; i++) p[i] = (byte)(i ^ seed);
        return p;
    }

    private static MediaTrack BuildTrack(CodecId codec, int sr, int channels) => new()
    {
        Index = 0, Id = 1, TimeBase = new Rational(1, sr),
        Codec = new AudioCodecParameters { Codec = codec, SampleRate = sr, Channels = channels },
    };

    private static MediaSample BuildSample(int trackIndex, byte[] data) => new()
    {
        TrackIndex = trackIndex, Pts = 0, Dts = 0,
        Duration = 1024, IsKeyFrame = true, Data = data,
    };

    private static async Task<byte[]> MuxAsync(int sr, int channels, byte[][] payloads)
    {
        using var ms = new MemoryStream();
        await using (var mux = new AdtsMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack(CodecId.Aac, sr, channels));
            await mux.StartAsync();
            for (int i = 0; i < payloads.Length; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = i * 1024, Dts = i * 1024, Duration = 1024,
                    IsKeyFrame = true, Data = payloads[i],
                });
            }
            await mux.FinishAsync();
        }
        return ms.ToArray();
    }
}
