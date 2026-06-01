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
        Assert.Throws<NotSupportedException>(() => dec.DecodeFrame(frame));
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

    private static byte[] BuildAdtsHeader(
        int profile,
        int sfIndex,
        int channelConfig,
        int frameLength,
        bool protectionAbsent,
        int rdbInFrame)
    {
        int size = protectionAbsent ? 7 : 9;
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

        // For CRC frames, leave the two CRC bytes as zero — the
        // decoder does not verify them in this revision.
        return h;
    }
}
