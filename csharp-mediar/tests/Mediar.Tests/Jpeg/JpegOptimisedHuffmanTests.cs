using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests.Jpeg;

public sealed class JpegOptimisedHuffmanTests
{
    [Fact]
    public void Build_ProducesValidPrefixCodeWithMaxLength16()
    {
        var freq = new int[257];
        for (int i = 0; i < 16; i++) freq[i] = 1 << (i + 1);
        var (bits, values) = JpegOptimisedHuffman.Build(freq);
        Assert.Equal(16, bits.Length);
        int total = 0; for (int i = 0; i < 16; i++) total += bits[i];
        Assert.Equal(values.Length, total);
        Assert.Equal(values.Length, new HashSet<byte>(values).Count);
    }

    [Fact]
    public void Build_NoSymbols_ProducesEmptyTable()
    {
        var freq = new int[257];
        var (bits, values) = JpegOptimisedHuffman.Build(freq);
        Assert.Equal(16, bits.Length);
        Assert.Empty(values);
    }
}
