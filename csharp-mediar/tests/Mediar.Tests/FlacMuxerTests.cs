using Mediar.Containers.Flac;
using Xunit;

namespace Mediar.Tests;

public sealed class FlacMuxerTests
{
    private static MediaTrack BuildTrack(byte fillByte = 0x80)
    {
        byte[] streamInfo = new byte[34];
        for (int i = 0; i < 34; i++) streamInfo[i] = (byte)(fillByte + i);
        return new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Flac,
                SampleRate = 44100,
                Channels = 2,
                BitsPerSample = 16,
                ExtraData = streamInfo,
            },
            TimeBase = new Rational(1, 44100),
        };
    }

    private static MediaSample BuildFrame(int length = 256, int trackIndex = 0, byte syncByte1 = 0xF8)
    {
        byte[] data = new byte[length];
        data[0] = 0xFF;
        data[1] = syncByte1;
        return new MediaSample
        {
            TrackIndex = trackIndex,
            Pts = 0,
            Dts = 0,
            Duration = 4096,
            IsKeyFrame = true,
            Data = data,
        };
    }

    [Fact]
    public async Task Writes_FLaC_Marker_And_StreamInfo()
    {
        var track = BuildTrack();
        using var ms = new MemoryStream();
        await using (var mux = new FlacMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            await mux.WriteSampleAsync(BuildFrame());
            await mux.FinishAsync();
        }

        var bytes = ms.ToArray();
        Assert.Equal((byte)'f', bytes[0]);
        Assert.Equal((byte)'L', bytes[1]);
        Assert.Equal((byte)'a', bytes[2]);
        Assert.Equal((byte)'C', bytes[3]);
        // metadata-block header: last-block flag (0x80) | type=STREAMINFO (0).
        Assert.Equal(0x80, bytes[4]);
        // 24-bit big-endian length = 34.
        Assert.Equal(0x00, bytes[5]);
        Assert.Equal(0x00, bytes[6]);
        Assert.Equal(0x22, bytes[7]);
        // Frame payload follows the 42-byte header verbatim.
        Assert.Equal(0xFF, bytes[42]);
        Assert.Equal(0xF8, bytes[43]);
    }

    [Fact]
    public void FormatName_Is_Flac()
    {
        using var ms = new MemoryStream();
        using var mux = new FlacMuxer(ms);
        Assert.Equal("flac", mux.FormatName);
    }

    // ----- Constructor validation -----

    [Fact]
    public void Constructor_ThrowsOnNullStream()
    {
        Assert.Throws<ArgumentNullException>(() => new FlacMuxer(null!));
    }

    [Fact]
    public void Constructor_ThrowsOnReadOnlyStream()
    {
        var ro = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new FlacMuxer(ro));
    }

    // ----- AddTrack validation -----

    [Fact]
    public void AddTrack_ThrowsOnNull()
    {
        using var ms = new MemoryStream();
        using var mux = new FlacMuxer(ms);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public async Task AddTrack_AfterStart_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack()));
    }

    [Fact]
    public void AddTrack_Twice_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new FlacMuxer(ms);
        mux.AddTrack(BuildTrack());
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack()));
    }

    [Fact]
    public void AddTrack_NonFlacAudio_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new FlacMuxer(ms);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, 48000),
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Aac,
                ExtraData = new byte[34],
            },
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public void AddTrack_NonAudioCodec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new FlacMuxer(ms);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, 90000),
            Codec = new VideoCodecParameters
            {
                Codec = CodecId.H264,
            },
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(16)]
    [InlineData(33)]
    [InlineData(35)]
    public void AddTrack_WithBadStreamInfoLength_Throws(int extraDataLen)
    {
        using var ms = new MemoryStream();
        using var mux = new FlacMuxer(ms);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, 44100),
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Flac,
                ExtraData = new byte[extraDataLen],
            },
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    // ----- StartAsync validation -----

    [Fact]
    public async Task StartAsync_WithoutTrack_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    // ----- WriteSampleAsync validation -----

    [Fact]
    public async Task WriteSampleAsync_BeforeStart_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await Assert.ThrowsAsync<InvalidOperationException>(
            async () => await mux.WriteSampleAsync(BuildFrame()));
    }

    [Fact]
    public async Task WriteSampleAsync_AfterFinish_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(
            async () => await mux.WriteSampleAsync(BuildFrame()));
    }

    [Fact]
    public async Task WriteSampleAsync_NullSample_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentNullException>(
            async () => await mux.WriteSampleAsync(null!));
    }

    [Fact]
    public async Task WriteSampleAsync_WrongTrackIndex_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentException>(
            async () => await mux.WriteSampleAsync(BuildFrame(trackIndex: 1)));
    }

    [Theory]
    [InlineData(new byte[] { 0xFF, 0xF8, 0x00 })]              // too short (< 4 bytes)
    [InlineData(new byte[] { 0xFE, 0xF8, 0x00, 0x00 })]         // wrong leading byte
    [InlineData(new byte[] { 0xFF, 0xF0, 0x00, 0x00 })]         // top-5 bits not all 1
    [InlineData(new byte[] { 0xFF, 0xFA, 0x00, 0x00 })]         // reserved bit (bit 1) set
    [InlineData(new byte[] { 0xFF, 0xFE, 0x00, 0x00 })]         // reserved bit (bit 1) set
    public async Task WriteSampleAsync_RejectsInvalidSyncCode(byte[] data)
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        var sample = new MediaSample
        {
            TrackIndex = 0,
            Pts = 0,
            Dts = 0,
            Duration = 4096,
            IsKeyFrame = true,
            Data = data,
        };
        await Assert.ThrowsAsync<InvalidDataException>(
            async () => await mux.WriteSampleAsync(sample));
    }

    [Theory]
    [InlineData((byte)0xF8)]   // top-5 bits 1, blocking strategy 0
    [InlineData((byte)0xF9)]   // top-5 bits 1, blocking strategy 1
    [InlineData((byte)0xFC)]   // canonical fixed-blocking sync (top-6 bits 1)
    [InlineData((byte)0xFD)]   // canonical variable-blocking sync
    public async Task WriteSampleAsync_AcceptsValidSyncBytes(byte byte1)
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(BuildFrame(syncByte1: byte1));
    }

    // ----- FinishAsync semantics -----

    [Fact]
    public async Task FinishAsync_IsIdempotent()
    {
        using var ms = new MemoryStream();
        await using var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync(); // must not throw
    }

    // ----- Dispose semantics -----

    [Fact]
    public void Dispose_FlushesWhenNotFinished_LeavesStreamOpenWhenLeaveOpenTrue()
    {
        var ms = new MemoryStream();
        var mux = new FlacMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        // No Start; Dispose should still succeed and leave the stream usable.
        mux.Dispose();
        // Underlying stream not closed because leaveOpen=true.
        ms.WriteByte(0x42);
        Assert.True(ms.Length > 0);
    }

    [Fact]
    public async Task DisposeAsync_ClosesStreamWhenLeaveOpenFalse()
    {
        var ms = new MemoryStream();
        await using (var mux = new FlacMuxer(ms, leaveOpen: false))
        {
            mux.AddTrack(BuildTrack());
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        // After leaveOpen=false dispose, the stream should be closed.
        Assert.Throws<ObjectDisposedException>(() => ms.WriteByte(0x42));
    }

    // ----- Multi-frame round-trip -----

    [Fact]
    public async Task Writes_AllFramesContiguously()
    {
        using var ms = new MemoryStream();
        await using (var mux = new FlacMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
            await mux.StartAsync();
            for (int i = 0; i < 5; i++)
            {
                await mux.WriteSampleAsync(BuildFrame(length: 64));
            }
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        // 42-byte header + 5*64 = 320 + 42 = 362.
        Assert.Equal(42 + 5 * 64, bytes.Length);
        // Every frame should start with FF F8 at the expected offsets.
        for (int i = 0; i < 5; i++)
        {
            int off = 42 + i * 64;
            Assert.Equal(0xFF, bytes[off]);
            Assert.Equal(0xF8, bytes[off + 1]);
        }
    }
}

