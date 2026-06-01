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
}
