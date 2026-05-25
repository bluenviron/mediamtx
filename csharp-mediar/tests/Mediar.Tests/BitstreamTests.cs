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
}
