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
