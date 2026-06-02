using Mediar.Codecs.Opus.Decoder;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Coverage for <see cref="OpusFramePacker"/> — Opus frame packing per
/// RFC 6716 §3.2. Tests every framing code (0/1/2/3 incl. CBR + VBR),
/// optional padding, the 1-byte vs 2-byte frame-length encoding, and the
/// six structural rejection rules (R1..R7).
/// </summary>
public sealed class OpusFramePackerTests
{
    private static byte TocByte(int config, bool stereo, int code)
        => (byte)((config << 3) | (stereo ? 0x4 : 0) | (code & 0x3));

    private static byte[] BuildCode0(int config, byte[] payload)
    {
        byte[] pkt = new byte[1 + payload.Length];
        pkt[0] = TocByte(config, stereo: false, code: 0);
        payload.CopyTo(pkt, 1);
        return pkt;
    }

    [Fact]
    public void Code0_Single_Frame_Spans_Rest_Of_Packet()
    {
        byte[] payload = new byte[100];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i * 3);
        byte[] pkt = BuildCode0(config: 5, payload);
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Single(frames);
        Assert.Equal(1, frames[0].Offset);
        Assert.Equal(100, frames[0].Length);
    }

    [Fact]
    public void Code0_Allows_Empty_Frame_Payload()
    {
        // A code-0 packet with only the TOC byte is a valid 1-byte packet
        // containing one zero-length frame (DTX / silence indicator).
        byte[] pkt = { TocByte(0, false, 0) };
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Single(frames);
        Assert.Equal(0, frames[0].Length);
    }

    [Fact]
    public void Code1_Two_Equal_Frames()
    {
        byte[] payload = new byte[80];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i + 1);
        byte[] pkt = new byte[1 + payload.Length];
        pkt[0] = TocByte(1, false, 1);
        payload.CopyTo(pkt, 1);
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Equal(2, frames.Count);
        Assert.Equal(1, frames[0].Offset);
        Assert.Equal(40, frames[0].Length);
        Assert.Equal(41, frames[1].Offset);
        Assert.Equal(40, frames[1].Length);
    }

    [Fact]
    public void Code1_Rejects_Odd_Payload_R3()
    {
        byte[] pkt = new byte[1 + 41]; // odd
        pkt[0] = TocByte(1, false, 1);
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code2_Two_Independent_Frames_Short_Length()
    {
        // length-1 fits in 1 byte (<252). Use len1=10, then len2 = remainder.
        byte[] pkt = new byte[1 + 1 + 10 + 25];
        pkt[0] = TocByte(2, false, 2);
        pkt[1] = 10;
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Equal(2, frames.Count);
        Assert.Equal(2, frames[0].Offset);
        Assert.Equal(10, frames[0].Length);
        Assert.Equal(12, frames[1].Offset);
        Assert.Equal(25, frames[1].Length);
    }

    [Fact]
    public void Code2_TwoByte_Length_Encoding_Round_Trips()
    {
        // Length 500 needs 2-byte encoding.
        Span<byte> lenEnc = stackalloc byte[2];
        int written = OpusFramePacker.WriteFrameLength(500, lenEnc);
        Assert.Equal(2, written);

        byte[] pkt = new byte[1 + 2 + 500 + 100];
        pkt[0] = TocByte(2, false, 2);
        pkt[1] = lenEnc[0];
        pkt[2] = lenEnc[1];
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Equal(2, frames.Count);
        Assert.Equal(3, frames[0].Offset);
        Assert.Equal(500, frames[0].Length);
        Assert.Equal(503, frames[1].Offset);
        Assert.Equal(100, frames[1].Length);
    }

    [Fact]
    public void Code2_Rejects_FirstFrame_Past_End_R4()
    {
        // first frame says length 200 but packet has only 50 payload bytes.
        byte[] pkt = new byte[1 + 1 + 50];
        pkt[0] = TocByte(2, false, 2);
        pkt[1] = 200;
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code2_Rejects_Missing_LengthByte_R4()
    {
        byte[] pkt = { TocByte(2, false, 2) };
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code3_Cbr_Equal_Frames_NoPadding()
    {
        // 4 frames × 10 ms each = 40 ms (≤120 ms), each frame 20 bytes.
        int m = 4;
        int per = 20;
        byte fc = (byte)m; // vbr=0, padding=0
        byte[] pkt = new byte[1 + 1 + m * per];
        pkt[0] = TocByte(config: 1 /* 20 ms? no, config 1 = 20ms SILK NB */, false, 3);
        // Use config 0 (10 ms) so 4 frames = 40 ms total
        pkt[0] = TocByte(config: 0, false, 3);
        pkt[1] = fc;
        for (int i = 0; i < m * per; i++) pkt[2 + i] = (byte)i;
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Equal(m, frames.Count);
        for (int i = 0; i < m; i++)
        {
            Assert.Equal(2 + i * per, frames[i].Offset);
            Assert.Equal(per, frames[i].Length);
        }
    }

    [Fact]
    public void Code3_Vbr_Frames_With_Encoded_Lengths()
    {
        // 3 frames at 20 ms (config 1) = 60 ms.
        int m = 3;
        int[] lens = { 7, 15, 23 };
        int payload = lens[0] + lens[1] + lens[2];
        byte fc = (byte)(0x80 | m); // vbr=1, padding=0
        byte[] pkt = new byte[1 + 1 + (m - 1) + payload];
        pkt[0] = TocByte(config: 1, false, 3);
        pkt[1] = fc;
        pkt[2] = (byte)lens[0];
        pkt[3] = (byte)lens[1];
        // frame data
        int cursor = 4;
        for (int i = 0; i < m; i++)
        {
            for (int b = 0; b < lens[i]; b++) pkt[cursor + b] = (byte)((i + 1) * (b + 1));
            cursor += lens[i];
        }
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Equal(m, frames.Count);
        Assert.Equal(lens[0], frames[0].Length);
        Assert.Equal(lens[1], frames[1].Length);
        Assert.Equal(lens[2], frames[2].Length);
    }

    [Fact]
    public void Code3_Padding_Small_Single_Byte()
    {
        // 2 frames × 10 ms CBR with 5 bytes of padding.
        int m = 2;
        int per = 10;
        byte fc = (byte)(0x40 | m); // padding=1, vbr=0
        int padding = 5;
        byte[] pkt = new byte[1 + 1 + 1 + m * per + padding];
        pkt[0] = TocByte(config: 0, false, 3);
        pkt[1] = fc;
        pkt[2] = (byte)padding;
        // Padding bytes are at the END of the packet; their content is
        // ignored but we fill them with a marker to make sure they aren't
        // mistaken for frame data.
        for (int i = 0; i < padding; i++) pkt[pkt.Length - 1 - i] = 0xCC;
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Equal(m, frames.Count);
        Assert.Equal(per, frames[0].Length);
        Assert.Equal(per, frames[1].Length);
        // Frames sit BEFORE the padding region.
        Assert.True(frames[1].Offset + frames[1].Length + padding == pkt.Length);
    }

    [Fact]
    public void Code3_Padding_Large_Multi_Byte()
    {
        // Padding length 510 = 254 + 254 + 2, encoded as [255, 255, 2].
        int padding = 510;
        int m = 2;
        int per = 12;
        byte fc = (byte)(0x40 | m);
        byte[] pkt = new byte[1 + 1 + 3 + m * per + padding];
        pkt[0] = TocByte(config: 0, false, 3);
        pkt[1] = fc;
        pkt[2] = 255;
        pkt[3] = 255;
        pkt[4] = 2;
        var frames = OpusFramePacker.Unpack(pkt);
        Assert.Equal(2, frames.Count);
        Assert.Equal(per, frames[0].Length);
        Assert.Equal(per, frames[1].Length);
    }

    [Fact]
    public void Code3_Rejects_M_Zero_R5()
    {
        byte[] pkt = { TocByte(0, false, 3), 0 };
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code3_Rejects_M_TooLarge_R5()
    {
        byte[] pkt = { TocByte(0, false, 3), 49 };
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code3_Rejects_DurationOver120ms_R5()
    {
        // config 11 = SILK WB 60 ms. M=3 → 180 ms > 120 ms.
        byte[] pkt = new byte[2 + 30];
        pkt[0] = TocByte(11, false, 3);
        pkt[1] = 3; // vbr=0, padding=0, m=3
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code3_Cbr_Rejects_Payload_Not_Divisible_R6()
    {
        // m=3, payload=10 → 10 % 3 != 0.
        byte[] pkt = new byte[2 + 10];
        pkt[0] = TocByte(0, false, 3);
        pkt[1] = 3;
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code3_Vbr_Rejects_Lengths_Exceeding_Packet_R6()
    {
        // m=3 vbr, declared lengths sum > available payload.
        byte[] pkt = new byte[1 + 1 + 2 + 5];
        pkt[0] = TocByte(0, false, 3);
        pkt[1] = (byte)(0x80 | 3);
        pkt[2] = 100;
        pkt[3] = 100;
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Code3_Rejects_Missing_Padding_Length_Byte_R6()
    {
        byte[] pkt = new byte[2];
        pkt[0] = TocByte(0, false, 3);
        pkt[1] = (byte)(0x40 | 2); // padding=1 but no padding-length byte follows
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(pkt));
    }

    [Fact]
    public void Rejects_Empty_Packet_R1()
    {
        Assert.Throws<InvalidDataException>(() => OpusFramePacker.Unpack(ReadOnlySpan<byte>.Empty));
    }

    [Theory]
    [InlineData(0)]    // single-byte 0
    [InlineData(1)]
    [InlineData(251)]  // single-byte boundary
    [InlineData(252)]  // smallest two-byte
    [InlineData(255)]
    [InlineData(1024)]
    [InlineData(1275)] // maximum
    public void FrameLength_Codec_Round_Trips(int len)
    {
        Span<byte> buf = stackalloc byte[2];
        int written = OpusFramePacker.WriteFrameLength(len, buf);
        Assert.InRange(written, 1, 2);
        int cursor = 0;
        int read = OpusFramePacker.ReadFrameLength(buf[..written], ref cursor);
        Assert.Equal(len, read);
        Assert.Equal(written, cursor);
    }

    [Fact]
    public void FrameLength_Encoder_Rejects_Too_Large()
    {
        var buf = new byte[2];
        Assert.Throws<ArgumentOutOfRangeException>(() => OpusFramePacker.WriteFrameLength(1276, buf));
    }
}
