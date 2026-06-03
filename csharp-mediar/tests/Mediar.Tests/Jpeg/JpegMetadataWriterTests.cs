using Mediar.Imaging;
using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests.Jpeg;

public sealed class JpegMetadataWriterTests
{
    [Fact]
    public void BuildExifPayload_BeginsWithExifTiffHeader()
    {
        var tags = new Dictionary<string, string> { ["0x010E"] = "Mediar test image" };
        var payload = JpegMetadataWriter.BuildExifPayload(tags);
        Assert.Equal((byte)'E', payload[0]);
        Assert.Equal((byte)'x', payload[1]);
        Assert.Equal((byte)'i', payload[2]);
        Assert.Equal((byte)'f', payload[3]);
        Assert.Equal((byte)0, payload[4]);
        Assert.Equal((byte)0, payload[5]);
    }

    [Fact]
    public void EncodeWithIccProfile_EmitsApp2IccSegment()
    {
        using var src = BuildSolid(16, 16);
        var icc = new byte[2048];
        for (int i = 0; i < icc.Length; i++) icc[i] = (byte)i;

        using var ms = new MemoryStream();
        JpegBaselineEncoder.Encode(src, ms, new JpegEncodeOptions
        {
            Quality = 75, Subsampling = JpegSubsampling.Yuv444, IccProfile = icc,
        });
        var bytes = ms.ToArray();

        bool found = false;
        for (int i = 0; i < bytes.Length - 16; i++)
        {
            if (bytes[i] == 0xFF && bytes[i + 1] == 0xE2)
            {
                string sig = System.Text.Encoding.ASCII.GetString(bytes, i + 4, 11);
                if (sig == "ICC_PROFILE") { found = true; break; }
            }
        }
        Assert.True(found);
    }

    private static ImageFrame BuildSolid(int w, int h)
    {
        var (f, buf) = ImageFrame.Rent(w, h, PixelFormat.Rgb24, w * 3);
        for (int i = 0; i < buf.Length; i += 3) { buf[i] = 200; buf[i + 1] = 100; buf[i + 2] = 50; }
        return f;
    }
}
