using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests.Jpeg;

public sealed class JpegHighBitDepthDecoderTests
{
    [Fact]
    public void Decode_RejectsPrecisionOtherThan12()
    {
        var frame = new JpegFrame { BitsPerSample = 8, NumberOfComponents = 1, IsExtendedSequential = true };
        Assert.Throws<InvalidDataException>(() =>
            JpegHighBitDepthDecoder.Decode(frame, new JpegDecoderState(), Array.Empty<byte>()));
    }
}
