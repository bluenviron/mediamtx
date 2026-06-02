using Mediar.Codecs.Opus.Decoder;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Coverage for <see cref="OpusRangeDecoder"/> — the entropy decoder shared
/// by CELT and SILK (RFC 6716 §4.1).
/// </summary>
/// <remarks>
/// <para>
/// Writing a hand-rolled range <i>encoder</i> for round-trip tests is risky
/// (the carry chain is notoriously fiddly). The tests here therefore focus
/// on properties that are individually verifiable without an encoder:
/// initialisation invariants, the spec-defined <c>Ilog</c> helper, raw-bit
/// reads from the END of the buffer (which don't interact with the range
/// state), error flagging on under-flow, and <c>Tell</c>/<c>TellFrac</c>
/// monotonicity. The range decoder gets heavy round-trip exercise once
/// Phase 2 (CELT) starts feeding real packets through it.
/// </para>
/// </remarks>
public sealed class OpusRangeDecoderTests
{
    [Fact]
    public void Init_With_Empty_Buffer_Flags_Error()
    {
        var dec = new OpusRangeDecoder(ReadOnlySpan<byte>.Empty);
        // The constructor reads the first byte during init, which fails
        // for an empty buffer and trips HasError.
        Assert.True(dec.HasError);
    }

    [Fact]
    public void Init_Normalises_Range_Above_CodeBot()
    {
        // After construction the renormalise loop must have lifted rng
        // above CodeBot (1<<23). Even an all-zero buffer must satisfy this.
        byte[] buf = new byte[8];
        var dec = new OpusRangeDecoder(buf);
        Assert.True(dec.Range > OpusRangeDecoder.CodeBot,
            $"Range {dec.Range} must exceed CodeBot {OpusRangeDecoder.CodeBot} after init.");
    }

    [Fact]
    public void Init_Has_Expected_Spec_Constants()
    {
        // RFC 6716 §4.1: code bits = 32, symbol bits = 8, code extra = 7.
        Assert.Equal(32, OpusRangeDecoder.CodeBits);
        Assert.Equal(8, OpusRangeDecoder.SymbolBits);
        Assert.Equal(7, OpusRangeDecoder.CodeExtra);
        Assert.Equal(1u << 31, OpusRangeDecoder.CodeTop);
        Assert.Equal(1u << 23, OpusRangeDecoder.CodeBot);
        Assert.Equal(8, OpusRangeDecoder.UintBits);
    }

    [Theory]
    [InlineData(0u, 0)]
    [InlineData(1u, 1)]
    [InlineData(2u, 2)]
    [InlineData(3u, 2)]
    [InlineData(4u, 3)]
    [InlineData(7u, 3)]
    [InlineData(8u, 4)]
    [InlineData(127u, 7)]
    [InlineData(128u, 8)]
    [InlineData(0x4000_0000u, 31)]
    [InlineData(0x8000_0000u, 32)]
    [InlineData(0xFFFF_FFFFu, 32)]
    public void Ilog_Matches_Spec(uint value, int expected)
        => Assert.Equal(expected, OpusRangeDecoder.Ilog(value));

    [Fact]
    public void DecodeBits_Reads_Bits_From_End_LSB_First()
    {
        // The raw-bit reader takes bytes from the back of the packet and
        // unpacks LSB-first within each byte. With a 1-byte init payload
        // and a recognisable last byte (0xA5 = 1010_0101), the first
        // 4-bit read must yield 0x5, the next 4-bit read 0xA.
        byte[] buf = new byte[8];
        buf[0] = 0x80;
        buf[buf.Length - 1] = 0xA5;
        var dec = new OpusRangeDecoder(buf);
        Assert.Equal(0x5u, dec.DecodeBits(4));
        Assert.Equal(0xAu, dec.DecodeBits(4));
    }

    [Fact]
    public void DecodeBits_Spans_Multiple_End_Bytes()
    {
        // Read a 16-bit value that straddles two back-bytes.
        // Last byte = 0xCD (low 8 bits), second-to-last = 0xAB (high 8 bits)
        // → expected value 0xABCD.
        byte[] buf = new byte[16];
        buf[0] = 0x40;
        buf[buf.Length - 1] = 0xCD;
        buf[buf.Length - 2] = 0xAB;
        var dec = new OpusRangeDecoder(buf);
        Assert.Equal(0xABCDu, dec.DecodeBits(16));
    }

    [Fact]
    public void DecodeBits_Tracks_BackByteOffset()
    {
        byte[] buf = new byte[16];
        buf[0] = 0x40;
        var dec = new OpusRangeDecoder(buf);
        Assert.Equal(0, dec.BackByteOffset);
        // Reading 9 bits consumes 2 bytes from the end.
        dec.DecodeBits(9);
        Assert.True(dec.BackByteOffset >= 2,
            "Reading 9 bits must have pulled at least 2 bytes from the back.");
    }

    [Fact]
    public void DecodeBits_Throws_On_OutOfRange_Width()
    {
        byte[] buf = new byte[4]; buf[0] = 0x80;
        var dec = new OpusRangeDecoder(buf);
        // Need to materialise the struct to call DecodeBits with bad input.
        // Range-checked at parameter time so InvalidOperationException not needed.
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            var local = new OpusRangeDecoder(new byte[] { 0x80 });
            local.DecodeBits(26);
        });
    }

    [Fact]
    public void DecodeBin_Throws_On_OutOfRange_Width()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            var local = new OpusRangeDecoder(new byte[] { 0x80 });
            local.DecodeBin(32);
        });
    }

    [Fact]
    public void Decode_Rejects_Zero_Ft()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            var local = new OpusRangeDecoder(new byte[] { 0x80 });
            local.Decode(0);
        });
    }

    [Fact]
    public void Update_Rejects_Invalid_Symbol_Range()
    {
        Assert.Throws<ArgumentException>(() =>
        {
            var local = new OpusRangeDecoder(new byte[] { 0x80 });
            local.Decode(16);
            local.Update(fl: 5, fh: 3, ft: 16);
        });
    }

    [Fact]
    public void DecodeUint_Rejects_Ft_LessThan_2()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            var local = new OpusRangeDecoder(new byte[] { 0x80 });
            local.DecodeUint(1);
        });
    }

    [Fact]
    public void DecodeIcdf_Rejects_Empty_Table()
    {
        Assert.Throws<ArgumentException>(() =>
        {
            var local = new OpusRangeDecoder(new byte[] { 0x80 });
            local.DecodeIcdf(ReadOnlySpan<byte>.Empty, ftb: 4);
        });
    }

    [Fact]
    public void DecodeIcdf_Rejects_NonTerminated_Table()
    {
        Assert.Throws<ArgumentException>(() =>
        {
            var local = new OpusRangeDecoder(new byte[] { 0x80 });
            byte[] bad = { 8, 4, 2 }; // must end in 0
            local.DecodeIcdf(bad, ftb: 4);
        });
    }

    [Fact]
    public void DecodeBitLogP_Returns_Bit_Value()
    {
        // Without exercising specific outcomes (which depend on the buffer
        // contents), verify the result is always 0 or 1 for a range of
        // logp values.
        byte[] buf = new byte[16];
        for (int i = 0; i < buf.Length; i++) buf[i] = (byte)(i * 37);
        for (int logp = 1; logp <= 12; logp++)
        {
            var local = new OpusRangeDecoder(buf);
            int bit = local.DecodeBitLogP(logp);
            Assert.InRange(bit, 0, 1);
        }
    }

    [Fact]
    public void Tell_Does_Not_Decrease_Across_Operations()
    {
        byte[] buf = new byte[64];
        for (int i = 0; i < buf.Length; i++) buf[i] = (byte)(i * 17 + 3);
        var dec = new OpusRangeDecoder(buf);
        int prevTell = dec.Tell();
        uint prevFrac = dec.TellFrac();
        for (int i = 0; i < 20; i++)
        {
            uint fs = dec.Decode(16);
            dec.Update(fs, fs + 1, 16);
            int t = dec.Tell();
            uint f = dec.TellFrac();
            Assert.True(t >= prevTell, $"Tell regressed: {prevTell} → {t}");
            Assert.True(f >= prevFrac, $"TellFrac regressed: {prevFrac} → {f}");
            prevTell = t;
            prevFrac = f;
        }
    }

    [Fact]
    public void Reading_Past_End_Flags_HasError()
    {
        // Drain DecodeBits from a tiny buffer until we run off the end.
        byte[] buf = { 0x80, 0x55 };
        var dec = new OpusRangeDecoder(buf);
        // Each DecodeBits(8) pulls a byte from the END until exhausted.
        // After ~3-4 calls the back pointer overruns the front pointer.
        for (int i = 0; i < 5; i++) dec.DecodeBits(8);
        Assert.True(dec.HasError);
    }

    [Fact]
    public void Decoder_Exposes_Front_And_Back_Offsets()
    {
        byte[] buf = new byte[16];
        var dec = new OpusRangeDecoder(buf);
        Assert.True(dec.FrontByteOffset >= 1, "Init must consume at least one front byte.");
        Assert.Equal(0, dec.BackByteOffset);
        Assert.Equal(16, dec.BufferLength);
    }
}
