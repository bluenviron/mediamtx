using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacStandardSpectralCodebooksTests
{
    [Theory]
    [InlineData(1, 81)]
    [InlineData(2, 81)]
    [InlineData(3, 81)]
    [InlineData(4, 81)]
    [InlineData(5, 81)]
    [InlineData(6, 81)]
    [InlineData(7, 64)]
    [InlineData(8, 64)]
    [InlineData(9, 169)]
    [InlineData(10, 169)]
    [InlineData(11, 289)]
    public void GetCodebook_HasExpectedSymbolCount(int cbNumber, int expectedSymbols)
    {
        var book = AacStandardSpectralCodebooks.GetCodebook(cbNumber);
        Assert.NotNull(book);
        Assert.Equal(expectedSymbols, book.SymbolCount);
        Assert.Equal(expectedSymbols, book.Capacity);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    [InlineData(12)]
    [InlineData(100)]
    [InlineData(int.MinValue)]
    [InlineData(int.MaxValue)]
    public void GetCodebook_OutOfRange_Throws(int cbNumber)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacStandardSpectralCodebooks.GetCodebook(cbNumber));
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    [InlineData(5)]
    [InlineData(6)]
    [InlineData(7)]
    [InlineData(8)]
    [InlineData(9)]
    [InlineData(10)]
    [InlineData(11)]
    public void GetCodebook_ReturnsSameSharedInstance(int cbNumber)
    {
        var a = AacStandardSpectralCodebooks.GetCodebook(cbNumber);
        var b = AacStandardSpectralCodebooks.GetCodebook(cbNumber);
        Assert.Same(a, b);
    }

    [Fact]
    public void ConvenienceProperties_MatchGetCodebook()
    {
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(1), AacStandardSpectralCodebooks.Cb1);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(2), AacStandardSpectralCodebooks.Cb2);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(3), AacStandardSpectralCodebooks.Cb3);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(4), AacStandardSpectralCodebooks.Cb4);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(5), AacStandardSpectralCodebooks.Cb5);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(6), AacStandardSpectralCodebooks.Cb6);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(7), AacStandardSpectralCodebooks.Cb7);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(8), AacStandardSpectralCodebooks.Cb8);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(9), AacStandardSpectralCodebooks.Cb9);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(10), AacStandardSpectralCodebooks.Cb10);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(11), AacStandardSpectralCodebooks.Cb11);
    }

    [Theory]
    [InlineData(1, 11)]
    [InlineData(2, 9)]
    [InlineData(3, 16)]
    [InlineData(4, 12)]
    [InlineData(5, 13)]
    [InlineData(6, 11)]
    [InlineData(7, 12)]
    [InlineData(8, 10)]
    [InlineData(9, 15)]
    [InlineData(10, 12)]
    [InlineData(11, 12)]
    public void Codebook_HasExpectedMaxCodeLength(int cbNumber, int expectedMaxLen)
    {
        // Reference values cross-checked against the bitsN arrays in
        // FFmpeg's libavcodec/aactab.c.
        var book = AacStandardSpectralCodebooks.GetCodebook(cbNumber);
        Assert.Equal(expectedMaxLen, book.MaxCodeLength);
    }

    [Fact]
    public void Cb1_FirstCodewordDecodes_To_Symbol0()
    {
        // From FFmpeg aactab.c: codes1[0] = 0x7f8, bits1[0] = 11.
        // 0x7f8 = 2040 → 11-bit codeword "11111111000".
        // Packed MSB-first into byte stream: 1111_1111 000x_xxxx
        // = 0xFF, 0x00 (last 5 bits unused).
        var book = AacStandardSpectralCodebooks.Cb1;
        byte[] data = { 0xFF, 0x00 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var sym));
        Assert.Equal(0, sym);
    }

    [Fact]
    public void Cb1_MiddleCodewordDecodes_To_Symbol40()
    {
        // From FFmpeg aactab.c: codes1[40] = 0x0000, bits1[40] = 1.
        // Single-bit code "0" - the most-likely symbol in a signed
        // 4-tuple is the all-zero tuple at the centre of the table.
        var book = AacStandardSpectralCodebooks.Cb1;
        byte[] data = { 0b0000_0000 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var sym));
        Assert.Equal(40, sym);
    }

    [Fact]
    public void Cb7_FirstCodewordDecodes_To_Symbol0()
    {
        // From FFmpeg aactab.c: codes7[0] = 0x000, bits7[0] = 1.
        // Single-bit "0" → byte 0x00 first bit decodes to symbol 0.
        var book = AacStandardSpectralCodebooks.Cb7;
        byte[] data = { 0b0000_0000 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var sym));
        Assert.Equal(0, sym);
    }

    [Fact]
    public void Cb9_FirstCodewordDecodes_To_Symbol0()
    {
        // From FFmpeg aactab.c: codes9[0] = 0x0000, bits9[0] = 1.
        // Single-bit "0" for the all-zero 2-tuple (most likely value).
        var book = AacStandardSpectralCodebooks.Cb9;
        byte[] data = { 0b0000_0000 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var sym));
        Assert.Equal(0, sym);
    }

    [Fact]
    public void Cb11_LastTableEntryDecodes_To_Symbol288()
    {
        // From FFmpeg aactab.c: codes11[288] = 0x004, bits11[288] = 5.
        // Codeword "00100" → byte 0010_0xxx left-aligned = 0x20.
        var book = AacStandardSpectralCodebooks.Cb11;
        byte[] data = { 0b0010_0000 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var sym));
        Assert.Equal(288, sym);
    }

    [Fact]
    public void Cb5_KnownCodewords_Decode_In_Sequence()
    {
        // cb 5 is a 2-tuple signed codebook with 81 symbols where the
        // most-likely symbol (idx 40 = pair (0, 0)) has the single-bit
        // "0" code from FFmpeg aactab.c: codes5[40] = 0x0000, bits5[40] = 1.
        // Decode three back-to-back zeros: "0 0 0" → 0000_0000 = 0x00.
        var book = AacStandardSpectralCodebooks.Cb5;
        byte[] data = { 0x00 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var v0));
        Assert.Equal(40, v0);
        Assert.True(book.TryDecode(ref reader, out var v1));
        Assert.Equal(40, v1);
        Assert.True(book.TryDecode(ref reader, out var v2));
        Assert.Equal(40, v2);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    [InlineData(5)]
    [InlineData(6)]
    [InlineData(7)]
    [InlineData(8)]
    [InlineData(9)]
    [InlineData(10)]
    [InlineData(11)]
    public void Codebook_BuildsLazily_WithoutThrowing(int cbNumber)
    {
        // Force the Lazy<T> to evaluate and confirm no Kraft / overflow
        // exception escapes from the construction of any of the eleven
        // tables.
        var book = AacStandardSpectralCodebooks.GetCodebook(cbNumber);
        Assert.NotNull(book);
        Assert.True(book.MaxCodeLength is >= 1 and <= 16);
    }
}
