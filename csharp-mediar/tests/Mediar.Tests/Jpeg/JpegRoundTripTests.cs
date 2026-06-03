using Mediar.Imaging;
using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests.Jpeg;

public sealed class JpegRoundTripTests
{
    [Fact]
    public async Task ReencodeDecodedFrame_IsPixelStable()
    {
        using var src = BuildGradient(40, 40);
        var opts = new JpegEncodeOptions { Quality = 85, Subsampling = JpegSubsampling.Yuv420 };

        using var ms1 = new MemoryStream();
        JpegBaselineEncoder.Encode(src, ms1, opts);
        using var decoded = await Decode(ms1.ToArray());

        using var ms2 = new MemoryStream();
        JpegBaselineEncoder.Encode(decoded, ms2, opts);
        using var decoded2 = await Decode(ms2.ToArray());

        Assert.Equal(decoded.Width, decoded2.Width);
        Assert.Equal(decoded.Height, decoded2.Height);
        var a = decoded.Pixels.Span; var b = decoded2.Pixels.Span;
        long diff = 0; for (int i = 0; i < a.Length; i++) diff += Math.Abs(a[i] - b[i]);
        Assert.True(diff / (double)a.Length <= 2.0);
    }

    private static ImageFrame BuildGradient(int w, int h)
    {
        var (f, buf) = ImageFrame.Rent(w, h, PixelFormat.Rgb24, w * 3);
        for (int y = 0; y < h; y++)
        for (int x = 0; x < w; x++)
        {
            int i = y * w * 3 + x * 3;
            buf[i + 0] = (byte)(x * 255 / Math.Max(1, w - 1));
            buf[i + 1] = (byte)(y * 255 / Math.Max(1, h - 1));
            buf[i + 2] = 128;
        }
        return f;
    }

    private static async Task<ImageFrame> Decode(byte[] bytes)
    {
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms, ownsStream: false);
        await foreach (var f in reader.ReadFramesAsync()) return f;
        throw new InvalidOperationException("No frame.");
    }
}
