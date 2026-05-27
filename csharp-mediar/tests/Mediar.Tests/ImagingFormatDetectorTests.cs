using Mediar.Imaging;
using Xunit;

namespace Mediar.Tests;

public sealed class ImagingFormatDetectorTests
{
    [Theory]
    [InlineData(new byte[] { 0x89, (byte)'P', (byte)'N', (byte)'G', 0x0D, 0x0A, 0x1A, 0x0A }, ImageFormat.Png)]
    [InlineData(new byte[] { (byte)'G', (byte)'I', (byte)'F', (byte)'8', (byte)'9', (byte)'a' }, ImageFormat.Gif)]
    [InlineData(new byte[] { (byte)'B', (byte)'M', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 }, ImageFormat.Bmp)]
    [InlineData(new byte[] { 0xFF, 0xD8, 0xFF, 0xE0, 0, 0, (byte)'J', (byte)'F', (byte)'I', (byte)'F' }, ImageFormat.Jpeg)]
    [InlineData(new byte[] { 0x49, 0x49, 0x2A, 0x00, 8, 0, 0, 0 }, ImageFormat.Tiff)]
    [InlineData(new byte[] { 0x4D, 0x4D, 0x00, 0x2A, 0, 0, 0, 8 }, ImageFormat.Tiff)]
    [InlineData(new byte[] { (byte)'D', (byte)'D', (byte)'S', (byte)' ' }, ImageFormat.Dds)]
    [InlineData(new byte[] { (byte)'i', (byte)'c', (byte)'n', (byte)'s', 0, 0, 0, 8 }, ImageFormat.Icns)]
    [InlineData(new byte[] { (byte)'#', (byte)'?', (byte)'R', (byte)'A', (byte)'D', (byte)'I', (byte)'A', (byte)'N', (byte)'C', (byte)'E', (byte)'\n' }, ImageFormat.Hdr)]
    [InlineData(new byte[] { 0x0A, 0x05, 0x01, 0x08, 0, 0, 0, 0, 1, 0, 1, 0 }, ImageFormat.Pcx)]
    public void DetectsCommonMagicByteSequences(byte[] head, ImageFormat expected)
    {
        var actual = ImageFormatDetector.Detect(head);
        Assert.Equal(expected, actual);
    }

    [Fact]
    public void DetectsApngByActlChunk()
    {
        // PNG signature + IHDR length + IHDR + acTL chunk inside the first 64 bytes.
        var bytes = new byte[64];
        new byte[] { 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A }.CopyTo(bytes, 0);
        // place "acTL" at offset 16
        bytes[16] = (byte)'a'; bytes[17] = (byte)'c';
        bytes[18] = (byte)'T'; bytes[19] = (byte)'L';
        Assert.Equal(ImageFormat.Apng, ImageFormatDetector.Detect(bytes));
    }

    [Theory]
    [InlineData(".png", ImageFormat.Png)]
    [InlineData(".PNG", ImageFormat.Png)]
    [InlineData(".jpeg", ImageFormat.Jpeg)]
    [InlineData(".heic", ImageFormat.Heic)]
    [InlineData(".jxl", ImageFormat.Jxl)]
    [InlineData(".cr3", ImageFormat.Cr3)]
    [InlineData(".dcm", ImageFormat.Dicom)]
    [InlineData(".bogus-not-a-format", ImageFormat.Unknown)]
    public void ExtensionMapping(string ext, ImageFormat expected)
    {
        Assert.Equal(expected, ImageFormatExtensions.FromExtension(ext));
    }
}
