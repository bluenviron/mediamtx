using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacAdtsFrameIndexerTests
{
    [Fact]
    public void BuildIndex_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AacAdtsFrameIndexer.BuildIndex(null!));
    }

    [Fact]
    public void BuildIndex_EmptyStream_ReturnsEmpty()
    {
        using var ms = new MemoryStream();
        var index = AacAdtsFrameIndexer.BuildIndex(ms);
        Assert.Empty(index);
    }

    [Fact]
    public void BuildIndex_SingleFrame_RecordsOneEntryAtOffsetZero()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        var index = AacAdtsFrameIndexer.BuildIndex(ms);

        Assert.Single(index);
        var e = index[0];
        Assert.Equal(0L, e.ByteOffset);
        Assert.Equal(frame.Length, e.FrameLength);
        Assert.Equal(1, e.BlockCount);
        Assert.Equal(0L, e.SampleOffset);
        Assert.Equal(48000, e.SampleRate);
        Assert.Equal(1, e.ChannelConfiguration);
    }

    [Fact]
    public void BuildIndex_TwoFrames_RecordsCumulativeOffsets()
    {
        byte[] a = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] b = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, payload, 0, a.Length);
        Buffer.BlockCopy(b, 0, payload, a.Length, b.Length);
        using var ms = new MemoryStream(payload);

        var index = AacAdtsFrameIndexer.BuildIndex(ms);
        Assert.Equal(2, index.Count);
        Assert.Equal(0L, index[0].ByteOffset);
        Assert.Equal(0L, index[0].SampleOffset);
        Assert.Equal(a.Length, index[1].ByteOffset);
        // 1024 samples per block; single-block frames.
        Assert.Equal(1024L, index[1].SampleOffset);
    }

    [Fact]
    public void BuildIndex_MultiBlockFrame_AccountsForAllBlocksInSampleOffset()
    {
        byte[] multi = AacAdtsStreamReaderTests.BuildAdtsMonoMultiSceFrameShared(blockCount: 3);
        byte[] single = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[multi.Length + single.Length];
        Buffer.BlockCopy(multi, 0, payload, 0, multi.Length);
        Buffer.BlockCopy(single, 0, payload, multi.Length, single.Length);
        using var ms = new MemoryStream(payload);

        var index = AacAdtsFrameIndexer.BuildIndex(ms);
        Assert.Equal(2, index.Count);
        Assert.Equal(3, index[0].BlockCount);
        Assert.Equal(0L, index[0].SampleOffset);
        // 3 blocks * 1024 samples = 3072 samples preceding the second frame.
        Assert.Equal(3072L, index[1].SampleOffset);
        Assert.Equal(1, index[1].BlockCount);
    }

    [Fact]
    public void BuildIndex_LostSync_Throws()
    {
        byte[] payload = new byte[] { 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07 };
        using var ms = new MemoryStream(payload);
        Assert.Throws<InvalidDataException>(() => AacAdtsFrameIndexer.BuildIndex(ms));
    }

    [Fact]
    public void BuildIndex_TruncatedFrame_Throws()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] truncated = frame.AsSpan(0, frame.Length - 4).ToArray();
        using var ms = new MemoryStream(truncated);
        Assert.Throws<InvalidDataException>(() => AacAdtsFrameIndexer.BuildIndex(ms));
    }

    [Fact]
    public void BuildIndex_NonReadableStream_Throws()
    {
        using var sink = new WriteOnlyStream();
        Assert.Throws<ArgumentException>(() => AacAdtsFrameIndexer.BuildIndex(sink));
    }

    [Fact]
    public void BuildIndex_TooSmallBuffer_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacAdtsFrameIndexer.BuildIndex(ms, initialBufferSize: 4));
    }

    [Fact]
    public async Task BuildIndexAsync_TwoFrames_MatchesSyncIndex()
    {
        byte[] a = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] b = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, payload, 0, a.Length);
        Buffer.BlockCopy(b, 0, payload, a.Length, b.Length);

        using var msSync = new MemoryStream(payload);
        var sync = AacAdtsFrameIndexer.BuildIndex(msSync);

        using var msAsync = new MemoryStream(payload);
        var async = await AacAdtsFrameIndexer.BuildIndexAsync(msAsync);

        Assert.Equal(sync.Count, async.Count);
        for (int i = 0; i < sync.Count; i++)
        {
            Assert.Equal(sync[i].ByteOffset, async[i].ByteOffset);
            Assert.Equal(sync[i].FrameLength, async[i].FrameLength);
            Assert.Equal(sync[i].SampleOffset, async[i].SampleOffset);
            Assert.Equal(sync[i].BlockCount, async[i].BlockCount);
        }
    }

    [Fact]
    public async Task BuildIndexAsync_Cancelled_Throws()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
            await AacAdtsFrameIndexer.BuildIndexAsync(ms, cancellationToken: cts.Token));
    }

    [Fact]
    public async Task BuildIndexAsync_NullStream_Throws()
    {
        await Assert.ThrowsAsync<ArgumentNullException>(async () =>
            await AacAdtsFrameIndexer.BuildIndexAsync(null!));
    }

    [Fact]
    public void FindFrameAtSample_NullIndex_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacAdtsFrameIndexer.FindFrameAtSample(null!, 0));
    }

    [Fact]
    public void FindFrameAtSample_EmptyIndex_ReturnsMinusOne()
    {
        var empty = Array.Empty<AacAdtsFrameIndexEntry>();
        Assert.Equal(-1, AacAdtsFrameIndexer.FindFrameAtSample(empty, 0));
        Assert.Equal(-1, AacAdtsFrameIndexer.FindFrameAtSample(empty, 1_000_000));
    }

    [Fact]
    public void FindFrameAtSample_NegativeTarget_Throws()
    {
        var entry = new AacAdtsFrameIndexEntry
        {
            ByteOffset = 0,
            FrameLength = 64,
            BlockCount = 1,
            SampleOffset = 0,
            SampleRate = 48000,
            ChannelConfiguration = 1,
        };
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacAdtsFrameIndexer.FindFrameAtSample(new[] { entry }, -1));
    }

    [Fact]
    public void FindFrameAtSample_TargetBeforeFirst_ReturnsMinusOne()
    {
        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0, FrameLength = 64, BlockCount = 1, SampleOffset = 5_000, SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 64, FrameLength = 64, BlockCount = 1, SampleOffset = 6_024, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        Assert.Equal(-1, AacAdtsFrameIndexer.FindFrameAtSample(index, 1_000));
    }

    [Fact]
    public void FindFrameAtSample_TargetMatchesExactBoundary_ReturnsThatEntry()
    {
        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0,   FrameLength = 64, BlockCount = 1, SampleOffset = 0,    SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 64,  FrameLength = 64, BlockCount = 1, SampleOffset = 1024, SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 128, FrameLength = 64, BlockCount = 1, SampleOffset = 2048, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        Assert.Equal(0, AacAdtsFrameIndexer.FindFrameAtSample(index, 0));
        Assert.Equal(1, AacAdtsFrameIndexer.FindFrameAtSample(index, 1024));
        Assert.Equal(2, AacAdtsFrameIndexer.FindFrameAtSample(index, 2048));
    }

    [Fact]
    public void FindFrameAtSample_TargetInsideRange_ReturnsBracketingEntry()
    {
        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0,   FrameLength = 64, BlockCount = 1, SampleOffset = 0,    SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 64,  FrameLength = 64, BlockCount = 1, SampleOffset = 1024, SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 128, FrameLength = 64, BlockCount = 1, SampleOffset = 2048, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        Assert.Equal(0, AacAdtsFrameIndexer.FindFrameAtSample(index, 500));
        Assert.Equal(1, AacAdtsFrameIndexer.FindFrameAtSample(index, 1500));
        Assert.Equal(2, AacAdtsFrameIndexer.FindFrameAtSample(index, 2500));
    }

    [Fact]
    public void FindFrameAtSample_TargetPastLastEntry_ClampsToLast()
    {
        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0,  FrameLength = 64, BlockCount = 1, SampleOffset = 0,    SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 64, FrameLength = 64, BlockCount = 1, SampleOffset = 1024, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        Assert.Equal(1, AacAdtsFrameIndexer.FindFrameAtSample(index, 1_000_000));
    }

    [Fact]
    public void FindFrameAtTime_NegativeTime_Throws()
    {
        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0, FrameLength = 64, BlockCount = 1, SampleOffset = 0, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacAdtsFrameIndexer.FindFrameAtTime(index, TimeSpan.FromSeconds(-0.1), 48000));
    }

    [Fact]
    public void FindFrameAtTime_NonPositiveSampleRate_Throws()
    {
        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0, FrameLength = 64, BlockCount = 1, SampleOffset = 0, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacAdtsFrameIndexer.FindFrameAtTime(index, TimeSpan.Zero, 0));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacAdtsFrameIndexer.FindFrameAtTime(index, TimeSpan.Zero, -48000));
    }

    [Fact]
    public void FindFrameAtTime_ConvertsTimeToSampleAndDispatches()
    {
        // 48 kHz: every 1024 samples = 1024/48000 s = ~21.333 ms.
        var index = new[]
        {
            new AacAdtsFrameIndexEntry { ByteOffset = 0,   FrameLength = 64, BlockCount = 1, SampleOffset = 0,    SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 64,  FrameLength = 64, BlockCount = 1, SampleOffset = 1024, SampleRate = 48000, ChannelConfiguration = 1 },
            new AacAdtsFrameIndexEntry { ByteOffset = 128, FrameLength = 64, BlockCount = 1, SampleOffset = 2048, SampleRate = 48000, ChannelConfiguration = 1 },
        };
        Assert.Equal(0, AacAdtsFrameIndexer.FindFrameAtTime(index, TimeSpan.FromMilliseconds(10), 48000));
        Assert.Equal(1, AacAdtsFrameIndexer.FindFrameAtTime(index, TimeSpan.FromMilliseconds(30), 48000));
        Assert.Equal(2, AacAdtsFrameIndexer.FindFrameAtTime(index, TimeSpan.FromMilliseconds(50), 48000));
    }

    // ----- ID3v2 skip -----

    [Fact]
    public void BuildIndex_SkipId3v2_StripsTagBeforeIndexing()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] tag = BuildId3v2Tag(payloadSize: 73);
        byte[] payload = new byte[tag.Length + frame.Length];
        Buffer.BlockCopy(tag, 0, payload, 0, tag.Length);
        Buffer.BlockCopy(frame, 0, payload, tag.Length, frame.Length);

        using var ms = new MemoryStream(payload);
        var index = AacAdtsFrameIndexer.BuildIndex(ms, skipId3v2: true);
        Assert.Single(index);
        // ByteOffset is the absolute offset in the original stream
        // (including the skipped tag), so callers can seek directly.
        Assert.Equal((long)tag.Length, index[0].ByteOffset);
        Assert.Equal(frame.Length, index[0].FrameLength);
    }

    [Fact]
    public void BuildIndex_SkipId3v2_NoTag_FallsBackToParsing()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        // A seekable MemoryStream can be rewound, so this works even
        // when there is no leading ID3 tag.
        using var ms = new MemoryStream(frame);
        var index = AacAdtsFrameIndexer.BuildIndex(ms, skipId3v2: true);
        Assert.Single(index);
        Assert.Equal(0L, index[0].ByteOffset);
    }

    [Fact]
    public async Task BuildIndexAsync_SkipId3v2_StripsTagBeforeIndexing()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] tag = BuildId3v2Tag(payloadSize: 73);
        byte[] payload = new byte[tag.Length + frame.Length];
        Buffer.BlockCopy(tag, 0, payload, 0, tag.Length);
        Buffer.BlockCopy(frame, 0, payload, tag.Length, frame.Length);

        using var ms = new MemoryStream(payload);
        var index = await AacAdtsFrameIndexer.BuildIndexAsync(ms, skipId3v2: true);
        Assert.Single(index);
        Assert.Equal((long)tag.Length, index[0].ByteOffset);
    }

    [Fact]
    public void BuildIndex_SkipId3v2_NonSeekableNoTag_Throws()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var inner = new MemoryStream(frame);
        using var ns = new NonSeekableReadStream(inner);
        Assert.Throws<InvalidDataException>(() =>
            AacAdtsFrameIndexer.BuildIndex(ns, skipId3v2: true));
    }

    // ----- lost-sync recovery -----

    [Fact]
    public void BuildIndex_RecoverFromLostSync_LeadingGarbageThenFrame_RecoversAtFrame()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[16 + frame.Length];
        for (int i = 0; i < 16; i++) payload[i] = (byte)(i + 1); // non-syncword garbage
        Buffer.BlockCopy(frame, 0, payload, 16, frame.Length);

        using var ms = new MemoryStream(payload);
        var index = AacAdtsFrameIndexer.BuildIndex(ms, skipId3v2: false, recoverFromLostSync: true);

        Assert.Single(index);
        Assert.Equal(16L, index[0].ByteOffset);
        Assert.Equal(0L, index[0].SampleOffset);
    }

    [Fact]
    public void BuildIndex_RecoverFromLostSync_GarbageBetweenFrames_SkipsGarbage()
    {
        byte[] a = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] b = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        // a + 8 garbage bytes + b
        byte[] payload = new byte[a.Length + 8 + b.Length];
        Buffer.BlockCopy(a, 0, payload, 0, a.Length);
        for (int i = 0; i < 8; i++) payload[a.Length + i] = (byte)(0x10 + i);
        Buffer.BlockCopy(b, 0, payload, a.Length + 8, b.Length);

        using var ms = new MemoryStream(payload);
        var index = AacAdtsFrameIndexer.BuildIndex(ms, skipId3v2: false, recoverFromLostSync: true);

        Assert.Equal(2, index.Count);
        Assert.Equal(0L, index[0].ByteOffset);
        Assert.Equal((long)(a.Length + 8), index[1].ByteOffset);
        // SampleOffset counts only successfully-indexed frames; the garbage
        // contributes zero samples so frame 2 starts at frame-1's sample-count.
        Assert.Equal(0L, index[0].SampleOffset);
        Assert.Equal(1024L, index[1].SampleOffset);
    }

    [Fact]
    public void BuildIndex_RecoverFromLostSync_TrailingGarbage_DropsItSilently()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[frame.Length + 3]; // 3 trailing bytes is < header size
        Buffer.BlockCopy(frame, 0, payload, 0, frame.Length);
        payload[frame.Length] = 0x55;
        payload[frame.Length + 1] = 0xAA;
        payload[frame.Length + 2] = 0x33;

        using var ms = new MemoryStream(payload);
        var index = AacAdtsFrameIndexer.BuildIndex(ms, skipId3v2: false, recoverFromLostSync: true);

        Assert.Single(index);
    }

    [Fact]
    public void BuildIndex_RecoverFromLostSync_OnlyGarbage_ReturnsEmpty()
    {
        byte[] payload = new byte[64];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i + 1);

        using var ms = new MemoryStream(payload);
        var index = AacAdtsFrameIndexer.BuildIndex(ms, skipId3v2: false, recoverFromLostSync: true);

        Assert.Empty(index);
    }

    [Fact]
    public void BuildIndex_RecoverFromLostSync_False_StillThrowsOnLostSync()
    {
        byte[] payload = new byte[64];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i + 1);

        using var ms = new MemoryStream(payload);
        Assert.Throws<InvalidDataException>(() =>
            AacAdtsFrameIndexer.BuildIndex(ms, skipId3v2: false, recoverFromLostSync: false));
    }

    [Fact]
    public async Task BuildIndexAsync_RecoverFromLostSync_LeadingGarbage_RecoversAtFrame()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        byte[] payload = new byte[16 + frame.Length];
        for (int i = 0; i < 16; i++) payload[i] = (byte)(i + 1);
        Buffer.BlockCopy(frame, 0, payload, 16, frame.Length);

        using var ms = new MemoryStream(payload);
        var index = await AacAdtsFrameIndexer.BuildIndexAsync(
            ms, skipId3v2: false, recoverFromLostSync: true);

        Assert.Single(index);
        Assert.Equal(16L, index[0].ByteOffset);
    }

    private static byte[] BuildId3v2Tag(int payloadSize)
    {
        // 10-byte header + payloadSize bytes of filler. Synchsafe size
        // encodes payloadSize across bytes 6..9.
        byte[] tag = new byte[10 + payloadSize];
        tag[0] = (byte)'I';
        tag[1] = (byte)'D';
        tag[2] = (byte)'3';
        tag[3] = 0x04; // major version
        tag[4] = 0x00; // revision
        tag[5] = 0x00; // flags
        tag[6] = (byte)((payloadSize >> 21) & 0x7F);
        tag[7] = (byte)((payloadSize >> 14) & 0x7F);
        tag[8] = (byte)((payloadSize >> 7) & 0x7F);
        tag[9] = (byte)(payloadSize & 0x7F);
        // Tag body is just zeros; the indexer never inspects it.
        return tag;
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
}
