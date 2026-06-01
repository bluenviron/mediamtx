using Mediar.Containers.Gsm;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class GsmRoundTripTests
{
    [Fact]
    public async Task Gsm_RoundTrips_Frames_Through_Muxer()
    {
        const int frameCount = 50;
        byte[] frames = new byte[frameCount * GsmDemuxer.FrameBytes];
        for (int i = 0; i < frames.Length; i++) frames[i] = (byte)(i ^ 0x5A);

        byte[] file = await MuxAsync(frames, frameCount);
        Assert.Equal(frameCount * GsmDemuxer.FrameBytes, file.Length);

        using var src = new MemoryRandomAccessSource(file);
        using var dx = GsmDemuxer.Open(src);

        Assert.Equal("gsm", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.Gsm610, a.Codec);
        Assert.Equal(8000, a.SampleRate);

        int seen = 0;
        long totalBytes = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(GsmDemuxer.FrameBytes, s.Data.Length);
                Assert.Equal(GsmDemuxer.FrameSamples, s.Duration);
                totalBytes += s.Data.Length;
                seen++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(frameCount, seen);
        Assert.Equal(frames.Length, totalBytes);
    }

    [Fact]
    public async Task Muxer_Rejects_Non_33_Byte_Frames()
    {
        await using var ms = new MemoryStream();
        await using var mux = new GsmMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        var ex = await Assert.ThrowsAsync<InvalidDataException>(async () =>
        {
            await mux.StartAsync();
            await mux.WriteSampleAsync(BuildSample(new byte[20]));
        });
        Assert.Contains("33", ex.Message);
    }

    // ---------- Muxer ----------

    [Fact]
    public void Muxer_Constructor_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new GsmMuxer(null!));
    }

    [Fact]
    public void Muxer_Constructor_NonWritable_Throws()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new GsmMuxer(ms));
    }

    [Fact]
    public void Muxer_FormatName_Is_Gsm()
    {
        using var ms = new MemoryStream();
        using var mux = new GsmMuxer(ms, leaveOpen: true);
        Assert.Equal("gsm", mux.FormatName);
    }

    [Fact]
    public void Muxer_AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new GsmMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void Muxer_AddTrack_Wrong_Codec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new GsmMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1, TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS16Le, SampleRate = 8000, Channels = 1 },
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Video_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new GsmMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1, TimeBase = new Rational(1, 90000),
            Codec = new VideoCodecParameters { Codec = CodecId.H264 },
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Second_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new GsmMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack()));
    }

    [Fact]
    public async Task Muxer_StartAsync_Without_Track_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new GsmMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_WriteSample_Auto_Starts()
    {
        await using var ms = new MemoryStream();
        await using (var mux = new GsmMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
            // No StartAsync — but WriteSampleAsync should start automatically.
            await mux.WriteSampleAsync(BuildSample(new byte[GsmDemuxer.FrameBytes]));
            await mux.FinishAsync();
        }
        Assert.Equal(GsmDemuxer.FrameBytes, ms.Length);
    }

    [Fact]
    public async Task Muxer_FinishAsync_Sets_Finished()
    {
        await using var ms = new MemoryStream();
        await using var mux = new GsmMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync(); // idempotent
    }

    [Fact]
    public async Task Muxer_LeaveOpen_True_Keeps_Stream()
    {
        var ms = new MemoryStream();
        await using (var mux = new GsmMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
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
        await using (var mux = new GsmMuxer(ms, leaveOpen: false))
        {
            mux.AddTrack(BuildTrack());
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public void Muxer_Sync_Dispose_Works()
    {
        var ms = new MemoryStream();
        var mux = new GsmMuxer(ms, leaveOpen: false);
        mux.AddTrack(BuildTrack());
        mux.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    // ---------- Demuxer ----------

    [Fact]
    public void Demuxer_Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => GsmDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Demuxer_Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-gsm-missing-{Guid.NewGuid():N}.gsm");
        Assert.Throws<FileNotFoundException>(() => GsmDemuxer.Open(path));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    public void Demuxer_Open_Bad_SampleRate_Throws(int sr)
    {
        using var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes * 2]);
        Assert.Throws<ArgumentOutOfRangeException>(() => GsmDemuxer.Open(src, ownsSource: false, sampleRate: sr));
    }

    [Fact]
    public void Demuxer_Open_Custom_SampleRate_Threads_Through()
    {
        using var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes * 2]);
        using var dx = GsmDemuxer.Open(src, ownsSource: false, sampleRate: 11025);
        var a = (AudioCodecParameters)dx.Tracks[0].Codec;
        Assert.Equal(11025, a.SampleRate);
    }

    [Fact]
    public void Demuxer_Open_NonMultiple_Length_Rounds_Down()
    {
        // 2 full frames + 5 extra bytes.
        using var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes * 2 + 5]);
        using var dx = GsmDemuxer.Open(src);
        Assert.Equal(2 * GsmDemuxer.FrameSamples, dx.Tracks[0].DurationTicks);
    }

    [Fact]
    public void Demuxer_Empty_Source_Allowed()
    {
        using var src = new MemoryRandomAccessSource(Array.Empty<byte>());
        using var dx = GsmDemuxer.Open(src);
        Assert.Equal(0, dx.Tracks[0].DurationTicks);
        Assert.Equal(TimeSpan.Zero, dx.Duration);
    }

    [Fact]
    public async Task Demuxer_Empty_Source_Yields_No_Samples()
    {
        using var src = new MemoryRandomAccessSource(Array.Empty<byte>());
        using var dx = GsmDemuxer.Open(src);
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            s.Owner?.Dispose();
            count++;
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public void Demuxer_Duration_Reflects_Length()
    {
        using var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes * 10]);
        using var dx = GsmDemuxer.Open(src);
        // 10 frames × 160 samples / 8000 Hz = 200 ms.
        Assert.Equal(TimeSpan.FromMilliseconds(200), dx.Duration);
    }

    [Fact]
    public async Task Demuxer_Pts_Increments_By_FrameSamples()
    {
        byte[] file = await MuxAsync(new byte[3 * GsmDemuxer.FrameBytes], 3);
        using var src = new MemoryRandomAccessSource(file);
        using var dx = GsmDemuxer.Open(src);
        var pts = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { pts.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(new long[] { 0, GsmDemuxer.FrameSamples, 2 * GsmDemuxer.FrameSamples }, pts);
    }

    [Fact]
    public async Task Demuxer_Seek_Skips_Earlier_Frames()
    {
        byte[] file = await MuxAsync(new byte[5 * GsmDemuxer.FrameBytes], 5);
        using var src = new MemoryRandomAccessSource(file);
        using var dx = GsmDemuxer.Open(src);
        // Seek into the 3rd frame (sample 320 onwards).
        await dx.SeekAsync(TimeSpan.FromMilliseconds(40));
        var pts = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { pts.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(new long[] { 320, 480, 640 }, pts);
    }

    [Fact]
    public async Task Demuxer_Seek_Negative_Reads_All()
    {
        byte[] file = await MuxAsync(new byte[2 * GsmDemuxer.FrameBytes], 2);
        using var src = new MemoryRandomAccessSource(file);
        using var dx = GsmDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(-1));
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            s.Owner?.Dispose();
            count++;
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public async Task Demuxer_Seek_Past_End_Yields_No_Samples()
    {
        byte[] file = await MuxAsync(new byte[2 * GsmDemuxer.FrameBytes], 2);
        using var src = new MemoryRandomAccessSource(file);
        using var dx = GsmDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(60));
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            s.Owner?.Dispose();
            count++;
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public void Demuxer_Metadata_Is_Empty()
    {
        using var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes]);
        using var dx = GsmDemuxer.Open(src);
        Assert.True(dx.Metadata.IsEmpty);
    }

    [Fact]
    public void Demuxer_Dispose_Idempotent()
    {
        var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes]);
        var dx = GsmDemuxer.Open(src, ownsSource: true);
        dx.Dispose();
        dx.Dispose();
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes]);
        var dx = GsmDemuxer.Open(src, ownsSource: true);
        await dx.DisposeAsync();
    }

    [Fact]
    public void Demuxer_OwnsSource_False_Leaves_Source_Open()
    {
        var src = new MemoryRandomAccessSource(new byte[GsmDemuxer.FrameBytes]);
        var dx = GsmDemuxer.Open(src, ownsSource: false);
        dx.Dispose();
        _ = src.Length; // still accessible
        src.Dispose();
    }

    // ---------- helpers ----------

    private static MediaTrack BuildTrack() => new()
    {
        Index = 0, Id = 1, TimeBase = new Rational(1, 8000),
        Codec = new AudioCodecParameters { Codec = CodecId.Gsm610, SampleRate = 8000, Channels = 1 },
    };

    private static MediaSample BuildSample(byte[] data) => new()
    {
        TrackIndex = 0, Pts = 0, Dts = 0, Duration = GsmDemuxer.FrameSamples,
        IsKeyFrame = true, Data = data,
    };

    private static async Task<byte[]> MuxAsync(byte[] frames, int frameCount)
    {
        await using var ms = new MemoryStream();
        await using var mux = new GsmMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        for (int i = 0; i < frameCount; i++)
        {
            byte[] one = new byte[GsmDemuxer.FrameBytes];
            Array.Copy(frames, i * GsmDemuxer.FrameBytes, one, 0, GsmDemuxer.FrameBytes);
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = i * GsmDemuxer.FrameSamples,
                Dts = i * GsmDemuxer.FrameSamples,
                Duration = GsmDemuxer.FrameSamples, IsKeyFrame = true, Data = one,
            });
        }
        await mux.FinishAsync();
        return ms.ToArray();
    }
}
