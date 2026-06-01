using Mediar.Containers.Mp2;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class Mp2DemuxerTests
{
    // Layer-II MPEG-1 @ 48 kHz / 128 kbps / mono. Frame size = 384 bytes.
    private const byte L2H1 = 0xFD;
    private const byte L2H2 = 0x84;
    private const byte L2H3Mono = 0xC0;
    private const byte L2H3Stereo = 0x00;
    private const int L2FrameSize = 384;
    private const int L2Samples = 1152;

    // Layer-I MPEG-1 @ 48 kHz / 128 kbps / mono. Frame size = 128 bytes.
    private const byte L1H1 = 0xFF;
    private const byte L1H2 = 0x44;
    private const byte L1H3Mono = 0xC0;
    private const int L1FrameSize = 128;
    private const int L1Samples = 384;

    private static byte[] BuildL2Stream(int frameCount, byte h3 = L2H3Mono)
    {
        byte[] bytes = new byte[L2FrameSize * frameCount];
        for (int f = 0; f < frameCount; f++)
        {
            int o = f * L2FrameSize;
            bytes[o + 0] = 0xFF;
            bytes[o + 1] = L2H1;
            bytes[o + 2] = L2H2;
            bytes[o + 3] = h3;
            for (int i = 4; i < L2FrameSize; i++) bytes[o + i] = (byte)(i & 0xFF);
        }
        return bytes;
    }

    private static byte[] BuildL1Stream(int frameCount)
    {
        byte[] bytes = new byte[L1FrameSize * frameCount];
        for (int f = 0; f < frameCount; f++)
        {
            int o = f * L1FrameSize;
            bytes[o + 0] = 0xFF;
            bytes[o + 1] = L1H1;
            bytes[o + 2] = L1H2;
            bytes[o + 3] = L1H3Mono;
            for (int i = 4; i < L1FrameSize; i++) bytes[o + i] = (byte)(i & 0xFF);
        }
        return bytes;
    }

    [Fact]
    public async Task Reads_Mp2_Frames_And_Reports_Codec()
    {
        byte[] stream = BuildL2Stream(4);
        using var src = new MemoryRandomAccessSource(stream);
        using var dx = Mp2Demuxer.Open(src);

        Assert.Equal("mp2", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.Mp2, a.Codec);
        Assert.Equal(48000, a.SampleRate);
        Assert.Equal(1, a.Channels);

        int seen = 0;
        long lastDuration = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(L2FrameSize, s.Data.Length);
                lastDuration = s.Duration;
                seen++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(4, seen);
        Assert.Equal(L2Samples, lastDuration);
    }

    [Fact]
    public void Refuses_Layer_III_Stream()
    {
        byte[] frame = new byte[417];
        frame[0] = 0xFF;
        frame[1] = 0xFB;
        frame[2] = 0x90;
        frame[3] = 0x00;
        using var src = new MemoryRandomAccessSource(frame);
        Assert.Throws<InvalidDataException>(() => Mp2Demuxer.Open(src));
    }

    [Fact]
    public async Task Layer_I_Stream_Reports_Mp1_Codec()
    {
        byte[] stream = BuildL1Stream(2);
        using var src = new MemoryRandomAccessSource(stream);
        using var dx = Mp2Demuxer.Open(src);
        var a = (AudioCodecParameters)dx.Tracks[0].Codec;
        Assert.Equal(CodecId.Mp1, a.Codec);
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(L1FrameSize, s.Data.Length);
                Assert.Equal(L1Samples, s.Duration);
                seen++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(2, seen);
    }

    [Fact]
    public async Task Stereo_Header_Reports_Two_Channels()
    {
        byte[] stream = BuildL2Stream(1, L2H3Stereo);
        using var src = new MemoryRandomAccessSource(stream);
        using var dx = Mp2Demuxer.Open(src);
        var a = (AudioCodecParameters)dx.Tracks[0].Codec;
        Assert.Equal(2, a.Channels);
        await Task.CompletedTask;
    }

    [Fact]
    public void Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => Mp2Demuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Empty_Stream_Throws()
    {
        using var src = new MemoryRandomAccessSource(Array.Empty<byte>());
        Assert.Throws<InvalidDataException>(() => Mp2Demuxer.Open(src));
    }

    [Fact]
    public void Garbage_Only_Stream_Throws()
    {
        using var src = new MemoryRandomAccessSource(new byte[] { 0, 1, 2, 3, 4, 5, 6, 7, 8, 9 });
        Assert.Throws<InvalidDataException>(() => Mp2Demuxer.Open(src));
    }

    [Fact]
    public async Task Leading_Garbage_Before_Sync_Is_Skipped()
    {
        byte[] frames = BuildL2Stream(2);
        byte[] padded = new byte[16 + frames.Length];
        for (int i = 0; i < 16; i++) padded[i] = (byte)i; // junk
        Array.Copy(frames, 0, padded, 16, frames.Length);
        using var src = new MemoryRandomAccessSource(padded);
        using var dx = Mp2Demuxer.Open(src);
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(2, seen);
    }

    [Fact]
    public async Task Truncated_Final_Frame_Is_Dropped()
    {
        byte[] frames = BuildL2Stream(3);
        // Cut 20 bytes off the final frame.
        byte[] truncated = frames.AsSpan(0, frames.Length - 20).ToArray();
        using var src = new MemoryRandomAccessSource(truncated);
        using var dx = Mp2Demuxer.Open(src);
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(2, seen);
    }

    [Fact]
    public async Task Sample_Pts_Increments_By_Samples_Per_Frame()
    {
        byte[] frames = BuildL2Stream(3);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        var ptsList = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { ptsList.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(new long[] { 0, L2Samples, 2 * L2Samples }, ptsList);
    }

    [Fact]
    public async Task Sample_Is_Key_Frame_And_TrackIndex_Zero()
    {
        byte[] frames = BuildL2Stream(1);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.True(s.IsKeyFrame);
                Assert.Equal(0, s.TrackIndex);
                Assert.Equal(s.Pts, s.Dts);
            }
            finally { s.Owner?.Dispose(); }
        }
    }

    [Fact]
    public async Task Seek_Negative_Clamps_To_Zero()
    {
        byte[] frames = BuildL2Stream(3);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(-1));
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(3, seen);
    }

    [Fact]
    public async Task Seek_Past_End_Yields_No_Samples()
    {
        byte[] frames = BuildL2Stream(3);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromHours(1));
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(0, seen);
    }

    [Fact]
    public async Task Seek_To_Middle_Skips_Earlier_Frames()
    {
        // 5 frames at 1152 samples/frame = 5760 samples = 0.12 seconds at 48k.
        // Seek to 1.5 frames worth (≈36 ms) → skip frame 0 + 1, deliver 2/3/4.
        byte[] frames = BuildL2Stream(5);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(1.5 * L2Samples / 48000.0));
        var ptsList = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { ptsList.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(new long[] { L2Samples, 2 * L2Samples, 3 * L2Samples, 4 * L2Samples }, ptsList);
    }

    [Fact]
    public void Metadata_Is_Empty()
    {
        byte[] frames = BuildL2Stream(1);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        Assert.Same(MediaMetadata.Empty, dx.Metadata);
    }

    [Fact]
    public void Duration_Is_Zero()
    {
        byte[] frames = BuildL2Stream(3);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        Assert.Equal(TimeSpan.Zero, dx.Duration);
    }

    [Fact]
    public void Track_Has_TimeBase_Matching_SampleRate()
    {
        byte[] frames = BuildL2Stream(1);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        var t = dx.Tracks[0];
        Assert.Equal(1, t.TimeBase.Numerator);
        Assert.Equal(48000, t.TimeBase.Denominator);
    }

    [Fact]
    public void Open_Path_Works()
    {
        byte[] frames = BuildL2Stream(2);
        var path = Path.Combine(Path.GetTempPath(), $"mediar-mp2-{Guid.NewGuid():N}.mp2");
        File.WriteAllBytes(path, frames);
        try
        {
            using var dx = Mp2Demuxer.Open(path);
            Assert.Single(dx.Tracks);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-mp2-missing-{Guid.NewGuid():N}.mp2");
        Assert.Throws<FileNotFoundException>(() => Mp2Demuxer.Open(path));
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        byte[] frames = BuildL2Stream(1);
        var dx = Mp2Demuxer.Open(new MemoryRandomAccessSource(frames), ownsSource: true);
        dx.Dispose();
        dx.Dispose();
    }

    [Fact]
    public async Task DisposeAsync_Works()
    {
        byte[] frames = BuildL2Stream(1);
        var dx = Mp2Demuxer.Open(new MemoryRandomAccessSource(frames), ownsSource: true);
        await dx.DisposeAsync();
    }

    [Fact]
    public async Task Dispose_With_OwnsSource_False_Leaves_Source_Open()
    {
        byte[] frames = BuildL2Stream(2);
        var src = new MemoryRandomAccessSource(frames);
        var dx = Mp2Demuxer.Open(src, ownsSource: false);
        dx.Dispose();
        // source still usable for the second pass
        using var dx2 = Mp2Demuxer.Open(src);
        int seen = 0;
        await foreach (var s in dx2.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(2, seen);
    }

    [Fact]
    public async Task ReadSamplesAsync_Cancellation_Throws()
    {
        byte[] frames = BuildL2Stream(10);
        using var src = new MemoryRandomAccessSource(frames);
        using var dx = Mp2Demuxer.Open(src);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var s in dx.ReadSamplesAsync(cts.Token))
            {
                s.Owner?.Dispose();
            }
        });
    }
}
