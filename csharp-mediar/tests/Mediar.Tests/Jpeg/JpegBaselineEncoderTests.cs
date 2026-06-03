using Mediar.Imaging;
using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests.Jpeg;

public sealed class JpegBaselineEncoderTests
{
    private static ImageFrame BuildRgbGradient(int width, int height)
    {
        var (frame, buf) = ImageFrame.Rent(width, height, PixelFormat.Rgb24, width * 3);
        for (int y = 0; y < height; y++)
        for (int x = 0; x < width; x++)
        {
            int i = y * width * 3 + x * 3;
            buf[i + 0] = (byte)(x * 255 / Math.Max(1, width - 1));
            buf[i + 1] = (byte)(y * 255 / Math.Max(1, height - 1));
            buf[i + 2] = (byte)((x + y) * 255 / Math.Max(1, width + height - 2));
        }
        return frame;
    }

    private static async Task<ImageFrame> DecodeAsync(byte[] encoded)
    {
        await using var ms = new MemoryStream(encoded);
        using var reader = JpegReader.Open(ms, ownsStream: false);
        await foreach (var f in reader.ReadFramesAsync()) return f;
        throw new InvalidOperationException("Encoder produced no decodable frame.");
    }

    [Fact]
    public async Task EncodeRgb_Yuv444_Q90_HasSoiEoiAndDecodesPlausibly()
    {
        using var src = BuildRgbGradient(64, 64);
        using var ms = new MemoryStream();
        JpegBaselineEncoder.Encode(src, ms, new JpegEncodeOptions { Quality = 90, Subsampling = JpegSubsampling.Yuv444 });
        var bytes = ms.ToArray();
        Assert.True(bytes.Length > 4);
        Assert.Equal(0xFF, bytes[0]); Assert.Equal(0xD8, bytes[1]);
        Assert.Equal(0xFF, bytes[^2]); Assert.Equal(0xD9, bytes[^1]);
        using var decoded = await DecodeAsync(bytes);
        Assert.Equal(64, decoded.Width); Assert.Equal(64, decoded.Height);
    }

    [Fact]
    public async Task EncodeRgb_AllSubsamplings_ProduceDecodableJpegs()
    {
        foreach (var ss in new[] { JpegSubsampling.Yuv444, JpegSubsampling.Yuv422, JpegSubsampling.Yuv420 })
        {
            using var src = BuildRgbGradient(32, 32);
            using var ms = new MemoryStream();
            JpegBaselineEncoder.Encode(src, ms, new JpegEncodeOptions { Quality = 80, Subsampling = ss });
            using var decoded = await DecodeAsync(ms.ToArray());
            Assert.Equal(32, decoded.Width); Assert.Equal(32, decoded.Height);
        }
    }
}
