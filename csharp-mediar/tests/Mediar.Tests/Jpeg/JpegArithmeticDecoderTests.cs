using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests.Jpeg;

public sealed class JpegArithmeticDecoderTests
{
    [Fact]
    public void QeTable_Has113Entries()
    {
        Assert.Equal(113, JpegArithmeticDecoder.QeTable.Length);
        Assert.Equal(0x5A1D, JpegArithmeticDecoder.QeTable[0].Qe);
    }

    [Fact]
    public void InitializeDecoder_PrimesAandData()
    {
        var data = new byte[] { 0x12, 0x34, 0x56, 0x78 };
        var s = default(JpegArithmeticDecoder.ArithmeticDecoderState);
        JpegArithmeticDecoder.InitializeDecoder(ref s, data);
        Assert.Equal((uint)0x10000, s.A);
        Assert.NotNull(s.Data);
    }

    [Fact]
    public void Decode_RejectsSof11()
    {
        var frame = new JpegFrame { BitsPerSample = 8, NumberOfComponents = 1, SofMarker = 0xCB };
        var ex = Assert.Throws<InvalidDataException>(() =>
            JpegArithmeticDecoder.Decode(frame, new JpegDecoderState(), Array.Empty<byte>(), 0xCB));
        Assert.Contains("SOF11", ex.Message);
    }

    [Fact]
    public void Decode_RejectsSof10()
    {
        var frame = new JpegFrame { BitsPerSample = 8, NumberOfComponents = 1, SofMarker = 0xCA };
        var ex = Assert.Throws<InvalidDataException>(() =>
            JpegArithmeticDecoder.Decode(frame, new JpegDecoderState(), Array.Empty<byte>(), 0xCA));
        Assert.Contains("SOF10", ex.Message);
    }
}
