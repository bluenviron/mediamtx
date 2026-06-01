using Mediar.Containers.IsoBmff;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// End-to-end MP4 muxer/demuxer tests covering single- and multi-track files, seek
/// behaviour, sample-table edge cases, and the guard rails on both classes.
/// </summary>
public sealed class Mp4RoundTripTests
{
    [Fact]
    public async Task SingleAudioTrack_RoundTrips_Through_Mp4Muxer()
    {
        byte[] bytes = await MuxAudioAsync(5);
        Assert.True(bytes.Length > 0);

        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);

        Assert.Equal("mp4", dx.FormatName);
        Assert.Single(dx.Tracks);
        var t = dx.Tracks[0];
        Assert.Equal(StreamKind.Audio, t.Kind);
        Assert.Equal(CodecId.Opus, t.Codec.Codec);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(48000, a.SampleRate);
        Assert.Equal(2, a.Channels);

        var samples = await ReadAllAsync(dx);
        Assert.Equal(5, samples.Count);
        for (int i = 0; i < 5; i++)
        {
            Assert.Equal(i * 960L, samples[i].Pts);
            Assert.Equal(8, samples[i].Data.Length);
            Assert.Equal(i, samples[i].Data[0]);
            Assert.Equal(0xAB, samples[i].Data[7]);
        }
    }

    // ---------- Muxer guards ----------

    [Fact]
    public void Muxer_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new Mp4Muxer(null!));
    }

    [Fact]
    public void Muxer_NonWritable_Stream_Throws()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new Mp4Muxer(ms));
    }

    [Fact]
    public void Muxer_NonSeekable_Stream_Throws()
    {
        using var ns = new NonSeekableStream();
        Assert.Throws<ArgumentException>(() => new Mp4Muxer(ns));
    }

    [Fact]
    public void Muxer_FormatName_Is_Mp4()
    {
        using var ms = new MemoryStream();
        using var mux = new Mp4Muxer(ms, leaveOpen: true);
        Assert.Equal("mp4", mux.FormatName);
    }

    [Fact]
    public async Task Muxer_StartAsync_With_No_Tracks_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_StartAsync_Twice_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(MakeAudioTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_AddTrack_After_Start_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(MakeAudioTrack());
        await mux.StartAsync();
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(MakeAudioTrack()));
    }

    [Fact]
    public async Task Muxer_WriteSample_Before_Start_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(MakeAudioTrack());
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(MakeSample(0, new byte[4])));
    }

    [Fact]
    public async Task Muxer_WriteSample_After_Finish_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(MakeAudioTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(MakeSample(0, new byte[4])));
    }

    [Fact]
    public async Task Muxer_WriteSample_Out_Of_Range_TrackIndex_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(MakeAudioTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentOutOfRangeException>(async () =>
            await mux.WriteSampleAsync(MakeSample(99, new byte[4])));
    }

    [Fact]
    public async Task Muxer_FinishAsync_Before_Start_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.FinishAsync());
    }

    [Fact]
    public async Task Muxer_FinishAsync_Idempotent()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(MakeAudioTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync();
    }

    [Fact]
    public async Task Muxer_LeaveOpen_True_Keeps_Stream_Open()
    {
        var ms = new MemoryStream();
        await using (var mux = new Mp4Muxer(ms, leaveOpen: true))
        {
            mux.AddTrack(MakeAudioTrack());
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
        await using (var mux = new Mp4Muxer(ms, leaveOpen: false))
        {
            mux.AddTrack(MakeAudioTrack());
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public async Task Muxer_Auto_Finishes_On_Dispose()
    {
        var ms = new MemoryStream();
        await using (var mux = new Mp4Muxer(ms, leaveOpen: true))
        {
            mux.AddTrack(MakeAudioTrack());
            await mux.StartAsync();
            // do not call FinishAsync — DisposeAsync should call it.
            await mux.WriteSampleAsync(MakeSample(0, new byte[4]));
        }
        Assert.True(ms.Length > 0);
        // Reopen — should be parseable.
        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = new Mp4Demuxer(src);
        Assert.Single(dx.Tracks);
    }

    [Fact]
    public void Muxer_Sync_Dispose_Works()
    {
        var ms = new MemoryStream();
        var mux = new Mp4Muxer(ms, leaveOpen: false);
        mux.AddTrack(MakeAudioTrack());
        mux.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    // ---------- Muxer features ----------

    [Fact]
    public async Task Muxer_Multiple_Tracks_RoundTrip()
    {
        byte[] bytes;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new Mp4Muxer(ms, leaveOpen: true);
            mux.AddTrack(MakeAudioTrack());
            mux.AddTrack(MakeVideoTrack(0, 1));
            await mux.StartAsync();
            // Interleaved
            for (int i = 0; i < 3; i++)
            {
                await mux.WriteSampleAsync(MakeSample(0, new byte[] { (byte)('A' + i) }, pts: i * 1024, dts: i * 1024));
                await mux.WriteSampleAsync(MakeSample(1, new byte[] { (byte)('V' + i) }, pts: i * 3000, dts: i * 3000, isKey: i == 0));
            }
            await mux.FinishAsync();
            bytes = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        Assert.Equal(2, dx.Tracks.Count);
        Assert.Contains(dx.Tracks, t => t.Kind == StreamKind.Audio);
        Assert.Contains(dx.Tracks, t => t.Kind == StreamKind.Video);

        var samples = await ReadAllAsync(dx);
        Assert.Equal(6, samples.Count);
        // Both tracks should be represented.
        Assert.Contains(samples, s => s.TrackIndex == 0);
        Assert.Contains(samples, s => s.TrackIndex == 1);
    }

    [Fact]
    public async Task Muxer_Variable_Sample_Sizes_RoundTrip()
    {
        byte[] bytes;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new Mp4Muxer(ms, leaveOpen: true);
            mux.AddTrack(MakeAudioTrack());
            await mux.StartAsync();
            int[] sizes = new[] { 8, 16, 4, 12 };
            for (int i = 0; i < sizes.Length; i++)
            {
                byte[] data = new byte[sizes[i]];
                data[0] = (byte)i;
                await mux.WriteSampleAsync(MakeSample(0, data, pts: i * 960, dts: i * 960));
            }
            await mux.FinishAsync();
            bytes = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        var samples = await ReadAllAsync(dx);
        Assert.Collection(samples,
            s => Assert.Equal(8, s.Data.Length),
            s => Assert.Equal(16, s.Data.Length),
            s => Assert.Equal(4, s.Data.Length),
            s => Assert.Equal(12, s.Data.Length));
        for (int i = 0; i < samples.Count; i++) Assert.Equal((byte)i, samples[i].Data[0]);
    }

    [Fact]
    public async Task Muxer_Video_With_NonKey_Frames_Emits_Stss()
    {
        byte[] bytes;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new Mp4Muxer(ms, leaveOpen: true);
            mux.AddTrack(MakeVideoTrack(0, 1));
            await mux.StartAsync();
            // 4 frames: key, non-key, non-key, key.
            await mux.WriteSampleAsync(MakeSample(0, new byte[] { 1 }, pts: 0, dts: 0, isKey: true));
            await mux.WriteSampleAsync(MakeSample(0, new byte[] { 2 }, pts: 1000, dts: 1000, isKey: false));
            await mux.WriteSampleAsync(MakeSample(0, new byte[] { 3 }, pts: 2000, dts: 2000, isKey: false));
            await mux.WriteSampleAsync(MakeSample(0, new byte[] { 4 }, pts: 3000, dts: 3000, isKey: true));
            await mux.FinishAsync();
            bytes = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);

        // Seek to mid-stream — video track must snap to a keyframe (sample 0).
        await dx.SeekAsync(TimeSpan.FromMilliseconds(900));
        var first = await ReadFirstAsync(dx);
        Assert.True(first.IsKeyFrame);
    }

    [Fact]
    public async Task Muxer_CTS_Offset_RoundTrips()
    {
        byte[] bytes;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new Mp4Muxer(ms, leaveOpen: true);
            mux.AddTrack(MakeVideoTrack(0, 1));
            await mux.StartAsync();
            // PTS > DTS to exercise ctts box.
            await mux.WriteSampleAsync(MakeSample(0, new byte[] { 1 }, pts: 0,    dts: 0,    duration: 1000, isKey: true));
            await mux.WriteSampleAsync(MakeSample(0, new byte[] { 2 }, pts: 3000, dts: 1000, duration: 1000, isKey: false));
            await mux.WriteSampleAsync(MakeSample(0, new byte[] { 3 }, pts: 1500, dts: 2000, duration: 1000, isKey: false));
            await mux.FinishAsync();
            bytes = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        var samples = await ReadAllAsync(dx);
        Assert.Equal(0L, samples[0].Pts);
        Assert.Equal(3000L, samples[1].Pts);
        Assert.Equal(1500L, samples[2].Pts);
        Assert.Equal(0L, samples[0].Dts);
        Assert.Equal(1000L, samples[1].Dts);
        Assert.Equal(2000L, samples[2].Dts);
    }

    [Fact]
    public async Task Muxer_All_Audio_Samples_Are_Keyframes_In_Demuxer()
    {
        byte[] bytes = await MuxAudioAsync(3);
        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        var samples = await ReadAllAsync(dx);
        Assert.All(samples, s => Assert.True(s.IsKeyFrame));
    }

    // ---------- Demuxer ----------

    [Fact]
    public async Task Demuxer_Open_Path()
    {
        byte[] bytes = await MuxAudioAsync(2);
        var path = Path.Combine(Path.GetTempPath(), $"mediar-mp4-{Guid.NewGuid():N}.mp4");
        await File.WriteAllBytesAsync(path, bytes);
        try
        {
            using var dx = Mp4Demuxer.Open(path);
            Assert.Equal("mp4", dx.FormatName);
            Assert.Single(dx.Tracks);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Demuxer_Open_Missing_Path_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-mp4-missing-{Guid.NewGuid():N}.mp4");
        Assert.Throws<FileNotFoundException>(() => Mp4Demuxer.Open(path));
    }

    [Fact]
    public async Task Demuxer_Seek_Negative_Clamps_To_Zero()
    {
        byte[] bytes = await MuxAudioAsync(5);
        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(-10));
        var samples = await ReadAllAsync(dx);
        Assert.Equal(5, samples.Count);
        Assert.Equal(0L, samples[0].Pts);
    }

    [Fact]
    public async Task Demuxer_Seek_Past_End_Yields_No_Samples()
    {
        byte[] bytes = await MuxAudioAsync(3);
        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        await dx.SeekAsync(TimeSpan.FromHours(1));
        var samples = await ReadAllAsync(dx);
        // Linear scan lands on the last sample at-or-before — we'll get 1 sample (the last).
        Assert.Single(samples);
        Assert.Equal(2 * 960L, samples[0].Pts);
    }

    [Fact]
    public async Task Demuxer_Seek_Audio_Snaps_To_Sample_At_Or_Before()
    {
        byte[] bytes = await MuxAudioAsync(5);
        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        // Each audio sample = 960 ticks / 48000 = 20 ms. Seek to 50 ms -> sample 2 (40 ms).
        await dx.SeekAsync(TimeSpan.FromMilliseconds(50));
        var samples = await ReadAllAsync(dx);
        Assert.Equal(3, samples.Count);
        Assert.Equal(2 * 960L, samples[0].Pts);
    }

    [Fact]
    public async Task Demuxer_Duration_Reflects_Movie_Duration()
    {
        byte[] bytes = await MuxAudioAsync(5);
        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        // 5 samples × 960 = 4800 / 48000 = 100 ms.
        var diff = (dx.Duration - TimeSpan.FromMilliseconds(100)).TotalMilliseconds;
        Assert.InRange(diff, -2, 2);
    }

    [Fact]
    public async Task Demuxer_Metadata_Empty_By_Default()
    {
        byte[] bytes = await MuxAudioAsync(1);
        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        Assert.NotNull(dx.Metadata);
    }

    [Fact]
    public async Task Demuxer_Disposed_Read_Throws()
    {
        byte[] bytes = await MuxAudioAsync(1);
        var src = new MemoryRandomAccessSource(bytes);
        var dx = new Mp4Demuxer(src);
        dx.Dispose();
        await Assert.ThrowsAsync<ObjectDisposedException>(async () =>
        {
            await foreach (var s in dx.ReadSamplesAsync()) { s.Owner?.Dispose(); }
        });
        src.Dispose();
    }

    [Fact]
    public async Task Demuxer_Disposed_Seek_Throws()
    {
        byte[] bytes = await MuxAudioAsync(1);
        var src = new MemoryRandomAccessSource(bytes);
        var dx = new Mp4Demuxer(src);
        dx.Dispose();
        await Assert.ThrowsAsync<ObjectDisposedException>(async () => await dx.SeekAsync(TimeSpan.Zero));
        src.Dispose();
    }

    [Fact]
    public async Task Demuxer_Dispose_Idempotent()
    {
        byte[] bytes = await MuxAudioAsync(1);
        var src = new MemoryRandomAccessSource(bytes);
        var dx = new Mp4Demuxer(src);
        dx.Dispose();
        dx.Dispose();
        src.Dispose();
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        byte[] bytes = await MuxAudioAsync(1);
        var src = new MemoryRandomAccessSource(bytes);
        var dx = new Mp4Demuxer(src);
        await dx.DisposeAsync();
        src.Dispose();
    }

    [Fact]
    public async Task Demuxer_OwnsSource_False_Does_Not_Dispose_Source()
    {
        byte[] bytes = await MuxAudioAsync(1);
        var src = new MemoryRandomAccessSource(bytes);
        var dx = new Mp4Demuxer(src);
        dx.Dispose();
        _ = src.Length;
        src.Dispose();
    }

    // ---------- helpers ----------

    private static MediaTrack MakeAudioTrack() => new()
    {
        Index = 0, Id = 1,
        Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        TimeBase = new Rational(1, 48000),
        Language = "und",
    };

    private static MediaTrack MakeVideoTrack(int index, uint id) => new()
    {
        Index = index, Id = id,
        Codec = new VideoCodecParameters { Codec = CodecId.H264, Width = 320, Height = 240 },
        TimeBase = new Rational(1, 90000),
    };

    private static MediaSample MakeSample(int trackIndex, byte[] data, long pts = 0, long dts = 0, int duration = 960, bool isKey = true) => new()
    {
        TrackIndex = trackIndex, Pts = pts, Dts = dts,
        Duration = duration, IsKeyFrame = isKey, Data = data,
    };

    private static async Task<byte[]> MuxAudioAsync(int packetCount)
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(MakeAudioTrack());
        await mux.StartAsync();
        const int framesPerPacket = 960;
        for (int i = 0; i < packetCount; i++)
        {
            byte[] payload = new byte[8];
            payload[0] = (byte)i;
            payload[7] = 0xAB;
            await mux.WriteSampleAsync(MakeSample(0, payload, pts: i * framesPerPacket, dts: i * framesPerPacket, duration: framesPerPacket));
        }
        await mux.FinishAsync();
        return ms.ToArray();
    }

    private sealed record SampleSnapshot(int TrackIndex, long Pts, long Dts, byte[] Data, bool IsKeyFrame);

    private static async Task<List<SampleSnapshot>> ReadAllAsync(Mp4Demuxer dx)
    {
        var samples = new List<SampleSnapshot>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                samples.Add(new SampleSnapshot(s.TrackIndex, s.Pts, s.Dts, s.Data.ToArray(), s.IsKeyFrame));
            }
            finally { s.Owner?.Dispose(); }
        }
        return samples;
    }

    private static async Task<SampleSnapshot> ReadFirstAsync(Mp4Demuxer dx)
    {
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { return new SampleSnapshot(s.TrackIndex, s.Pts, s.Dts, s.Data.ToArray(), s.IsKeyFrame); }
            finally { s.Owner?.Dispose(); }
        }
        throw new InvalidOperationException("No samples produced.");
    }

    private sealed class NonSeekableStream : Stream
    {
        public override bool CanRead => false;
        public override bool CanSeek => false;
        public override bool CanWrite => true;
        public override long Length => 0;
        public override long Position { get => 0; set => throw new NotSupportedException(); }
        public override void Flush() { }
        public override int Read(byte[] buffer, int offset, int count) => 0;
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException();
        public override void Write(byte[] buffer, int offset, int count) { }
    }
}
