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

    // ----- helpers -----

    private static AacHuffmanCodebook GetSf() =>
        AacRawDataBlockTests.GetSharedSyntheticSfCodebook();

    private static AacAdtsStreamReader NewReader(Stream s) =>
        new(s, GetSf(), new AacHuffmanCodebook?[16]);

    private static byte[] BuildAdtsMonoSceFrame()
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        AacRawDataBlockTests.WriteEmptySceBodyShared(w, tag: 0, maxSfb: 10);
        w.Write((uint)AacSyntacticElementType.End, 3);
        byte[] payload = w.ToArray();

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
        h[6] = 0xFC;

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
