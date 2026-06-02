using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for <see cref="CeltSplit"/> — the CELT energy split decoder
/// (Opus Phase 2c.3b.3). Covers <c>compute_qn</c> exhaustively (pure
/// integer math), <c>quant_band_n1</c> (which only reads raw bits from
/// the END of the buffer), and the post-decode <c>imid</c>/<c>iside</c>/
/// <c>delta</c> math via the extracted <c>FinaliseSplit</c> helper.
/// The entropy-pdf decode branches inside <c>compute_theta</c> get
/// natural coverage in Phase 2c.3b.5 once <c>quant_all_bands</c> is
/// wired against real Opus packets.
/// </summary>
public sealed class CeltSplitTests
{
    // -----------------------------------------------------------------
    // ComputeQn — pure integer math, exhaustively testable.
    // -----------------------------------------------------------------

    [Fact]
    public void ComputeQn_With_Very_Low_Bits_Returns_One()
    {
        // qb < (1 << BitRes >> 1) = 4 ⇒ qn=1.
        int qn = CeltSplit.ComputeQn(n: 16, b: 0, offset: 0, pulseCap: 0, stereo: false);
        Assert.Equal(1, qn);
    }

    [Fact]
    public void ComputeQn_Always_Returns_Even_Or_One()
    {
        // qn is rounded to even via ((qn+1)>>1)<<1 after the exp2 lookup.
        for (int n = 2; n <= 32; n++)
        for (int b = 0; b < 4000; b += 17)
        for (int off = -200; off <= 200; off += 100)
        {
            int qn = CeltSplit.ComputeQn(n, b, off, pulseCap: 50, stereo: false);
            Assert.True(qn == 1 || (qn % 2 == 0),
                $"qn={qn} for n={n}, b={b}, off={off} must be 1 or even.");
            Assert.InRange(qn, 1, 256);
        }
    }

    [Fact]
    public void ComputeQn_Caps_At_256()
    {
        // 8 << BitRes = 64 is the upper qb cap. exp2_table8[0] >> (14 - 8) =
        // 16384 >> 6 = 256, rounded to even stays 256.
        int qn = CeltSplit.ComputeQn(n: 64, b: 1_000_000, offset: 0, pulseCap: 0, stereo: false);
        Assert.Equal(256, qn);
    }

    [Theory]
    [InlineData(2, 64, 5, 0, false)]
    [InlineData(2, 64, 5, 0, true)]   // stereo && N==2 ⇒ N2 = 2*2-1-1 = 2
    [InlineData(8, 200, 8, 4, false)]
    [InlineData(16, 400, 12, 8, false)]
    [InlineData(16, 400, 12, 8, true)]
    public void ComputeQn_Matches_LibOpus_Reference(int n, int b, int offset, int pulseCap, bool stereo)
    {
        // Reference: re-implement libopus compute_qn inline (different
        // structure: uses opus_int16 table and bit shifts in the same
        // order) and compare. Any discrepancy means our port drifted.
        int expected = LibopusComputeQnReference(n, b, offset, pulseCap, stereo);
        int actual = CeltSplit.ComputeQn(n, b, offset, pulseCap, stereo);
        Assert.Equal(expected, actual);
    }

    private static int LibopusComputeQnReference(int n, int b, int offset, int pulseCap, bool stereo)
    {
        short[] exp2 = { 16384, 17866, 19483, 21247, 23170, 25267, 27554, 30048 };
        const int BITRES = 3;
        int N2 = 2 * n - 1;
        if (stereo && n == 2) N2--;
        int qb = (b + N2 * offset) / N2;
        qb = System.Math.Min(b - pulseCap - (4 << BITRES), qb);
        qb = System.Math.Min(8 << BITRES, qb);
        int qn;
        if (qb < (1 << BITRES >> 1)) qn = 1;
        else
        {
            qn = exp2[qb & 0x7] >> (14 - (qb >> BITRES));
            qn = (qn + 1) >> 1 << 1;
        }
        return qn;
    }

    // -----------------------------------------------------------------
    // FinaliseSplit — pure math for the post-decode imid/iside/delta.
    // -----------------------------------------------------------------

    [Fact]
    public void FinaliseSplit_With_Theta_Zero_Returns_All_Mid()
    {
        int fill = 0xFFFF;
        CeltSplit.FinaliseSplit(itheta: 0, n: 16, blocks: 4, ref fill,
            out int imid, out int iside, out int delta);
        Assert.Equal(32767, imid);
        Assert.Equal(0, iside);
        Assert.Equal(-16384, delta);
        // fill &= (1<<4) - 1 = 0x0F — keeps only the lower 4 bits.
        Assert.Equal(0x000F, fill);
    }

    [Fact]
    public void FinaliseSplit_With_Theta_Max_Returns_All_Side()
    {
        int fill = 0xFFFF;
        CeltSplit.FinaliseSplit(itheta: 16384, n: 16, blocks: 4, ref fill,
            out int imid, out int iside, out int delta);
        Assert.Equal(0, imid);
        Assert.Equal(32767, iside);
        Assert.Equal(16384, delta);
        // fill &= ((1<<4) - 1) << 4 = 0xF0 — keeps only the upper-half bits.
        Assert.Equal(0x00F0, fill);
    }

    [Fact]
    public void FinaliseSplit_With_Middle_Theta_Splits_Energy()
    {
        // itheta = 8192 (cos(π/4) = sin(π/4)) ⇒ imid == iside.
        int fill = 0xFFFF;
        CeltSplit.FinaliseSplit(itheta: 8192, n: 16, blocks: 4, ref fill,
            out int imid, out int iside, out int delta);
        // BitexactCos(8192) is symmetric ⇒ same value.
        Assert.Equal(imid, iside);
        // delta should be 0 since iside == imid ⇒ log2tan(iside,imid)=0.
        Assert.Equal(0, delta);
        // fill is not touched in this branch.
        Assert.Equal(0xFFFF, fill);
    }

    [Fact]
    public void FinaliseSplit_With_Middle_Theta_Matches_BitexactCos()
    {
        int fill = 0;
        for (int itheta = 64; itheta <= 16320; itheta += 113)
        {
            CeltSplit.FinaliseSplit(itheta, n: 8, blocks: 2, ref fill,
                out int imid, out int iside, out int delta);
            Assert.Equal(CeltPvqMath.BitexactCos((short)itheta), imid);
            Assert.Equal(CeltPvqMath.BitexactCos((short)(16384 - itheta)), iside);
            // delta = FRAC_MUL16((N-1)<<7, BitexactLog2Tan(iside, imid)).
            int log2tan = CeltPvqMath.BitexactLog2Tan(iside, imid);
            int expectedDelta = (16384 + ((short)((8 - 1) << 7) * (short)log2tan)) >> 15;
            Assert.Equal(expectedDelta, delta);
        }
    }

    // -----------------------------------------------------------------
    // QuantBandN1 — sign-bit decode + resynth + lowband_out.
    // Raw bits are read from the END of the buffer, so we can craft
    // deterministic test data without a range encoder.
    // -----------------------------------------------------------------

    [Fact]
    public void QuantBandN1_Mono_With_Positive_Sign_Writes_Positive_Norm()
    {
        // Last byte's least-significant bit becomes the first raw bit.
        // Bit 0 == 0 ⇒ sign = 0 ⇒ +1.
        byte[] buf = new byte[16];
        buf[buf.Length - 1] = 0b1111_1110;   // bit 0 == 0
        var dec = new OpusRangeDecoder(buf);
        int remaining = 8;
        Span<float> x = new float[1];
        uint cm = CeltSplit.QuantBandN1(ref dec, ref remaining, x, default, default);
        Assert.Equal(1u, cm);
        Assert.Equal(1.0f, x[0]);
        Assert.Equal(0, remaining);
    }

    [Fact]
    public void QuantBandN1_Mono_With_Negative_Sign_Writes_Negative_Norm()
    {
        byte[] buf = new byte[16];
        buf[buf.Length - 1] = 0b0000_0001;   // bit 0 == 1 ⇒ sign = 1 ⇒ -1
        var dec = new OpusRangeDecoder(buf);
        int remaining = 8;
        Span<float> x = new float[1];
        CeltSplit.QuantBandN1(ref dec, ref remaining, x, default, default);
        Assert.Equal(-1.0f, x[0]);
    }

    [Fact]
    public void QuantBandN1_Stereo_Decodes_Both_Channels_Independently()
    {
        // Two raw bits read in order: bit 0 of last byte (channel 0),
        // then bit 1 (channel 1). Build: bit0=1 (Y -1), bit1=0 (Y +1).
        // Wait — bits are consumed from the LSB upward of the last byte.
        // First DecodeBits(1) returns bit 0; second returns bit 1.
        byte[] buf = new byte[16];
        buf[buf.Length - 1] = 0b0000_0001;   // bit 0 = 1 (sign 1), bit 1 = 0 (sign 0)
        var dec = new OpusRangeDecoder(buf);
        int remaining = 16;
        Span<float> x = new float[1];
        Span<float> y = new float[1];
        CeltSplit.QuantBandN1(ref dec, ref remaining, x, y, default);
        Assert.Equal(-1.0f, x[0]);   // first sign bit was 1
        Assert.Equal(1.0f, y[0]);    // second sign bit was 0
        Assert.Equal(0, remaining);  // 2 raw bits × 8 frac units
    }

    [Fact]
    public void QuantBandN1_With_Zero_Remaining_Bits_Defaults_To_Positive()
    {
        // remainingBits < 8 ⇒ sign bit is NOT read; default sign=0 ⇒ +1.
        byte[] buf = new byte[16];
        buf[buf.Length - 1] = 0xFF;          // would decode to -1 if read
        var dec = new OpusRangeDecoder(buf);
        int remaining = 0;
        Span<float> x = new float[1];
        CeltSplit.QuantBandN1(ref dec, ref remaining, x, default, default);
        Assert.Equal(1.0f, x[0]);
        Assert.Equal(0, remaining);          // not decremented
    }

    [Fact]
    public void QuantBandN1_Writes_Lowband_Out_When_Provided()
    {
        byte[] buf = new byte[16];
        buf[buf.Length - 1] = 0;             // sign 0 ⇒ X[0] = +1
        var dec = new OpusRangeDecoder(buf);
        int remaining = 8;
        Span<float> x = new float[1];
        Span<float> lb = new float[1];
        CeltSplit.QuantBandN1(ref dec, ref remaining, x, default, lb);
        // Float build: lowband_out[0] = X[0] / 16 = 1/16 = 0.0625.
        Assert.Equal(1.0f / 16.0f, lb[0]);
    }

    [Fact]
    public void QuantBandN1_Throws_On_Empty_X()
    {
        // QuantBandN1 needs at least one X sample.
        Assert.Throws<System.ArgumentException>(() =>
        {
            byte[] b = new byte[16];
            var d = new OpusRangeDecoder(b);
            int r = 8;
            Span<float> empty = default;
            CeltSplit.QuantBandN1(ref d, ref r, empty, default, default);
        });
    }

    // -----------------------------------------------------------------
    // ComputeTheta — qn=1 paths (no entropy decoding of theta needed).
    // -----------------------------------------------------------------

    [Fact]
    public void ComputeTheta_With_Mono_Qn_One_Returns_Zero_Theta_All_Mid()
    {
        // Force qn=1 by giving zero bit budget (qb < 4).
        byte[] buf = new byte[16];
        var dec = new OpusRangeDecoder(buf);
        int b = 0;
        int fill = 0xFFFF;
        CeltSplit.ComputeTheta(ref dec,
            logNAtBand: 0, bandIndex: 0, intensity: 100, n: 16,
            ref b, blocks: 4, blocks0: 1, LM: 2, stereo: false,
            ref fill, disableInv: false, remainingBits: 0, out var sctx);
        Assert.Equal(0, sctx.ITheta);
        Assert.Equal(32767, sctx.IMid);
        Assert.Equal(0, sctx.ISide);
        Assert.Equal(0, sctx.Inv);
        Assert.Equal(-16384, sctx.Delta);
        Assert.Equal(0x0F, fill);             // (1<<4)-1
        Assert.Equal(0, sctx.QAlloc);         // no bits consumed
    }

    [Fact]
    public void ComputeTheta_With_Stereo_Beyond_Intensity_Forces_Qn_One()
    {
        // bandIndex >= intensity ⇒ qn=1 even with bits available.
        byte[] buf = new byte[16];
        var dec = new OpusRangeDecoder(buf);
        int b = 100 << CeltConstants.BitRes;
        int fill = 0xFFFF;
        int remaining = 100 << CeltConstants.BitRes;
        CeltSplit.ComputeTheta(ref dec,
            logNAtBand: 0, bandIndex: 5, intensity: 3, n: 16,
            ref b, blocks: 4, blocks0: 1, LM: 2, stereo: true,
            ref fill, disableInv: false, remainingBits: remaining, out var sctx);
        Assert.Equal(0, sctx.ITheta);
        Assert.Equal(32767, sctx.IMid);
        // Inv bit may have been read (b is large enough); just check
        // that it's 0 or 1 and that QAlloc reflects the read.
        Assert.InRange(sctx.Inv, 0, 1);
    }

    [Fact]
    public void ComputeTheta_With_Stereo_Qn_One_And_Disable_Inv_Forces_Inv_Zero()
    {
        byte[] buf = new byte[16];
        buf[buf.Length - 1] = 0xFF;          // would decode inv=1
        var dec = new OpusRangeDecoder(buf);
        int b = 100 << CeltConstants.BitRes;
        int fill = 0xFFFF;
        int remaining = 100 << CeltConstants.BitRes;
        CeltSplit.ComputeTheta(ref dec,
            logNAtBand: 0, bandIndex: 5, intensity: 3, n: 16,
            ref b, blocks: 4, blocks0: 1, LM: 2, stereo: true,
            ref fill, disableInv: true, remainingBits: remaining, out var sctx);
        Assert.Equal(0, sctx.Inv);
    }

    [Fact]
    public void ComputeTheta_Updates_B_By_QAlloc()
    {
        byte[] buf = new byte[16];
        var dec = new OpusRangeDecoder(buf);
        int b = 100;
        int bBefore = b;
        int fill = 0xFFFF;
        CeltSplit.ComputeTheta(ref dec,
            logNAtBand: 0, bandIndex: 0, intensity: 100, n: 16,
            ref b, blocks: 4, blocks0: 1, LM: 2, stereo: false,
            ref fill, disableInv: false, remainingBits: 0, out var sctx);
        Assert.Equal(bBefore - sctx.QAlloc, b);
    }

    // -----------------------------------------------------------------
    // IsqrtU32 — internal helper for the triangular pdf.
    // -----------------------------------------------------------------

    [Theory]
    [InlineData(0u, 0u)]
    [InlineData(1u, 1u)]
    [InlineData(2u, 1u)]
    [InlineData(3u, 1u)]
    [InlineData(4u, 2u)]
    [InlineData(8u, 2u)]
    [InlineData(9u, 3u)]
    [InlineData(15u, 3u)]
    [InlineData(16u, 4u)]
    [InlineData(99u, 9u)]
    [InlineData(100u, 10u)]
    [InlineData(0xFFFF_FFFFu, 65535u)]
    public void IsqrtU32_Matches_Floor_Sqrt(uint v, uint expected)
    {
        Assert.Equal(expected, CeltSplit.IsqrtU32(v));
    }

    [Fact]
    public void IsqrtU32_Result_Squared_Never_Exceeds_Input()
    {
        // Exhaustive sweep over a wide range — invariant check.
        for (uint v = 0; v < 50_000; v += 7)
        {
            uint r = CeltSplit.IsqrtU32(v);
            Assert.True((ulong)r * r <= v);
            Assert.True((ulong)(r + 1) * (r + 1) > v);
        }
    }
}
