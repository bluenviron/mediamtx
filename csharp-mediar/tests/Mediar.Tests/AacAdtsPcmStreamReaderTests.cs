using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacAdtsPcmStreamReaderTests
{
    // ----- constructor guards (forwarded from inner reader) -----

    [Fact]
    public void Ctor_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacAdtsPcmStreamReader(
            stream: null!,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullScaleFactorCodebook_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentNullException>(() => new AacAdtsPcmStreamReader(
            stream: ms,
            scaleFactorCodebook: null!,
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullSpectralCodebooks_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentNullException>(() => new AacAdtsPcmStreamReader(
            stream: ms,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: null!));
    }

    // ----- empty / EOF behaviour -----

    [Fact]
    public void ReadNextPcmFrame_EmptyStream_ReturnsNull()
    {
        using var ms = new MemoryStream();
        using var reader = NewReader(ms);
        Assert.Null(reader.ReadNextPcmFrame());
        Assert.Equal(0, reader.FrameCount);
        Assert.Null(reader.CurrentConfig);
    }

    // ----- happy-path single-block frame -----

    [Fact]
    public void ReadNextPcmFrame_SingleSceFrame_ProducesInterleavedMono()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var pcm = reader.ReadNextPcmFrame();
        Assert.NotNull(pcm);
        Assert.Equal(1, pcm!.ChannelCount);
        Assert.Equal(1024, pcm.SamplesPerChannel);
        Assert.Equal(1024, pcm.Samples.Length);
        Assert.Equal(48000, pcm.SampleRate);
        Assert.Single(pcm.Speakers);
        Assert.Equal(AacSpeaker.FrontCentre, pcm.Speakers[0]);

        // Empty SCE produces all-zero spectrum -> all-zero PCM frame.
        Assert.All(pcm.Samples, s => Assert.Equal(0f, s));

        // Second call returns null on clean EOF.
        Assert.Null(reader.ReadNextPcmFrame());
    }

    // ----- multi-block fan-out (one PCM frame per block) -----

    [Fact]
    public void ReadNextPcmFrame_MultiBlockFrame_YieldsOnePcmFramePerBlock()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoMultiSceFrameShared(blockCount: 3);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var p1 = reader.ReadNextPcmFrame();
        var p2 = reader.ReadNextPcmFrame();
        var p3 = reader.ReadNextPcmFrame();
        var eof = reader.ReadNextPcmFrame();

        Assert.NotNull(p1);
        Assert.NotNull(p2);
        Assert.NotNull(p3);
        Assert.Null(eof);

        Assert.Equal(1024, p1!.Samples.Length);
        Assert.Equal(1024, p2!.Samples.Length);
        Assert.Equal(1024, p3!.Samples.Length);
    }

    // ----- iterator wrapper -----

    [Fact]
    public void ReadPcmFrames_EnumeratesAllBlocksThenStops()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoMultiSceFrameShared(blockCount: 2);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        int count = 0;
        foreach (var p in reader.ReadPcmFrames())
        {
            Assert.Equal(1, p.ChannelCount);
            count++;
        }
        Assert.Equal(2, count);
    }

    // ----- ResetState -----

    [Fact]
    public void ResetState_AfterRead_ClearsCount()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        _ = reader.ReadNextPcmFrame();
        Assert.Equal(1, reader.FrameCount);

        reader.ResetState();
        Assert.Equal(0, reader.FrameCount);
        Assert.Null(reader.CurrentConfig);
    }

    // ----- Dispose -----

    [Fact]
    public void ReadNextPcmFrame_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => reader.ReadNextPcmFrame());
    }

    [Fact]
    public void Dispose_LeaveOpenFalse_DisposesStream()
    {
        var ms = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: false);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Dispose_LeaveOpenTrue_KeepsStreamOpen()
    {
        using var ms = new MemoryStream(new byte[] { 1, 2, 3 });
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        Assert.Equal(1, ms.ReadByte());
    }

    // ----- Int16 helpers -----

    [Fact]
    public void ReadNextInt16Frame_EmptyStream_ReturnsNull()
    {
        using var ms = new MemoryStream();
        using var reader = NewReader(ms);
        Assert.Null(reader.ReadNextInt16Frame());
    }

    [Fact]
    public void ReadNextInt16Frame_SingleSceFrame_AllZeroSamples()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var pcm = reader.ReadNextInt16Frame();
        Assert.NotNull(pcm);
        Assert.Equal(1, pcm!.ChannelCount);
        Assert.Equal(1024, pcm.SamplesPerChannel);
        Assert.Equal(1024, pcm.Samples.Length);
        Assert.Equal(48000, pcm.SampleRate);
        Assert.All(pcm.Samples, s => Assert.Equal((short)0, s));
    }

    [Fact]
    public void ReadInt16Frames_MultiBlock_YieldsPerBlock()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoMultiSceFrameShared(blockCount: 2);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        int count = 0;
        foreach (var p in reader.ReadInt16Frames())
        {
            Assert.Equal(1024, p.Samples.Length);
            count++;
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public void ReadNextInt16Frame_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => reader.ReadNextInt16Frame());
    }

    // ----- async surface -----

    [Fact]
    public async Task ReadNextPcmFrameAsync_EmptyStream_ReturnsNull()
    {
        using var ms = new MemoryStream();
        using var reader = NewReader(ms);
        Assert.Null(await reader.ReadNextPcmFrameAsync());
    }

    [Fact]
    public async Task ReadNextPcmFrameAsync_SingleFrame_MatchesSyncShape()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();

        using var msSync = new MemoryStream(frame);
        using var sync = NewReader(msSync);
        var syncFrame = sync.ReadNextPcmFrame();
        Assert.NotNull(syncFrame);

        using var msAsync = new MemoryStream(frame);
        using var async = NewReader(msAsync);
        var asyncFrame = await async.ReadNextPcmFrameAsync();
        Assert.NotNull(asyncFrame);

        Assert.Equal(syncFrame!.ChannelCount, asyncFrame!.ChannelCount);
        Assert.Equal(syncFrame.SamplesPerChannel, asyncFrame.SamplesPerChannel);
        Assert.Equal(syncFrame.SampleRate, asyncFrame.SampleRate);
        Assert.Equal(syncFrame.Samples.Length, asyncFrame.Samples.Length);
    }

    [Fact]
    public async Task ReadPcmFramesAsync_TwoFrames_YieldsBoth()
    {
        byte[] a = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[a.Length * 2];
        Buffer.BlockCopy(a, 0, payload, 0, a.Length);
        Buffer.BlockCopy(a, 0, payload, a.Length, a.Length);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        int count = 0;
        await foreach (var f in reader.ReadPcmFramesAsync())
        {
            Assert.NotNull(f);
            count++;
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public async Task ReadNextInt16FrameAsync_SingleFrame_ReturnsInt16Frame()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);
        var int16 = await reader.ReadNextInt16FrameAsync();
        Assert.NotNull(int16);
        Assert.Equal(1, int16!.ChannelCount);
        Assert.Equal(1024, int16.SamplesPerChannel);
    }

    [Fact]
    public async Task ReadInt16FramesAsync_TwoFrames_YieldsBoth()
    {
        byte[] a = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[a.Length * 2];
        Buffer.BlockCopy(a, 0, payload, 0, a.Length);
        Buffer.BlockCopy(a, 0, payload, a.Length, a.Length);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        int count = 0;
        await foreach (var f in reader.ReadInt16FramesAsync())
        {
            Assert.NotNull(f);
            count++;
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public async Task ReadNextPcmFrameAsync_Cancelled_Throws()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(
            async () => await reader.ReadNextPcmFrameAsync(cts.Token));
    }

    [Fact]
    public async Task ReadNextPcmFrameAsync_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        await Assert.ThrowsAsync<ObjectDisposedException>(
            async () => await reader.ReadNextPcmFrameAsync());
    }

    // ----- seek surface (forwards inner reader) -----

    [Fact]
    public void CanSeek_DefaultMemoryStream_True()
    {
        using var ms = new MemoryStream();
        using var reader = NewReader(ms);
        Assert.True(reader.CanSeek);
    }

    [Fact]
    public void SeekToFrame_Entry_RepositionsUnderlyingStream()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[frame.Length * 3];
        for (int i = 0; i < 3; i++)
        {
            Buffer.BlockCopy(frame, 0, payload, i * frame.Length, frame.Length);
        }

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        // Consume all 3 to drain the buffer state.
        Assert.NotNull(reader.ReadNextPcmFrame());
        Assert.NotNull(reader.ReadNextPcmFrame());
        Assert.NotNull(reader.ReadNextPcmFrame());

        var entry = new AacAdtsFrameIndexEntry
        {
            ByteOffset = frame.Length,
            FrameLength = frame.Length,
            BlockCount = 1,
            SampleOffset = 1024,
            SampleRate = 48000,
            ChannelConfiguration = 1,
        };
        reader.SeekToFrame(entry);

        Assert.Equal(frame.Length, (int)ms.Position);
        Assert.NotNull(reader.ReadNextPcmFrame());
        Assert.NotNull(reader.ReadNextPcmFrame());
        Assert.Null(reader.ReadNextPcmFrame());
    }

    [Fact]
    public void SeekToFrame_IndexLookup_HitsBracketingEntry()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[frame.Length * 3];
        for (int i = 0; i < 3; i++)
        {
            Buffer.BlockCopy(frame, 0, payload, i * frame.Length, frame.Length);
        }

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0, FrameLength = frame.Length, BlockCount = 1, SampleOffset = 0, SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = frame.Length, FrameLength = frame.Length, BlockCount = 1, SampleOffset = 1024, SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = frame.Length * 2, FrameLength = frame.Length, BlockCount = 1, SampleOffset = 2048, SampleRate = 48000, ChannelConfiguration = 1 },
        };

        var hit = reader.SeekToFrame(index, sampleTarget: 1500);
        Assert.NotNull(hit);
        Assert.Equal(frame.Length, (int)hit!.ByteOffset);
        Assert.Equal(frame.Length, (int)ms.Position);
    }

    [Fact]
    public void SeekToFrame_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        var entry = new AacAdtsFrameIndexEntry
        {
            ByteOffset = 0,
            FrameLength = 64,
            BlockCount = 1,
            SampleOffset = 0,
            SampleRate = 48000,
            ChannelConfiguration = 1,
        };
        Assert.Throws<ObjectDisposedException>(() => reader.SeekToFrame(entry));
    }

    // ----- async dispose -----

    [Fact]
    public async Task DisposeAsync_DisposesUnderlyingStream()
    {
        var inner = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(inner, GetSf(), new AacHuffmanCodebook?[16]);
        await reader.DisposeAsync();
        // Subsequent reads must throw because the inner reader is disposed.
        await Assert.ThrowsAsync<ObjectDisposedException>(
            async () => await reader.ReadNextPcmFrameAsync());
    }

    [Fact]
    public async Task DisposeAsync_Idempotent()
    {
        var inner = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(inner, GetSf(), new AacHuffmanCodebook?[16]);
        await reader.DisposeAsync();
        await reader.DisposeAsync(); // must not throw
    }

    // ----- helpers -----

    private static AacHuffmanCodebook GetSf() =>
        AacRawDataBlockTests.GetSharedSyntheticSfCodebook();

    private static AacAdtsPcmStreamReader NewReader(Stream s) =>
        new(s, GetSf(), new AacHuffmanCodebook?[16]);
}
