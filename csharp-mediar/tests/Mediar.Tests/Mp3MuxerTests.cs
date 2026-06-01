using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

public sealed class Mp3MuxerTests
{
    private static MediaTrack BuildTrack(int index = 0) => new()
    {
        Index = index,
        Id = 1,
        Codec = new AudioCodecParameters { Codec = CodecId.Mp3, SampleRate = 44100, Channels = 2 },
        TimeBase = new Rational(1, 44100),
    };

    private static byte[] BuildFrame(int length = 417, byte b1 = 0xFB)
    {
        var f = new byte[length];
        f[0] = 0xFF;
        f[1] = b1;
        f[2] = 0x90;
        f[3] = 0x00;
        return f;
    }

    private static MediaSample BuildSample(byte[] data, int trackIndex = 0) => new()
    {
        TrackIndex = trackIndex,
        Pts = 0,
        Dts = 0,
        Duration = 1152,
        IsKeyFrame = true,
        Data = data,
    };

    [Fact]
    public async Task WriteSample_With_Sync_Succeeds()
    {
        using var ms = new MemoryStream();
        await using (var mux = new Mp3Muxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
            await mux.StartAsync();
            await mux.WriteSampleAsync(BuildSample(BuildFrame()));
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        Assert.Equal(417, bytes.Length);
        Assert.Equal(0xFF, bytes[0]);
        Assert.Equal(0xFB, bytes[1]);
    }

    [Fact]
    public async Task WriteSample_Without_Sync_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        var ex = await Assert.ThrowsAnyAsync<Exception>(async () =>
            await mux.WriteSampleAsync(BuildSample(new byte[] { 0x00, 0x00, 0x00, 0x00 })));
        Assert.Contains("sync", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Constructor_Null_Output_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new Mp3Muxer(null!));
    }

    [Fact]
    public void Constructor_NonWritable_Stream_Throws()
    {
        using var ms = new MemoryStream(new byte[4], writable: false);
        Assert.Throws<ArgumentException>(() => new Mp3Muxer(ms));
    }

    [Fact]
    public void FormatName_Is_Mp3()
    {
        using var ms = new MemoryStream();
        using var mux = new Mp3Muxer(ms, leaveOpen: true);
        Assert.Equal("mp3", mux.FormatName);
    }

    [Fact]
    public void AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new Mp3Muxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public async Task AddTrack_After_Start_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack()));
    }

    [Fact]
    public void AddTrack_Twice_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack(index: 1)));
    }

    [Fact]
    public void AddTrack_With_NonMp3_Audio_Codec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new Mp3Muxer(ms, leaveOpen: true);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Aac, SampleRate = 44100, Channels = 2 },
            TimeBase = new Rational(1, 44100),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public void AddTrack_With_Video_Codec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new Mp3Muxer(ms, leaveOpen: true);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new VideoCodecParameters { Codec = CodecId.H264, Width = 640, Height = 480 },
            TimeBase = new Rational(1, 30),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public async Task StartAsync_Without_Track_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task WriteSampleAsync_Before_Start_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(BuildSample(BuildFrame())));
    }

    [Fact]
    public async Task WriteSampleAsync_After_Finish_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(BuildSample(BuildFrame())));
    }

    [Fact]
    public async Task WriteSampleAsync_Null_Sample_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentNullException>(async () =>
            await mux.WriteSampleAsync(null!));
    }

    [Fact]
    public async Task WriteSampleAsync_Wrong_TrackIndex_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentException>(async () =>
            await mux.WriteSampleAsync(BuildSample(BuildFrame(), trackIndex: 99)));
    }

    [Theory]
    [InlineData(new byte[] { 0xFF, 0xFB, 0x90 })]                 // too short
    [InlineData(new byte[] { 0xFE, 0xFB, 0x90, 0x00 })]           // bad byte0
    [InlineData(new byte[] { 0xFF, 0x0F, 0x90, 0x00 })]           // bits 7-5 of byte1 not 111
    [InlineData(new byte[] { 0xFF, 0x80, 0x00, 0x00 })]           // byte1 1000_0000 → (b1 & 0xE0)=0x80
    [InlineData(new byte[] { 0xFF, 0xC0, 0x00, 0x00 })]           // byte1 1100_0000 → (b1 & 0xE0)=0xC0
    public async Task WriteSampleAsync_Invalid_Sync_Throws(byte[] data)
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<InvalidDataException>(async () =>
            await mux.WriteSampleAsync(BuildSample(data)));
    }

    [Theory]
    [InlineData(0xE0)] // 1110_0000
    [InlineData(0xE3)] // 1110_0011
    [InlineData(0xF3)] // 1111_0011
    [InlineData(0xFB)] // 1111_1011
    [InlineData(0xFF)] // 1111_1111
    public async Task WriteSampleAsync_Valid_Sync_Bytes_Pass(byte b1)
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(BuildSample(BuildFrame(length: 32, b1: b1)));
        var bytes = ms.ToArray();
        Assert.Equal(32, bytes.Length);
        Assert.Equal(b1, bytes[1]);
    }

    [Fact]
    public async Task Multiple_Frames_Are_Concatenated_In_Order()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        for (int i = 0; i < 5; i++)
        {
            var f = BuildFrame(length: 64);
            f[10] = (byte)i;
            await mux.WriteSampleAsync(BuildSample(f));
        }
        await mux.FinishAsync();
        var bytes = ms.ToArray();
        Assert.Equal(5 * 64, bytes.Length);
        for (int i = 0; i < 5; i++)
        {
            Assert.Equal((byte)i, bytes[i * 64 + 10]);
        }
    }

    [Fact]
    public async Task FinishAsync_Is_Idempotent()
    {
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync(); // should not throw
    }

    [Fact]
    public async Task Dispose_With_LeaveOpen_True_Keeps_Stream_Usable()
    {
        var ms = new MemoryStream();
        var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(BuildSample(BuildFrame(length: 8)));
        mux.Dispose();
        // Stream still writable
        ms.WriteByte(0xAB);
        ms.Dispose();
    }

    [Fact]
    public void Dispose_With_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        var mux = new Mp3Muxer(ms);
        mux.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.WriteByte(0xAB));
    }

    [Fact]
    public async Task DisposeAsync_With_LeaveOpen_True_Keeps_Stream_Usable()
    {
        var ms = new MemoryStream();
        var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(BuildSample(BuildFrame(length: 8)));
        await mux.DisposeAsync();
        ms.WriteByte(0xAB);
        await ms.DisposeAsync();
    }

    [Fact]
    public async Task DisposeAsync_With_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        var mux = new Mp3Muxer(ms);
        await mux.DisposeAsync();
        Assert.Throws<ObjectDisposedException>(() => ms.WriteByte(0xAB));
    }

    [Fact]
    public async Task Dispose_Without_Finish_Flushes_Pending_Writes()
    {
        var ms = new MemoryStream();
        var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(BuildSample(BuildFrame(length: 16)));
        mux.Dispose();
        Assert.Equal(16, ms.ToArray().Length);
        ms.Dispose();
    }
}
