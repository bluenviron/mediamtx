using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacAdtsFrameDecoderTests
{
    // ----- constructor null guards -----

    [Fact]
    public void Ctor_NullScaleFactorCodebook_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacAdtsFrameDecoder(
            scaleFactorCodebook: null!,
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullSpectralCodebooks_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacAdtsFrameDecoder(
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: null!));
    }

    [Fact]
    public void Ctor_BeforeFirstDecode_StateIsEmpty()
    {
        var dec = new AacAdtsFrameDecoder(GetSf(), new AacHuffmanCodebook?[16]);
        Assert.Null(dec.CurrentConfig);
        Assert.Null(dec.CurrentSpeakers);
        Assert.Equal(0, dec.CurrentSampleRate);
        Assert.Equal(0, dec.CurrentChannelCount);
        Assert.Equal(0, dec.FrameCount);
    }

    // ----- TryParseFrameLength -----

    [Fact]
    public void TryParseFrameLength_ValidHeader_ReturnsTrue()
    {
        var header = BuildAdtsHeader(profile: 1, sfIndex: 3, channelConfig: 1,
            frameLength: 200, protectionAbsent: true, rdbInFrame: 0);
        Assert.True(AacAdtsFrameDecoder.TryParseFrameLength(header, out int len));
        Assert.Equal(200, len);
    }

    [Fact]
    public void TryParseFrameLength_BufferTooShort_ReturnsFalse()
    {
        Assert.False(AacAdtsFrameDecoder.TryParseFrameLength([0xFF, 0xF1, 0x40], out _));
    }

    [Fact]
    public void TryParseFrameLength_BadSync_ReturnsFalse()
    {
        var header = BuildAdtsHeader(profile: 1, sfIndex: 3, channelConfig: 1,
            frameLength: 200, protectionAbsent: true, rdbInFrame: 0);
        header[0] = 0x00; // sync byte 0 corrupted
        Assert.False(AacAdtsFrameDecoder.TryParseFrameLength(header, out _));
    }

    [Fact]
    public void TryParseFrameLength_BadLayer_ReturnsFalse()
    {
        var header = BuildAdtsHeader(profile: 1, sfIndex: 3, channelConfig: 1,
            frameLength: 200, protectionAbsent: true, rdbInFrame: 0);
        header[1] |= 0x02; // set layer bits non-zero
        Assert.False(AacAdtsFrameDecoder.TryParseFrameLength(header, out _));
    }

    [Fact]
    public void TryParseFrameLength_FrameLengthBelowHeader_ReturnsFalse()
    {
        var header = BuildAdtsHeader(profile: 1, sfIndex: 3, channelConfig: 1,
            frameLength: 6, protectionAbsent: true, rdbInFrame: 0);
        Assert.False(AacAdtsFrameDecoder.TryParseFrameLength(header, out _));
    }

    // ----- DecodeFrame happy paths -----

    [Fact]
    public void DecodeFrame_SingleMonoSce_DecodesAndPopulatesState()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);

        var block = dec.DecodeFrame(frame);

        Assert.Single(block.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, block.Channels[0].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, block.Channels[0].Samples.Length);

        Assert.NotNull(dec.CurrentConfig);
        Assert.Equal(2, dec.CurrentConfig!.AudioObjectType);
        Assert.Equal(48_000, dec.CurrentConfig.SamplingFrequency);
        Assert.Equal(1, dec.CurrentConfig.ChannelConfiguration);
        Assert.Equal(1, dec.CurrentChannelCount);
        Assert.Equal(48_000, dec.CurrentSampleRate);
        Assert.NotNull(dec.CurrentSpeakers);
        Assert.Single(dec.CurrentSpeakers!);
        Assert.Equal(AacSpeaker.FrontCentre, dec.CurrentSpeakers![0]);
        Assert.Equal(1, dec.FrameCount);
    }

    [Fact]
    public void DecodeFrame_SingleMonoShortSce_DecodesAndPopulatesState()
    {
        // ADTS-wrapped frame whose raw_data_block carries an EightShort SCE.
        var dec = NewDecoder();
        var frame = BuildAdtsMonoShortSceFrame(sfIndex: 3, channelConfig: 1);

        var block = dec.DecodeFrame(frame);

        Assert.Single(block.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, block.Channels[0].Speaker);
        // The synthesis filterbank still produces 1024 PCM samples
        // for EightShort frames (8 × 128 IMDCT output overlap-added).
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, block.Channels[0].Samples.Length);
        Assert.Equal(1, dec.FrameCount);
        Assert.Equal(48_000, dec.CurrentSampleRate);
    }

    [Fact]
    public void DecodeFrame_TwoConsecutiveShortFrames_FrameCountIncrements()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoShortSceFrame(sfIndex: 3, channelConfig: 1);

        _ = dec.DecodeFrame(frame);
        _ = dec.DecodeFrame(frame);

        Assert.Equal(2, dec.FrameCount);
    }

    [Fact]
    public void DecodeFrame_MixedLongAndShortFrames_ReusesInnerDecoder()
    {
        // Same sample rate and channel config → the inner AacFrameDecoder
        // should be reused across the long → short transition without a
        // rebuild, since the ADTS header is identical.
        var dec = NewDecoder();
        var longFrame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        var shortFrame = BuildAdtsMonoShortSceFrame(sfIndex: 3, channelConfig: 1);

        _ = dec.DecodeFrame(longFrame);
        var firstConfig = dec.CurrentConfig;
        _ = dec.DecodeFrame(shortFrame);

        Assert.Same(firstConfig, dec.CurrentConfig);
        Assert.Equal(2, dec.FrameCount);
    }

    [Fact]
    public void DecodeFrame_TwoFrames_FrameCountIncrements()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);

        _ = dec.DecodeFrame(frame);
        _ = dec.DecodeFrame(frame);

        Assert.Equal(2, dec.FrameCount);
    }

    [Fact]
    public void DecodeFrame_SameConfig_ReusesInnerDecoder()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);

        _ = dec.DecodeFrame(frame);
        var firstConfigInstance = dec.CurrentConfig;
        _ = dec.DecodeFrame(frame);

        // Same config object reused across calls when header is unchanged.
        Assert.Same(firstConfigInstance, dec.CurrentConfig);
    }

    [Fact]
    public void DecodeFrame_DifferentSampleRate_RebuildsInnerDecoder()
    {
        var dec = NewDecoder();
        var frameA = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1); // 48000 Hz
        var frameB = BuildAdtsMonoSceFrame(sfIndex: 4, channelConfig: 1); // 44100 Hz

        _ = dec.DecodeFrame(frameA);
        var firstConfig = dec.CurrentConfig;
        Assert.Equal(48_000, firstConfig!.SamplingFrequency);

        _ = dec.DecodeFrame(frameB);
        var secondConfig = dec.CurrentConfig;
        Assert.NotSame(firstConfig, secondConfig);
        Assert.Equal(44_100, secondConfig!.SamplingFrequency);
        Assert.Equal(44_100, dec.CurrentSampleRate);
        Assert.Equal(2, dec.FrameCount);
    }

    [Fact]
    public void DecodeFrame_AdtsWithCrc_DecodesAfterSkippingCrcBytes()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1, withCrc: true);

        var block = dec.DecodeFrame(frame);

        Assert.Single(block.Channels);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, block.Channels[0].Samples.Length);
    }

    // ----- DecodeFrame failures -----

    [Fact]
    public void DecodeFrame_NoSyncword_ThrowsArgument()
    {
        var dec = NewDecoder();
        var garbage = new byte[16];
        Assert.Throws<ArgumentException>(() => dec.DecodeFrame(garbage));
    }

    [Fact]
    public void DecodeFrame_BufferShorterThanDeclaredFrameLength_ThrowsArgument()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        // Pass only the first 12 bytes — header is valid but frame is truncated.
        var ex = Assert.Throws<ArgumentException>(() => dec.DecodeFrame(frame.AsSpan(0, 12).ToArray()));
        Assert.Contains("shorter than the declared", ex.Message);
    }

    [Fact]
    public void DecodeFrame_MultiRawDataBlock_ThrowsNotSupported()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1, rdbInFrame: 1);
        var ex = Assert.Throws<NotSupportedException>(() => dec.DecodeFrame(frame));
        Assert.Contains("DecodeBlocks", ex.Message);
    }

    // ----- multi-block DecodeBlocks coverage -----

    [Fact]
    public void DecodeBlocks_NullSink_Throws()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        Assert.Throws<ArgumentNullException>(() =>
            dec.DecodeBlocks(frame, null!));
    }

    [Fact]
    public void DecodeBlocks_SingleBlockFrame_InvokesSinkOnceReturnsOne()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);

        int callCount = 0;
        AacDecodedRawDataBlock? captured = null;
        int returned = dec.DecodeBlocks(frame, b => { callCount++; captured = b; });

        Assert.Equal(1, returned);
        Assert.Equal(1, callCount);
        Assert.NotNull(captured);
        Assert.Single(captured!.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, captured.Channels[0].Speaker);
    }

    [Fact]
    public void DecodeBlocks_TwoBlockFrame_InvokesSinkTwiceReturnsTwo()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMultiSceFrame(sfIndex: 3, channelConfig: 1, blockCount: 2);

        var blocks = new List<AacDecodedRawDataBlock>();
        int returned = dec.DecodeBlocks(frame, b => blocks.Add(b));

        Assert.Equal(2, returned);
        Assert.Equal(2, blocks.Count);
        Assert.All(blocks, b =>
        {
            Assert.Single(b.Channels);
            Assert.Equal(AacSpeaker.FrontCentre, b.Channels[0].Speaker);
            Assert.Equal(AacSynthesisFilterbank.LongFrameLength, b.Channels[0].Samples.Length);
        });
    }

    [Fact]
    public void DecodeBlocks_ThreeBlockFrame_InvokesSinkThriceReturnsThree()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMultiSceFrame(sfIndex: 3, channelConfig: 1, blockCount: 3);

        int callCount = 0;
        int returned = dec.DecodeBlocks(frame, _ => callCount++);

        Assert.Equal(3, returned);
        Assert.Equal(3, callCount);
    }

    [Fact]
    public void DecodeBlocks_MultiBlockFrame_AdvancesFrameCountPerBlock()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMultiSceFrame(sfIndex: 3, channelConfig: 1, blockCount: 3);

        _ = dec.DecodeBlocks(frame, _ => { });

        // FrameCount counts decoded raw_data_blocks, not ADTS frame
        // envelopes — so a 3-block frame contributes 3 to the count.
        Assert.Equal(3, dec.FrameCount);
    }

    [Fact]
    public void DecodeBlocks_TruncatedBuffer_ThrowsArgument()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        var ex = Assert.Throws<ArgumentException>(() =>
            dec.DecodeBlocks(frame.AsSpan(0, frame.Length - 2).ToArray(), _ => { }));
        Assert.Contains("shorter than the declared", ex.Message);
    }

    [Fact]
    public void DecodeBlocks_BadSync_ThrowsArgument()
    {
        var dec = NewDecoder();
        byte[] garbage = new byte[20];
        Assert.Throws<ArgumentException>(() =>
            dec.DecodeBlocks(garbage, _ => { }));
    }

    [Fact]
    public void DecodeBlocks_ChannelConfigZero_ThrowsInvalidData()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 0);
        Assert.Throws<InvalidDataException>(() =>
            dec.DecodeBlocks(frame, _ => { }));
    }

    [Fact]
    public void DecodeBlocks_ProtectedMultiBlock_DecodesAllBlocks()
    {
        var dec = NewDecoder();
        // Protected (protection_absent=0) + 2 raw_data_blocks: each
        // block trails a zeroed 16-bit crc_check that the decoder
        // skips without verifying.
        var frame = BuildProtectedMultiSceFrame(
            sfIndex: 3, channelConfig: 1, blockCount: 2);

        int count = 0;
        int decoded = dec.DecodeBlocks(frame, _ => count++);
        Assert.Equal(2, decoded);
        Assert.Equal(2, count);
        Assert.Equal(2, dec.FrameCount);
    }

    [Fact]
    public void DecodeBlocks_ProtectedMultiBlock_MissingCrcTrailer_Throws()
    {
        var dec = NewDecoder();
        var frame = BuildProtectedMultiSceFrame(
            sfIndex: 3, channelConfig: 1, blockCount: 2);
        // Strip the trailing 2 CRC bytes from the last block. The
        // frame_length field is rebuilt by the helper, so we have to
        // patch both the buffer length AND the length field.
        int newLen = frame.Length - 2;
        var truncated = frame.AsSpan(0, newLen).ToArray();
        // frame_length: bits [3:30..29] [4:28..21] [5:20..18]
        truncated[3] = (byte)((truncated[3] & 0xFC) | ((newLen >> 11) & 0x03));
        truncated[4] = (byte)((newLen >> 3) & 0xFF);
        truncated[5] = (byte)((truncated[5] & 0x1F) | ((newLen & 0x07) << 5));

        Assert.Throws<InvalidDataException>(() =>
            dec.DecodeBlocks(truncated, _ => { }));
    }

    [Fact]
    public void DecodeFrames_MultiBlockFrame_InvokesSinkPerBlock()
    {
        var dec = NewDecoder();
        var frameA = BuildAdtsMultiSceFrame(sfIndex: 3, channelConfig: 1, blockCount: 2);
        var frameB = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        var buffer = Concat(frameA, frameB);

        int callCount = 0;
        int consumed = dec.DecodeFrames(buffer, _ => callCount++);

        // The walker fans out each ADTS frame into its contained
        // raw_data_blocks: 2 + 1 = 3 sink invocations.
        Assert.Equal(3, callCount);
        Assert.Equal(buffer.Length, consumed);
    }

    [Fact]
    public void DecodeFrame_ChannelConfigZero_ThrowsInvalidData()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 0);
        Assert.Throws<InvalidDataException>(() => dec.DecodeFrame(frame));
    }

    // ----- ResetState -----

    [Fact]
    public void ResetState_DropsConfigAndResetsCount()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        _ = dec.DecodeFrame(frame);
        Assert.Equal(1, dec.FrameCount);
        Assert.NotNull(dec.CurrentConfig);

        dec.ResetState();

        Assert.Equal(0, dec.FrameCount);
        Assert.Null(dec.CurrentConfig);
        Assert.Null(dec.CurrentSpeakers);
        Assert.Equal(0, dec.CurrentSampleRate);
    }

    [Fact]
    public void ResetState_AllowsSubsequentDecode()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);

        _ = dec.DecodeFrame(frame);
        dec.ResetState();
        var block = dec.DecodeFrame(frame);

        Assert.Single(block.Channels);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, block.Channels[0].Samples.Length);
        Assert.Equal(1, dec.FrameCount);
    }

    // ----- DecodeFrames (multi-frame walker) -----

    [Fact]
    public void DecodeFrames_NullSink_Throws()
    {
        var dec = NewDecoder();
        Assert.Throws<ArgumentNullException>(() =>
            dec.DecodeFrames(ReadOnlySpan<byte>.Empty, null!));
    }

    [Fact]
    public void DecodeFrames_EmptyInput_ConsumesZero()
    {
        var dec = NewDecoder();
        int consumed = dec.DecodeFrames(ReadOnlySpan<byte>.Empty, _ => { });
        Assert.Equal(0, consumed);
        Assert.Equal(0, dec.FrameCount);
    }

    [Fact]
    public void DecodeFrames_ThreeBackToBack_InvokesSinkThreeTimes()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        byte[] buffer = Concat(frame, frame, frame);

        int callbacks = 0;
        int consumed = dec.DecodeFrames(buffer, _ => callbacks++);

        Assert.Equal(3, callbacks);
        Assert.Equal(buffer.Length, consumed);
        Assert.Equal(3, dec.FrameCount);
    }

    [Fact]
    public void DecodeFrames_TruncatedTail_ReportsConsumedBytesAndKeepsRemainder()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        // Two whole frames + first 12 bytes of a third (truncated).
        byte[] buffer = Concat(frame, frame, frame.AsSpan(0, 12).ToArray());

        int callbacks = 0;
        int consumed = dec.DecodeFrames(buffer, _ => callbacks++);

        Assert.Equal(2, callbacks);
        Assert.Equal(frame.Length * 2, consumed);
        Assert.Equal(12, buffer.Length - consumed);
    }

    [Fact]
    public void DecodeFrames_PartialHeader_DefersWithoutThrowing()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        // First whole frame followed by just 3 header bytes of the next.
        byte[] buffer = Concat(frame, new byte[] { 0xFF, 0xF1, 0x40 });

        int callbacks = 0;
        int consumed = dec.DecodeFrames(buffer, _ => callbacks++);

        Assert.Equal(1, callbacks);
        Assert.Equal(frame.Length, consumed);
    }

    [Fact]
    public void DecodeFrames_LostSync_ThrowsInvalidData()
    {
        var dec = NewDecoder();
        var frame = BuildAdtsMonoSceFrame(sfIndex: 3, channelConfig: 1);
        // First frame + 8 bytes of garbage (>= 6 → cannot defer as partial header).
        byte[] buffer = Concat(frame, new byte[] { 0, 1, 2, 3, 4, 5, 6, 7 });

        var ex = Assert.Throws<InvalidDataException>(() =>
            dec.DecodeFrames(buffer, _ => { }));
        Assert.Contains("Lost ADTS sync", ex.Message);
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

    // ----- helpers -----

    private static AacHuffmanCodebook GetSf() =>
        AacRawDataBlockTests.GetSharedSyntheticSfCodebook();

    private static AacAdtsFrameDecoder NewDecoder() =>
        new(GetSf(), new AacHuffmanCodebook?[16]);

    private static byte[] BuildEmptySceRawDataBlock(int tag = 0, int maxSfb = 10)
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        AacRawDataBlockTests.WriteEmptySceBodyShared(w, tag, maxSfb);
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }

    private static byte[] BuildEmptyShortSceRawDataBlock(int tag = 0, int maxSfb = 4, byte grouping = 0x7F)
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        AacRawDataBlockTests.WriteEmptyShortSceBodyShared(w, tag, maxSfb, grouping);
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }

    private static byte[] BuildAdtsMonoSceFrame(
        int sfIndex,
        int channelConfig,
        bool withCrc = false,
        int rdbInFrame = 0)
    {
        byte[] payload = BuildEmptySceRawDataBlock();
        int headerSize = withCrc ? 9 : 7;
        int frameLength = headerSize + payload.Length;
        byte[] header = BuildAdtsHeader(
            profile: 1,            // AAC-LC
            sfIndex: sfIndex,
            channelConfig: channelConfig,
            frameLength: frameLength,
            protectionAbsent: !withCrc,
            rdbInFrame: rdbInFrame);

        byte[] frame = new byte[frameLength];
        Buffer.BlockCopy(header, 0, frame, 0, header.Length);
        Buffer.BlockCopy(payload, 0, frame, headerSize, payload.Length);
        return frame;
    }

    private static byte[] BuildAdtsMonoShortSceFrame(
        int sfIndex,
        int channelConfig,
        bool withCrc = false,
        int rdbInFrame = 0)
    {
        byte[] payload = BuildEmptyShortSceRawDataBlock();
        int headerSize = withCrc ? 9 : 7;
        int frameLength = headerSize + payload.Length;
        byte[] header = BuildAdtsHeader(
            profile: 1,
            sfIndex: sfIndex,
            channelConfig: channelConfig,
            frameLength: frameLength,
            protectionAbsent: !withCrc,
            rdbInFrame: rdbInFrame);

        byte[] frame = new byte[frameLength];
        Buffer.BlockCopy(header, 0, frame, 0, header.Length);
        Buffer.BlockCopy(payload, 0, frame, headerSize, payload.Length);
        return frame;
    }

    private static byte[] BuildAdtsMultiSceFrame(
        int sfIndex,
        int channelConfig,
        int blockCount)
    {
        // Each raw_data_block is independently byte-aligned by its
        // BitWriter; concatenating them gives the multi-block payload
        // shape the spec describes (each block ends with
        // byte_alignment(), so the next block starts at a byte
        // boundary in the outer buffer).
        byte[][] blocks = new byte[blockCount][];
        int payloadLen = 0;
        for (int i = 0; i < blockCount; i++)
        {
            blocks[i] = BuildEmptySceRawDataBlock(tag: i, maxSfb: 10);
            payloadLen += blocks[i].Length;
        }

        int headerSize = 7;
        int frameLength = headerSize + payloadLen;
        byte[] header = BuildAdtsHeader(
            profile: 1,
            sfIndex: sfIndex,
            channelConfig: channelConfig,
            frameLength: frameLength,
            protectionAbsent: true,
            rdbInFrame: blockCount - 1);

        byte[] frame = new byte[frameLength];
        Buffer.BlockCopy(header, 0, frame, 0, headerSize);
        int o = headerSize;
        foreach (var b in blocks)
        {
            Buffer.BlockCopy(b, 0, frame, o, b.Length);
            o += b.Length;
        }
        return frame;
    }

    private static byte[] BuildProtectedMultiSceFrame(
        int sfIndex,
        int channelConfig,
        int blockCount)
    {
        // Build N raw_data_blocks (byte-aligned), each followed by a
        // 2-byte zeroed CRC trailer. The header is the protected
        // variant with raw_data_block_position[] pointers also zeroed
        // (the decoder ignores them and walks the cursor instead).
        byte[][] blocks = new byte[blockCount][];
        int payloadLen = 0;
        for (int i = 0; i < blockCount; i++)
        {
            blocks[i] = BuildEmptySceRawDataBlock(tag: i, maxSfb: 10);
            payloadLen += blocks[i].Length + 2; // +2 for per-block crc_check
        }

        int headerSize = 9 + 2 * (blockCount - 1);
        int frameLength = headerSize + payloadLen;
        byte[] header = BuildAdtsHeader(
            profile: 1,
            sfIndex: sfIndex,
            channelConfig: channelConfig,
            frameLength: frameLength,
            protectionAbsent: false,
            rdbInFrame: blockCount - 1);

        byte[] frame = new byte[frameLength];
        Buffer.BlockCopy(header, 0, frame, 0, headerSize);
        int o = headerSize;
        for (int i = 0; i < blockCount; i++)
        {
            Buffer.BlockCopy(blocks[i], 0, frame, o, blocks[i].Length);
            o += blocks[i].Length;
            // Per-block crc_check trailer: 2 bytes, value irrelevant.
            o += 2;
        }
        return frame;
    }

    private static byte[] BuildProtectedMultiBlockHeader(
        int sfIndex,
        int channelConfig,
        int rdbInFrame,
        int payloadSize)
    {
        int headerSize = 9 + 2 * rdbInFrame;
        int frameLength = headerSize + payloadSize;
        byte[] header = BuildAdtsHeader(
            profile: 1,
            sfIndex: sfIndex,
            channelConfig: channelConfig,
            frameLength: frameLength,
            protectionAbsent: false,
            rdbInFrame: rdbInFrame);

        byte[] frame = new byte[frameLength];
        Buffer.BlockCopy(header, 0, frame, 0, headerSize);
        return frame;
    }

    private static byte[] BuildAdtsHeader(
        int profile,
        int sfIndex,
        int channelConfig,
        int frameLength,
        bool protectionAbsent,
        int rdbInFrame)
    {
        int size = protectionAbsent ? 7 : 9 + 2 * rdbInFrame;
        byte[] h = new byte[size];

        // syncword 0xFFF, ID=0 (MPEG-4), layer=0, protection_absent
        h[0] = 0xFF;
        h[1] = (byte)(0xF0 | (protectionAbsent ? 0x01 : 0x00));

        // profile (2), sample_freq_index (4), private_bit (1), ch_cfg high bit (1)
        h[2] = (byte)(((profile & 0x03) << 6) | ((sfIndex & 0x0F) << 2) | ((channelConfig >> 2) & 0x01));

        // ch_cfg low 2 bits, original/copy (1), home (1), copyright_id_bit (1),
        // copyright_id_start (1), frame_length high 2 bits
        h[3] = (byte)(((channelConfig & 0x03) << 6) | ((frameLength >> 11) & 0x03));

        // frame_length middle 8 bits
        h[4] = (byte)((frameLength >> 3) & 0xFF);

        // frame_length low 3 bits, buffer_fullness high 5 bits (0)
        h[5] = (byte)(((frameLength & 0x07) << 5) | 0x1F);

        // buffer_fullness low 6 bits (all 1), number_of_raw_data_blocks_in_frame (2 bits)
        h[6] = (byte)(0xFC | (rdbInFrame & 0x03));

        // For CRC frames, leave the raw_data_block_position[] and
        // crc_check bytes as zero — the decoder does not verify them
        // in this revision.
        return h;
    }
}
