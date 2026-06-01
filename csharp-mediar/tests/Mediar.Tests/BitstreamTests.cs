using Mediar.Bitstream;
using Xunit;

namespace Mediar.Tests;

public sealed class BitstreamTests
{
    [Fact]
    public void Finds_3Byte_StartCode()
    {
        byte[] stream = { 0x00, 0x00, 0x01, 0x67, 0x42, 0x80, 0x00, 0x00, 0x01, 0x68, 0xCE };
        int idx = AnnexBScanner.FindNextStartCode(stream, 0);
        Assert.Equal(0, idx);
        int idx2 = AnnexBScanner.FindNextStartCode(stream, 3);
        Assert.Equal(6, idx2);
    }

    [Fact]
    public void FindNalUnits_Returns_Two_Nals()
    {
        byte[] stream = { 0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x80, 0x1F, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x3C, 0x80 };
        var nals = AnnexBScanner.FindNalUnits(stream);
        Assert.Equal(2, nals.Count);
        Assert.Equal(4, nals[0].Offset);
        Assert.Equal(4, nals[0].Length);   // 67 42 80 1F
        Assert.Equal(11, nals[1].Offset);
        Assert.Equal(4, nals[1].Length);   // 68 CE 3C 80
    }

    [Fact]
    public void AnnexB_To_Avcc_And_Back_RoundTrips()
    {
        byte[] annexB = { 0, 0, 0, 1, 0x67, 0x42, 0x80, 0x1F, 0, 0, 0, 1, 0x68, 0xCE, 0x3C, 0x80, 0, 0, 1, 0x65, 0xAA, 0xBB };
        byte[] avcc = AnnexBAvccConverter.AnnexBToLengthPrefixed(annexB, lengthSize: 4);
        byte[] backToAnnexB = AnnexBAvccConverter.LengthPrefixedToAnnexB(avcc, lengthSize: 4);
        // Re-parse both to compare NAL payloads.
        var n1 = AnnexBScanner.FindNalUnits(annexB);
        var n2 = AnnexBScanner.FindNalUnits(backToAnnexB);
        Assert.Equal(n1.Count, n2.Count);
        for (int i = 0; i < n1.Count; i++)
        {
            Assert.Equal(
                annexB.AsSpan(n1[i].Offset, n1[i].Length).ToArray(),
                backToAnnexB.AsSpan(n2[i].Offset, n2[i].Length).ToArray());
        }
    }

    [Fact]
    public void Emulation_Prevention_RoundTrips()
    {
        byte[] rbsp = { 0x67, 0x00, 0x00, 0x00, 0x01, 0x02, 0xFF };
        byte[] ebsp = AnnexBAvccConverter.AddEmulationPrevention(rbsp);
        byte[] back = AnnexBAvccConverter.RemoveEmulationPrevention(ebsp);
        Assert.Equal(rbsp, back);
        Assert.Contains((byte)0x03, ebsp);
        Assert.True(ebsp.Length > rbsp.Length);
    }

    // ----- FindNextStartCode edge cases -----

    [Fact]
    public void FindNextStartCode_NoMatch_ReturnsMinusOne()
    {
        byte[] stream = { 0x01, 0x02, 0x03, 0x04, 0x05 };
        Assert.Equal(-1, AnnexBScanner.FindNextStartCode(stream, 0));
    }

    [Fact]
    public void FindNextStartCode_EmptySpan_ReturnsMinusOne()
    {
        Assert.Equal(-1, AnnexBScanner.FindNextStartCode(Array.Empty<byte>(), 0));
    }

    [Fact]
    public void FindNextStartCode_StartAtEnd_ReturnsMinusOne()
    {
        byte[] stream = { 0x00, 0x00, 0x01, 0x67 };
        Assert.Equal(-1, AnnexBScanner.FindNextStartCode(stream, 4));
    }

    [Fact]
    public void FindNextStartCode_ExercisesAvx2FastPath()
    {
        // Buffer >= 34 bytes triggers the AVX2 SIMD branch (when supported).
        // The match lands in the SIMD chunk so the result must equal the
        // scalar fallback even on machines without AVX2.
        var stream = new byte[64];
        stream[40] = 0x00;
        stream[41] = 0x00;
        stream[42] = 0x01;
        stream[43] = 0x67;
        Assert.Equal(40, AnnexBScanner.FindNextStartCode(stream, 0));
    }

    // ----- FindNalUnits edge cases -----

    [Fact]
    public void FindNalUnits_EmptySpan_ReturnsEmptyList()
    {
        Assert.Empty(AnnexBScanner.FindNalUnits(Array.Empty<byte>()));
    }

    [Fact]
    public void FindNalUnits_NoStartCode_ReturnsEmptyList()
    {
        byte[] stream = { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06 };
        Assert.Empty(AnnexBScanner.FindNalUnits(stream));
    }

    [Fact]
    public void FindNalUnits_SingleNal_ReturnsOneRange()
    {
        byte[] stream = { 0x00, 0x00, 0x00, 0x01, 0x67, 0x42 };
        var nals = AnnexBScanner.FindNalUnits(stream);
        var n = Assert.Single(nals);
        Assert.Equal(4, n.Offset);
        Assert.Equal(2, n.Length);
    }

    [Fact]
    public void NalRange_RecordEquality_Holds()
    {
        Assert.Equal(new NalRange(4, 8), new NalRange(4, 8));
        Assert.NotEqual(new NalRange(4, 8), new NalRange(4, 9));
    }

    // ----- AnnexBToLengthPrefixed length-size variants -----

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(4)]
    public void AnnexB_To_Avcc_RoundTrips_AtEveryValidLengthSize(int lengthSize)
    {
        byte[] annexB = { 0, 0, 0, 1, 0x67, 0x42, 0x80, 0x1F, 0, 0, 1, 0x68, 0xCE };
        var avcc = AnnexBAvccConverter.AnnexBToLengthPrefixed(annexB, lengthSize);
        var back = AnnexBAvccConverter.LengthPrefixedToAnnexB(avcc, lengthSize);
        var n1 = AnnexBScanner.FindNalUnits(annexB);
        var n2 = AnnexBScanner.FindNalUnits(back);
        Assert.Equal(n1.Count, n2.Count);
        for (int i = 0; i < n1.Count; i++)
        {
            Assert.Equal(
                annexB.AsSpan(n1[i].Offset, n1[i].Length).ToArray(),
                back.AsSpan(n2[i].Offset, n2[i].Length).ToArray());
        }
    }

    [Theory]
    [InlineData(0)]
    [InlineData(3)]
    [InlineData(5)]
    [InlineData(8)]
    public void AnnexBToLengthPrefixed_InvalidLengthSize_Throws(int lengthSize)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AnnexBAvccConverter.AnnexBToLengthPrefixed(new byte[] { 0, 0, 1, 0x67 }, lengthSize));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(3)]
    [InlineData(5)]
    [InlineData(8)]
    public void LengthPrefixedToAnnexB_InvalidLengthSize_Throws(int lengthSize)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AnnexBAvccConverter.LengthPrefixedToAnnexB(new byte[] { 0, 0, 0, 1, 0x67 }, lengthSize));
    }

    [Fact]
    public void LengthPrefixedToAnnexB_TruncatedNal_Throws()
    {
        // 4-byte length prefix claims 10 bytes, but only 4 follow.
        byte[] avcc = { 0x00, 0x00, 0x00, 0x0A, 0x67, 0x42, 0x80, 0x1F };
        Assert.Throws<InvalidDataException>(() => AnnexBAvccConverter.LengthPrefixedToAnnexB(avcc, lengthSize: 4));
    }

    [Fact]
    public void LengthPrefixedToAnnexB_Empty_ReturnsEmpty()
    {
        var result = AnnexBAvccConverter.LengthPrefixedToAnnexB(Array.Empty<byte>(), lengthSize: 4);
        Assert.Empty(result);
    }

    [Fact]
    public void LengthPrefixedToAnnexB_PrefixesEveryNalWithFourByteStartCode()
    {
        // Two NALs of 2 bytes each, length-size 4.
        byte[] avcc = { 0, 0, 0, 2, 0x67, 0x42, 0, 0, 0, 2, 0x68, 0xCE };
        var annexB = AnnexBAvccConverter.LengthPrefixedToAnnexB(avcc, lengthSize: 4);
        // Expect: [00 00 00 01] 67 42 [00 00 00 01] 68 CE
        Assert.Equal(12, annexB.Length);
        Assert.Equal(new byte[] { 0, 0, 0, 1, 0x67, 0x42, 0, 0, 0, 1, 0x68, 0xCE }, annexB);
    }

    // ----- Emulation prevention details -----

    [Fact]
    public void RemoveEmulationPrevention_DropsInsertedThreeByte()
    {
        byte[] ebsp = { 0x00, 0x00, 0x03, 0x01 };
        var rbsp = AnnexBAvccConverter.RemoveEmulationPrevention(ebsp);
        Assert.Equal(new byte[] { 0x00, 0x00, 0x01 }, rbsp);
    }

    [Fact]
    public void RemoveEmulationPrevention_NoEmulationBytes_IsPassthrough()
    {
        byte[] ebsp = { 0x67, 0x42, 0x80, 0x1F };
        var rbsp = AnnexBAvccConverter.RemoveEmulationPrevention(ebsp);
        Assert.Equal(ebsp, rbsp);
    }

    [Fact]
    public void AddEmulationPrevention_InsertsThreeByteAfterTwoZerosOnlyForLowBytes()
    {
        // After "00 00", a 0x04 must NOT be preceded by 0x03;
        // a 0x01 must.
        byte[] rbsp = { 0x00, 0x00, 0x04, 0x00, 0x00, 0x01 };
        var ebsp = AnnexBAvccConverter.AddEmulationPrevention(rbsp);
        // 0x04 stays unprefixed; 0x01 gets a 0x03 prefix.
        Assert.Equal(new byte[] { 0x00, 0x00, 0x04, 0x00, 0x00, 0x03, 0x01 }, ebsp);
    }

    [Fact]
    public void AddEmulationPrevention_RoundTrip_OnPseudoRandomData()
    {
        var rnd = new Random(0xC0FFEE);
        var rbsp = new byte[256];
        rnd.NextBytes(rbsp);
        var ebsp = AnnexBAvccConverter.AddEmulationPrevention(rbsp);
        var back = AnnexBAvccConverter.RemoveEmulationPrevention(ebsp);
        Assert.Equal(rbsp, back);
    }
}

