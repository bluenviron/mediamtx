using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacAdtsStreamReaderTests
{
    // ----- constructor guards -----

    [Fact]
    public void Ctor_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacAdtsStreamReader(
            stream: null!,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullScaleFactorCodebook_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentNullException>(() => new AacAdtsStreamReader(
            stream: ms,
            scaleFactorCodebook: null!,
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullSpectralCodebooks_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentNullException>(() => new AacAdtsStreamReader(
            stream: ms,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: null!));
    }

    [Fact]
    public void Ctor_WriteOnlyStream_Throws()
    {
        using var sink = new WriteOnlyStream();
        Assert.Throws<ArgumentException>(() => new AacAdtsStreamReader(
            stream: sink,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_InitialBufferSizeTooSmall_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentOutOfRangeException>(() => new AacAdtsStreamReader(
            stream: ms,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16],
            initialBufferSize: 4));
    }

    // ----- happy paths -----

    [Fact]
    public void ReadNextFrame_EmptyStream_ReturnsNull()
    {
        using var ms = new MemoryStream();
        using var reader = NewReader(ms);
        Assert.Null(reader.ReadNextFrame());
        Assert.Equal(0, reader.FrameCount);
    }

    [Fact]
    public void ReadNextFrame_SingleFrame_DecodesAndPopulatesState()
    {
        var frame = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var block = reader.ReadNextFrame();

        Assert.NotNull(block);
        Assert.Single(block!.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, block.Channels[0].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, block.Channels[0].Samples.Length);
        Assert.Equal(1, reader.FrameCount);
        Assert.Equal(48_000, reader.CurrentSampleRate);
        Assert.Equal(1, reader.CurrentChannelCount);
    }

    [Fact]
    public void ReadNextFrame_SingleShortFrame_DecodesAndPopulatesState()
    {
        // Reader has to forward EightShort raw_data_blocks through the inner
        // AacAdtsFrameDecoder. The PCM output length and frame counter must
        // match the long-window contract.
        var frame = BuildAdtsMonoShortSceFrame();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var block = reader.ReadNextFrame();

        Assert.NotNull(block);
        Assert.Single(block!.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, block.Channels[0].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, block.Channels[0].Samples.Length);
        Assert.Equal(1, reader.FrameCount);
        Assert.Equal(48_000, reader.CurrentSampleRate);
    }

    [Fact]
    public void ReadNextFrame_MixedLongAndShortFrames_BothDecodedInOrder()
    {
        // A long frame followed by a short frame at the same ADTS header
        // exercises the inner decoder's per-frame window-sequence reset.
        var longFrame = BuildAdtsMonoSceFrame();
        var shortFrame = BuildAdtsMonoShortSceFrame();
        using var ms = new MemoryStream(Concat(longFrame, shortFrame));
        using var reader = NewReader(ms);

        var b1 = reader.ReadNextFrame();
        var b2 = reader.ReadNextFrame();

        Assert.NotNull(b1);
        Assert.NotNull(b2);
        Assert.Null(reader.ReadNextFrame());
        Assert.Equal(2, reader.FrameCount);
    }

    [Fact]
    public void ReadNextFrame_MultiBlockShortAdtsFrame_YieldsEachShortBlockSeparately()
    {
        // One ADTS frame whose number_of_raw_data_blocks=2 (rdbInFrame=1)
        // and both blocks are EightShort SCEs.
        byte[] frame = BuildAdtsMonoMultiShortSceFrame(blockCount: 2);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var b1 = reader.ReadNextFrame();
        var b2 = reader.ReadNextFrame();
        var eof = reader.ReadNextFrame();

        Assert.NotNull(b1);
        Assert.NotNull(b2);
        Assert.Null(eof);
        Assert.All(new[] { b1!, b2! }, b =>
        {
            Assert.Single(b.Channels);
            Assert.Equal(AacSpeaker.FrontCentre, b.Channels[0].Speaker);
        });
        Assert.Equal(2, reader.FrameCount);
    }

    [Fact]
    public void ReadNextFrame_ThreeFramesBackToBack_AllDecodedThenNull()
    {
        var frame = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(Concat(frame, frame, frame));
        using var reader = NewReader(ms);

        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.Null(reader.ReadNextFrame());
        Assert.Equal(3, reader.FrameCount);
    }

    [Fact]
    public void ReadFrames_EnumeratesAllFrames()
    {
        var frame = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(Concat(frame, frame, frame, frame));
        using var reader = NewReader(ms);

        int count = 0;
        foreach (var block in reader.ReadFrames())
        {
            Assert.Single(block.Channels);
            count++;
        }

        Assert.Equal(4, count);
        Assert.Equal(4, reader.FrameCount);
    }

    [Fact]
    public void ReadNextFrame_SmallBuffer_StillDecodesAllFramesViaCompaction()
    {
        // Three small frames in a 16-byte buffer guarantees that the
        // reader must compact / refill between every frame.
        var frame = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(Concat(frame, frame, frame));
        using var reader = new AacAdtsStreamReader(
            ms, GetSf(), new AacHuffmanCodebook?[16],
            initialBufferSize: 16);

        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.Null(reader.ReadNextFrame());
        Assert.Equal(3, reader.FrameCount);
    }

    [Fact]
    public void ReadNextFrame_ChunkedStream_StillDecodesEveryFrame()
    {
        var frame = BuildAdtsMonoSceFrame();
        using var ms = new ChunkingStream(Concat(frame, frame), maxBytesPerRead: 3);
        using var reader = NewReader(ms);

        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.Null(reader.ReadNextFrame());
    }

    [Fact]
    public void ReadNextFrame_StreamWithLeadingId3v2_SkipsTagAndDecodes()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        byte[] id3 = BuildId3v2Tag(payloadSize: 64);
        using var ms = new MemoryStream(Concat(id3, frame));
        using var reader = NewReader(ms);

        var block = reader.ReadNextFrame();
        Assert.NotNull(block);
        Assert.Equal(1, reader.FrameCount);
    }

    [Fact]
    public void ReadNextFrame_LeadingId3v2LargerThanBuffer_StillSkips()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        byte[] id3 = BuildId3v2Tag(payloadSize: 2000);
        using var ms = new MemoryStream(Concat(id3, frame));
        using var reader = new AacAdtsStreamReader(
            ms, GetSf(), new AacHuffmanCodebook?[16],
            initialBufferSize: 128);

        var block = reader.ReadNextFrame();
        Assert.NotNull(block);
    }

    // ----- failure paths -----

    [Fact]
    public void ReadNextFrame_LostSync_ThrowsInvalidData()
    {
        byte[] garbage = new byte[32];
        garbage[0] = 0x12;
        using var ms = new MemoryStream(garbage);
        using var reader = NewReader(ms);

        Assert.Throws<InvalidDataException>(() => reader.ReadNextFrame());
    }

    [Fact]
    public void ReadNextFrame_StreamEndsMidFrame_ThrowsInvalidData()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        // Keep only first 12 bytes — header parses, body is truncated.
        byte[] truncated = frame.AsSpan(0, 12).ToArray();
        using var ms = new MemoryStream(truncated);
        using var reader = NewReader(ms);

        var ex = Assert.Throws<InvalidDataException>(() => reader.ReadNextFrame());
        Assert.Contains("only", ex.Message);
    }

    [Fact]
    public void ReadNextFrame_TrailingPartialHeader_ThrowsInvalidData()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        // Whole frame + 3 leftover bytes that aren't a complete ADTS header.
        byte[] data = Concat(frame, new byte[] { 0xFF, 0xF1, 0x40 });
        using var ms = new MemoryStream(data);
        using var reader = NewReader(ms);

        Assert.NotNull(reader.ReadNextFrame());
        var ex = Assert.Throws<InvalidDataException>(() => reader.ReadNextFrame());
        Assert.Contains("unconsumed bytes", ex.Message);
    }

    // ----- lifecycle -----

    [Fact]
    public void ResetState_DropsDecoderStateAndBuffer()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(Concat(frame, frame));
        using var reader = NewReader(ms);

        _ = reader.ReadNextFrame();
        Assert.NotNull(reader.CurrentConfig);
        Assert.Equal(1, reader.FrameCount);

        ms.Position = 0;          // simulate caller seek
        reader.ResetState();

        Assert.Null(reader.CurrentConfig);
        Assert.Equal(0, reader.FrameCount);

        var block = reader.ReadNextFrame();
        Assert.NotNull(block);
        Assert.Equal(1, reader.FrameCount);
    }

    [Fact]
    public void Dispose_DisposesOwnedStream()
    {
        var ms = new MemoryStream();
        var reader = new AacAdtsStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16]);
        reader.Dispose();
        Assert.False(ms.CanRead);
    }

    [Fact]
    public void Dispose_LeaveOpenTrue_LeavesStreamOpen()
    {
        using var ms = new MemoryStream();
        var reader = new AacAdtsStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        Assert.True(ms.CanRead);
    }

    [Fact]
    public void ReadNextFrame_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = new AacAdtsStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => reader.ReadNextFrame());
    }

    [Fact]
    public void ReadNextFrame_MultiBlockAdtsFrame_YieldsEachBlockSeparately()
    {
        byte[] frame = BuildAdtsMonoMultiSceFrame(blockCount: 3);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var block1 = reader.ReadNextFrame();
        var block2 = reader.ReadNextFrame();
        var block3 = reader.ReadNextFrame();
        var eof = reader.ReadNextFrame();

        Assert.NotNull(block1);
        Assert.NotNull(block2);
        Assert.NotNull(block3);
        Assert.Null(eof);
        Assert.All(new[] { block1!, block2!, block3! }, b =>
        {
            Assert.Single(b.Channels);
            Assert.Equal(AacSpeaker.FrontCentre, b.Channels[0].Speaker);
        });
        // FrameCount on the inner decoder counts every decoded
        // raw_data_block — three for one multi-block ADTS frame.
        Assert.Equal(3, reader.FrameCount);
    }

    [Fact]
    public void ReadFrames_MultiBlockEnumerator_YieldsAllInOrder()
    {
        byte[] frame = BuildAdtsMonoMultiSceFrame(blockCount: 2);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        int count = 0;
        foreach (var b in reader.ReadFrames())
        {
            Assert.Single(b.Channels);
            count++;
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public void ResetState_ClearsPendingMultiBlockQueue()
    {
        byte[] frameA = BuildAdtsMonoMultiSceFrame(blockCount: 3);
        byte[] frameB = BuildAdtsMonoSceFrame();
        byte[] payload = Concat(frameA, frameB);
        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        // Pull one block from the 3-block frame; the remaining two
        // sit on the pending queue. FrameCount reflects the inner
        // decoder's block counter, which counted all three blocks
        // when DecodeBlocks drained the ADTS frame.
        _ = reader.ReadNextFrame();
        Assert.Equal(3, reader.FrameCount);

        // ResetState must drop those pending blocks; the next read
        // pulls from the buffered second frame in the stream rather
        // than the queue. But because we don't rewind the stream,
        // the next ReadNextFrame may produce InvalidData (stream
        // mid-frame). The assertion here is just that ResetState
        // does not throw, and that FrameCount + buffer state are
        // both cleared.
        reader.ResetState();
        Assert.Equal(0, reader.FrameCount);
        Assert.Null(reader.CurrentConfig);
    }

    // ----- async surface -----

    [Fact]
    public async Task ReadNextFrameAsync_EmptyStream_ReturnsNull()
    {
        using var ms = new MemoryStream();
        using var reader = NewReader(ms);
        Assert.Null(await reader.ReadNextFrameAsync());
    }

    [Fact]
    public async Task ReadNextFrameAsync_SingleFrame_MatchesSync()
    {
        byte[] frame = BuildAdtsMonoSceFrame();

        using var msSync = new MemoryStream(frame);
        using var sync = NewReader(msSync);
        var syncBlock = sync.ReadNextFrame();
        Assert.NotNull(syncBlock);

        using var msAsync = new MemoryStream(frame);
        using var async = NewReader(msAsync);
        var asyncBlock = await async.ReadNextFrameAsync();
        Assert.NotNull(asyncBlock);

        Assert.Equal(sync.FrameCount, async.FrameCount);
        Assert.Equal(sync.CurrentSampleRate, async.CurrentSampleRate);
        Assert.Equal(sync.CurrentChannelCount, async.CurrentChannelCount);
    }

    [Fact]
    public async Task ReadNextFrameAsync_MultiBlockFrame_FansOutLikeSync()
    {
        byte[] frame = BuildAdtsMonoMultiSceFrame(blockCount: 3);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var blocks = new List<AacDecodedRawDataBlock>();
        AacDecodedRawDataBlock? b;
        while ((b = await reader.ReadNextFrameAsync()) is not null)
        {
            blocks.Add(b);
        }
        Assert.Equal(3, blocks.Count);
    }

    [Fact]
    public async Task ReadFramesAsync_TwoFrames_YieldsBoth()
    {
        byte[] a = BuildAdtsMonoSceFrame();
        byte[] payload = new byte[a.Length * 2];
        Buffer.BlockCopy(a, 0, payload, 0, a.Length);
        Buffer.BlockCopy(a, 0, payload, a.Length, a.Length);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        int count = 0;
        await foreach (var _ in reader.ReadFramesAsync())
        {
            count++;
        }
        Assert.Equal(2, count);
    }

    [Fact]
    public async Task ReadNextFrameAsync_Cancelled_Throws()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(
            async () => await reader.ReadNextFrameAsync(cts.Token));
    }

    [Fact]
    public async Task ReadNextFrameAsync_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = NewReader(ms);
        reader.Dispose();
        await Assert.ThrowsAsync<ObjectDisposedException>(
            async () => await reader.ReadNextFrameAsync());
    }

    // ----- seek surface -----

    [Fact]
    public void SeekToFrame_NonSeekableStream_Throws()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        using var inner = new MemoryStream(frame);
        using var ns = new NonSeekableReadStream(inner);
        using var reader = NewReader(ns);

        var entry = new AacAdtsFrameIndexEntry
        {
            ByteOffset = 0,
            FrameLength = frame.Length,
            BlockCount = 1,
            SampleOffset = 0,
            SampleRate = 48000,
            ChannelConfiguration = 1,
        };
        Assert.False(reader.CanSeek);
        Assert.Throws<NotSupportedException>(() => reader.SeekToFrame(entry));
    }

    [Fact]
    public void SeekToFrame_Entry_RepositionsToOffset()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        byte[] payload = new byte[frame.Length * 3];
        Buffer.BlockCopy(frame, 0, payload, 0, frame.Length);
        Buffer.BlockCopy(frame, 0, payload, frame.Length, frame.Length);
        Buffer.BlockCopy(frame, 0, payload, frame.Length * 2, frame.Length);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        // Read through all 3 to exercise the queue + buffer state.
        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.Null(reader.ReadNextFrame());

        // Seek back to the second frame and read again.
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

        Assert.Equal(0, reader.FrameCount);
        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.Null(reader.ReadNextFrame());
    }

    [Fact]
    public void SeekToFrame_IndexLookup_HitsBracketingEntry()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
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

        // Sample target inside the second frame's range.
        var hit = reader.SeekToFrame(index, sampleTarget: 1500);
        Assert.NotNull(hit);
        Assert.Equal(frame.Length, (int)hit!.ByteOffset);
        Assert.Equal(frame.Length, (int)ms.Position);
    }

    [Fact]
    public void SeekToFrame_IndexLookup_BeforeFirstEntry_ReturnsNullAndLeavesStreamAlone()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 100, FrameLength = frame.Length, BlockCount = 1, SampleOffset = 10_000, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        var hit = reader.SeekToFrame(index, sampleTarget: 0);
        Assert.Null(hit);
        Assert.Equal(0, (int)ms.Position);
    }

    [Fact]
    public void SeekToFrame_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = NewReader(ms);
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
        var tracking = new TrackingStream(inner);
        var reader = new AacAdtsStreamReader(tracking, GetSf(), new AacHuffmanCodebook?[16]);
        await reader.DisposeAsync();
        Assert.True(tracking.AsyncDisposed);
        await Assert.ThrowsAsync<ObjectDisposedException>(async () => await reader.ReadNextFrameAsync());
    }

    [Fact]
    public async Task DisposeAsync_LeaveOpen_DoesNotDisposeStream()
    {
        var inner = new MemoryStream();
        var tracking = new TrackingStream(inner);
        var reader = new AacAdtsStreamReader(tracking, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        await reader.DisposeAsync();
        Assert.False(tracking.AsyncDisposed);
        Assert.False(tracking.SyncDisposed);
    }

    [Fact]
    public async Task DisposeAsync_Idempotent_SecondCallIsNoop()
    {
        var inner = new MemoryStream();
        var tracking = new TrackingStream(inner);
        var reader = new AacAdtsStreamReader(tracking, GetSf(), new AacHuffmanCodebook?[16]);
        await reader.DisposeAsync();
        await reader.DisposeAsync(); // must not double-dispose
        Assert.Equal(1, tracking.AsyncDisposeCount);
    }

    [Fact]
    public async Task DisposeAsync_AfterSyncDispose_IsNoop()
    {
        var inner = new MemoryStream();
        var tracking = new TrackingStream(inner);
        var reader = new AacAdtsStreamReader(tracking, GetSf(), new AacHuffmanCodebook?[16]);
        reader.Dispose();
        await reader.DisposeAsync(); // already disposed sync, async must not run
        Assert.Equal(0, tracking.AsyncDisposeCount);
        Assert.Equal(1, tracking.SyncDisposeCount);
    }

    // ----- lost-sync recovery -----

    [Fact]
    public void ReadNextFrame_RecoverFromLostSync_LeadingGarbage_RecoversAtFrame()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        byte[] payload = new byte[16 + frame.Length];
        for (int i = 0; i < 16; i++) payload[i] = (byte)(i + 1);
        Buffer.BlockCopy(frame, 0, payload, 16, frame.Length);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);
        reader.RecoverFromLostSync = true;

        var block = reader.ReadNextFrame();
        Assert.NotNull(block);
        Assert.Null(reader.ReadNextFrame());
    }

    [Fact]
    public void ReadNextFrame_RecoverFromLostSync_GarbageBetweenFrames_RecoversAtSecondFrame()
    {
        byte[] a = BuildAdtsMonoSceFrame();
        byte[] b = BuildAdtsMonoSceFrame();
        byte[] payload = new byte[a.Length + 8 + b.Length];
        Buffer.BlockCopy(a, 0, payload, 0, a.Length);
        for (int i = 0; i < 8; i++) payload[a.Length + i] = (byte)(0x10 + i);
        Buffer.BlockCopy(b, 0, payload, a.Length + 8, b.Length);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);
        reader.RecoverFromLostSync = true;

        Assert.NotNull(reader.ReadNextFrame());
        Assert.NotNull(reader.ReadNextFrame());
        Assert.Null(reader.ReadNextFrame());
    }

    [Fact]
    public void ReadNextFrame_RecoverFromLostSync_OnlyGarbage_ReturnsNull()
    {
        byte[] payload = new byte[64];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i + 1);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);
        reader.RecoverFromLostSync = true;

        Assert.Null(reader.ReadNextFrame());
    }

    [Fact]
    public void ReadNextFrame_RecoverFromLostSync_False_StillThrowsOnLostSync()
    {
        byte[] payload = new byte[64];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i + 1);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        Assert.False(reader.RecoverFromLostSync);
        Assert.Throws<InvalidDataException>(() => reader.ReadNextFrame());
    }

    [Fact]
    public async Task ReadNextFrameAsync_RecoverFromLostSync_LeadingGarbage_RecoversAtFrame()
    {
        byte[] frame = BuildAdtsMonoSceFrame();
        byte[] payload = new byte[16 + frame.Length];
        for (int i = 0; i < 16; i++) payload[i] = (byte)(i + 1);
        Buffer.BlockCopy(frame, 0, payload, 16, frame.Length);

        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);
        reader.RecoverFromLostSync = true;

        var block = await reader.ReadNextFrameAsync();
        Assert.NotNull(block);
        Assert.Null(await reader.ReadNextFrameAsync());
    }

    // ----- TryReadNextFrame -----

    [Fact]
    public void TryReadNextFrame_ValidFrame_ReturnsTrueWithFrame()
    {
        byte[] payload = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        bool ok = reader.TryReadNextFrame(out var frame);
        Assert.True(ok);
        Assert.NotNull(frame);
    }

    [Fact]
    public void TryReadNextFrame_CleanEof_ReturnsTrueWithNullFrame()
    {
        using var ms = new MemoryStream(Array.Empty<byte>());
        using var reader = NewReader(ms);

        bool ok = reader.TryReadNextFrame(out var frame);
        Assert.True(ok);
        Assert.Null(frame);
    }

    [Fact]
    public void TryReadNextFrame_Garbage_ReturnsFalseWithNull()
    {
        byte[] payload = new byte[64];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i + 1);
        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        bool ok = reader.TryReadNextFrame(out var frame);
        Assert.False(ok);
        Assert.Null(frame);
    }

    [Fact]
    public async Task TryReadNextFrameAsync_ValidFrame_ReturnsTrueWithFrame()
    {
        byte[] payload = BuildAdtsMonoSceFrame();
        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        var (ok, frame) = await reader.TryReadNextFrameAsync();
        Assert.True(ok);
        Assert.NotNull(frame);
    }

    [Fact]
    public async Task TryReadNextFrameAsync_Garbage_ReturnsFalseWithNull()
    {
        byte[] payload = new byte[64];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i + 1);
        using var ms = new MemoryStream(payload);
        using var reader = NewReader(ms);

        var (ok, frame) = await reader.TryReadNextFrameAsync();
        Assert.False(ok);
        Assert.Null(frame);
    }

    // ----- helpers -----

    private static AacHuffmanCodebook GetSf() =>
        AacRawDataBlockTests.GetSharedSyntheticSfCodebook();

    private static AacAdtsStreamReader NewReader(Stream s) =>
        new(s, GetSf(), new AacHuffmanCodebook?[16]);

    private static byte[] BuildAdtsMonoSceFrame()
    {
        byte[] payload = BuildEmptySceRdb();
        return WrapAdtsHeader(payload, rdbInFrame: 0);
    }

    internal static byte[] BuildAdtsMonoSceFrameShared() => BuildAdtsMonoSceFrame();

    internal static byte[] BuildAdtsMonoMultiSceFrameShared(int blockCount) =>
        BuildAdtsMonoMultiSceFrame(blockCount);

    internal static byte[] BuildAdtsMonoShortSceFrameShared() => BuildAdtsMonoShortSceFrame();

    internal static byte[] BuildAdtsMonoMultiShortSceFrameShared(int blockCount) =>
        BuildAdtsMonoMultiShortSceFrame(blockCount);

    private static byte[] BuildAdtsMonoMultiSceFrame(int blockCount)
    {
        // Each empty SCE block's BitWriter byte-aligns on ToArray;
        // concatenating N gives the byte-aligned multi-block payload
        // shape required by ADTS with number_of_raw_data_blocks > 0.
        byte[][] blocks = new byte[blockCount][];
        int payloadLen = 0;
        for (int i = 0; i < blockCount; i++)
        {
            blocks[i] = BuildEmptySceRdb(tag: i);
            payloadLen += blocks[i].Length;
        }
        byte[] payload = new byte[payloadLen];
        int o = 0;
        foreach (var b in blocks)
        {
            Buffer.BlockCopy(b, 0, payload, o, b.Length);
            o += b.Length;
        }
        return WrapAdtsHeader(payload, rdbInFrame: blockCount - 1);
    }

    private static byte[] BuildEmptySceRdb(int tag = 0, int maxSfb = 10)
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        AacRawDataBlockTests.WriteEmptySceBodyShared(w, tag, maxSfb);
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }

    private static byte[] BuildEmptyShortSceRdb(int tag = 0, int maxSfb = 4, byte grouping = 0x7F)
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        AacRawDataBlockTests.WriteEmptyShortSceBodyShared(w, tag, maxSfb, grouping);
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }

    private static byte[] BuildAdtsMonoShortSceFrame()
    {
        byte[] payload = BuildEmptyShortSceRdb();
        return WrapAdtsHeader(payload, rdbInFrame: 0);
    }

    private static byte[] BuildAdtsMonoMultiShortSceFrame(int blockCount)
    {
        byte[][] blocks = new byte[blockCount][];
        int payloadLen = 0;
        for (int i = 0; i < blockCount; i++)
        {
            blocks[i] = BuildEmptyShortSceRdb(tag: i);
            payloadLen += blocks[i].Length;
        }
        byte[] payload = new byte[payloadLen];
        int o = 0;
        foreach (var b in blocks)
        {
            Buffer.BlockCopy(b, 0, payload, o, b.Length);
            o += b.Length;
        }
        return WrapAdtsHeader(payload, rdbInFrame: blockCount - 1);
    }

    private static byte[] WrapAdtsHeader(byte[] payload, int rdbInFrame)
    {
        int headerSize = 7;
        int frameLength = headerSize + payload.Length;

        byte[] h = new byte[headerSize];
        h[0] = 0xFF;
        h[1] = (byte)(0xF0 | 0x01); // MPEG-4, layer 0, protection_absent=1
        // profile=1 (AAC-LC), sfIndex=3 (48k), channelConfig=1
        h[2] = (byte)((1 << 6) | (3 << 2) | ((1 >> 2) & 0x01));
        h[3] = (byte)(((1 & 0x03) << 6) | ((frameLength >> 11) & 0x03));
        h[4] = (byte)((frameLength >> 3) & 0xFF);
        h[5] = (byte)(((frameLength & 0x07) << 5) | 0x1F);
        h[6] = (byte)(0xFC | (rdbInFrame & 0x03));

        byte[] frame = new byte[frameLength];
        Buffer.BlockCopy(h, 0, frame, 0, headerSize);
        Buffer.BlockCopy(payload, 0, frame, headerSize, payload.Length);
        return frame;
    }

    private static byte[] BuildId3v2Tag(int payloadSize)
    {
        byte[] tag = new byte[10 + payloadSize];
        tag[0] = (byte)'I';
        tag[1] = (byte)'D';
        tag[2] = (byte)'3';
        tag[3] = 0x03; // version 2.3
        tag[4] = 0x00;
        tag[5] = 0x00; // flags
        // 28-bit synchsafe size of payloadSize
        tag[6] = (byte)((payloadSize >> 21) & 0x7F);
        tag[7] = (byte)((payloadSize >> 14) & 0x7F);
        tag[8] = (byte)((payloadSize >> 7) & 0x7F);
        tag[9] = (byte)(payloadSize & 0x7F);
        return tag;
    }

    private static byte[] Concat(params byte[][] parts)
    {
        int total = 0;
        foreach (var p in parts) total += p.Length;
        byte[] result = new byte[total];
        int o = 0;
        foreach (var p in parts)
        {
            Buffer.BlockCopy(p, 0, result, o, p.Length);
            o += p.Length;
        }
        return result;
    }

    private sealed class WriteOnlyStream : Stream
    {
        public override bool CanRead => false;
        public override bool CanSeek => false;
        public override bool CanWrite => true;
        public override long Length => 0;
        public override long Position { get => 0; set { } }
        public override void Flush() { }
        public override int Read(byte[] buffer, int offset, int count) => throw new NotSupportedException();
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) { }
        public override void Write(byte[] buffer, int offset, int count) { }
    }

    private sealed class NonSeekableReadStream : Stream
    {
        private readonly Stream _inner;
        public NonSeekableReadStream(Stream inner) { _inner = inner; }
        public override bool CanRead => true;
        public override bool CanSeek => false;
        public override bool CanWrite => false;
        public override long Length => throw new NotSupportedException();
        public override long Position { get => throw new NotSupportedException(); set => throw new NotSupportedException(); }
        public override void Flush() { }
        public override int Read(byte[] buffer, int offset, int count) => _inner.Read(buffer, offset, count);
        public override Task<int> ReadAsync(byte[] buffer, int offset, int count, CancellationToken cancellationToken)
            => _inner.ReadAsync(buffer, offset, count, cancellationToken);
        public override ValueTask<int> ReadAsync(Memory<byte> buffer, CancellationToken cancellationToken = default)
            => _inner.ReadAsync(buffer, cancellationToken);
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException();
        public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();
    }

    private sealed class ChunkingStream : Stream
    {
        private readonly byte[] _data;
        private readonly int _maxBytesPerRead;
        private int _pos;

        public ChunkingStream(byte[] data, int maxBytesPerRead)
        {
            _data = data;
            _maxBytesPerRead = maxBytesPerRead;
        }

        public override bool CanRead => true;
        public override bool CanSeek => false;
        public override bool CanWrite => false;
        public override long Length => _data.Length;
        public override long Position { get => _pos; set => _pos = (int)value; }
        public override void Flush() { }
        public override int Read(byte[] buffer, int offset, int count)
        {
            int avail = _data.Length - _pos;
            if (avail <= 0) return 0;
            int n = Math.Min(Math.Min(count, _maxBytesPerRead), avail);
            Buffer.BlockCopy(_data, _pos, buffer, offset, n);
            _pos += n;
            return n;
        }
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) { }
        public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();
    }

    private sealed class TrackingStream : Stream
    {
        private readonly Stream _inner;
        public bool SyncDisposed { get; private set; }
        public bool AsyncDisposed { get; private set; }
        public int SyncDisposeCount { get; private set; }
        public int AsyncDisposeCount { get; private set; }
        public TrackingStream(Stream inner) { _inner = inner; }
        public override bool CanRead => _inner.CanRead;
        public override bool CanSeek => _inner.CanSeek;
        public override bool CanWrite => _inner.CanWrite;
        public override long Length => _inner.Length;
        public override long Position { get => _inner.Position; set => _inner.Position = value; }
        public override void Flush() => _inner.Flush();
        public override int Read(byte[] buffer, int offset, int count) => _inner.Read(buffer, offset, count);
        public override long Seek(long offset, SeekOrigin origin) => _inner.Seek(offset, origin);
        public override void SetLength(long value) => _inner.SetLength(value);
        public override void Write(byte[] buffer, int offset, int count) => _inner.Write(buffer, offset, count);
        protected override void Dispose(bool disposing)
        {
            if (disposing)
            {
                SyncDisposed = true;
                SyncDisposeCount++;
                _inner.Dispose();
            }
            base.Dispose(disposing);
        }
        [System.Diagnostics.CodeAnalysis.SuppressMessage("Usage", "CA2215:Dispose methods should call base class dispose",
            Justification = "Test helper deliberately bypasses base.DisposeAsync to track sync vs async dispose paths.")]
        public override ValueTask DisposeAsync()
        {
            AsyncDisposed = true;
            AsyncDisposeCount++;
            return _inner.DisposeAsync();
        }
    }
}
