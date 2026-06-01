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

    // ----- Coverage for additional magic-byte branches -----

    [Theory]
    // Container formats (length>=12).
    [InlineData(new byte[] { 0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A }, ImageFormat.Mng)]
    [InlineData(new byte[] { (byte)'R', (byte)'I', (byte)'F', (byte)'F', 0, 0, 0, 0, (byte)'W', (byte)'E', (byte)'B', (byte)'P' }, ImageFormat.WebP)]
    [InlineData(new byte[] { (byte)'R', (byte)'I', (byte)'F', (byte)'F', 0, 0, 0, 0, (byte)'C', (byte)'D', (byte)'R', 0 }, ImageFormat.Cdr)]
    // ISO-BMFF (".... ftyp <brand>") brand routing.
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'h', (byte)'e', (byte)'i', (byte)'c' }, ImageFormat.Heic)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'h', (byte)'e', (byte)'i', (byte)'x' }, ImageFormat.Heic)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'m', (byte)'i', (byte)'f', (byte)'1' }, ImageFormat.Heif)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'a', (byte)'v', (byte)'i', (byte)'f' }, ImageFormat.Avif)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'a', (byte)'v', (byte)'i', (byte)'s' }, ImageFormat.Avif)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'c', (byte)'r', (byte)'x', (byte)' ' }, ImageFormat.Cr3)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'q', (byte)'i', (byte)'f', (byte)' ' }, ImageFormat.Qtif)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'q', (byte)'t', (byte)' ', (byte)' ' }, ImageFormat.Qtif)]
    [InlineData(new byte[] { 0, 0, 0, 0x0C, (byte)'f', (byte)'t', (byte)'y', (byte)'p', (byte)'m', (byte)'p', (byte)'4', (byte)'1' }, ImageFormat.Unknown)]
    // TIFF variants / RAWs sharing TIFF byte-order marks.
    [InlineData(new byte[] { 0x49, 0x49, 0x2A, 0x00, 8, 0, 0, 0, (byte)'C', (byte)'R', 0x02, 0x00 }, ImageFormat.Cr2)]
    [InlineData(new byte[] { 0x49, 0x49, 0x2B, 0x00 }, ImageFormat.Tiff)]                // BigTIFF
    [InlineData(new byte[] { 0x49, 0x49, 0x55, 0x00 }, ImageFormat.Rw2)]                  // Panasonic / Leica
    [InlineData(new byte[] { 0x49, 0x49, 0x52, 0x4F }, ImageFormat.Orf)]                  // Olympus II+RO
    [InlineData(new byte[] { 0x49, 0x49, 0x52, 0x53 }, ImageFormat.Orf)]                  // Olympus II+RS
    [InlineData(new byte[] { 0x4D, 0x4D, 0x4F, 0x52 }, ImageFormat.Orf)]                  // Olympus MM+OR
    // PSD vs PSB (BigEndian version word at offset 4).
    [InlineData(new byte[] { (byte)'8', (byte)'B', (byte)'P', (byte)'S', 0x00, 0x01 }, ImageFormat.Psd)]
    [InlineData(new byte[] { (byte)'8', (byte)'B', (byte)'P', (byte)'S', 0x00, 0x02 }, ImageFormat.Psb)]
    // ICO / CUR
    [InlineData(new byte[] { 0x00, 0x00, 0x01, 0x00 }, ImageFormat.Ico)]
    [InlineData(new byte[] { 0x00, 0x00, 0x02, 0x00 }, ImageFormat.Cur)]
    // DCX (PCX bundle)
    [InlineData(new byte[] { 0xB1, 0x68, 0xDE, 0x3A }, ImageFormat.Dcx)]
    // PVR v3
    [InlineData(new byte[] { (byte)'P', (byte)'V', (byte)'R', 0x03 }, ImageFormat.Pvr)]
    // KTX 1.x / 2.x
    [InlineData(new byte[] { 0xAB, 0x4B, 0x54, 0x58, 0x20, 0x31, 0x31, 0xBB, 0x0D, 0x0A, 0x1A, 0x0A }, ImageFormat.Ktx)]
    [InlineData(new byte[] { 0xAB, 0x4B, 0x54, 0x58, 0x20, 0x32, 0x30, 0xBB, 0x0D, 0x0A, 0x1A, 0x0A }, ImageFormat.Ktx2)]
    // Gzipped vector (SVGZ / EMZ / WMZ) all map to Svgz.
    [InlineData(new byte[] { 0x1F, 0x8B }, ImageFormat.Svgz)]
    // XPM
    [InlineData(new byte[] { (byte)'/', (byte)'*', (byte)' ', (byte)'X', (byte)'P', (byte)'M', (byte)' ', (byte)'*', (byte)'/' }, ImageFormat.Xpm)]
    // PNM family.
    [InlineData(new byte[] { (byte)'P', (byte)'1', (byte)' ' }, ImageFormat.Pbm)]
    [InlineData(new byte[] { (byte)'P', (byte)'4', (byte)'\n' }, ImageFormat.Pbm)]
    [InlineData(new byte[] { (byte)'P', (byte)'2', (byte)'\r' }, ImageFormat.Pgm)]
    [InlineData(new byte[] { (byte)'P', (byte)'5', (byte)'\t' }, ImageFormat.Pgm)]
    [InlineData(new byte[] { (byte)'P', (byte)'3', (byte)' ' }, ImageFormat.Ppm)]
    [InlineData(new byte[] { (byte)'P', (byte)'6', (byte)' ' }, ImageFormat.Ppm)]
    [InlineData(new byte[] { (byte)'P', (byte)'7', (byte)' ' }, ImageFormat.Pnm)]
    // Radiance HDR alternate signature.
    [InlineData(new byte[] { (byte)'#', (byte)'?', (byte)'R', (byte)'G', (byte)'B', (byte)'E' }, ImageFormat.Hdr)]
    // BPG / FLIF.
    [InlineData(new byte[] { 0x42, 0x50, 0x47, 0xFB }, ImageFormat.Bpg)]
    [InlineData(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F' }, ImageFormat.Flif)]
    // JPEG 2000 codestream + container.
    [InlineData(new byte[] { 0xFF, 0x4F, 0xFF, 0x51 }, ImageFormat.J2k)]
    [InlineData(new byte[] { 0x00, 0x00, 0x00, 0x0C, (byte)'j', (byte)'P', 0x20, 0x20, 0x0D, 0x0A, 0x87, 0x0A }, ImageFormat.Jp2)]
    // JPEG XL codestream + container.
    [InlineData(new byte[] { 0xFF, 0x0A }, ImageFormat.Jxl)]
    [InlineData(new byte[] { 0x00, 0x00, 0x00, 0x0C, (byte)'J', (byte)'X', (byte)'L', (byte)' ', 0x0D, 0x0A, 0x87, 0x0A }, ImageFormat.Jxl)]
    // JPEG XR.
    [InlineData(new byte[] { 0x49, 0x49, 0xBC, 0x01 }, ImageFormat.Jxr)]
    // WMF placeable header.
    [InlineData(new byte[] { 0xD7, 0xCD, 0xC6, 0x9A }, ImageFormat.Apm)]
    // WMF plain.
    [InlineData(new byte[] { 0x01, 0x00, 0x09, 0x00 }, ImageFormat.Wmf)]
    // RAF (Fujifilm).
    [InlineData(new byte[] { (byte)'F', (byte)'U', (byte)'J', (byte)'I', (byte)'F', (byte)'I', (byte)'L', (byte)'M', (byte)'C', (byte)'C', (byte)'D', (byte)'-', (byte)'R', (byte)'A', (byte)'W' }, ImageFormat.Raf)]
    // MRW (Konica Minolta).
    [InlineData(new byte[] { 0x00, (byte)'M', (byte)'R', (byte)'M' }, ImageFormat.Mrw)]
    // X3F (Sigma).
    [InlineData(new byte[] { (byte)'F', (byte)'O', (byte)'V', (byte)'b' }, ImageFormat.X3f)]
    // CRW (Canon CIFF) - II + 4-byte hdr-len + "HEAPCCDR".
    [InlineData(new byte[] { 0x49, 0x49, 0x00, 0x00, 0x00, 0x00, (byte)'H', (byte)'E', (byte)'A', (byte)'P', (byte)'C', (byte)'C', (byte)'D', (byte)'R' }, ImageFormat.Crw)]
    // DjVu.
    [InlineData(new byte[] { (byte)'A', (byte)'T', (byte)'&', (byte)'T', (byte)'F', (byte)'O', (byte)'R', (byte)'M' }, ImageFormat.Djvu)]
    // ECW.
    [InlineData(new byte[] { 0x80, 0x42, 0x42, 0x42 }, ImageFormat.Ecw)]
    // PDF / Adobe Illustrator.
    [InlineData(new byte[] { (byte)'%', (byte)'P', (byte)'D', (byte)'F', (byte)'-' }, ImageFormat.Ai)]
    // ZIP container collapses to ODG.
    [InlineData(new byte[] { (byte)'P', (byte)'K', 0x03, 0x04 }, ImageFormat.Odg)]
    // TGA weak-heuristic at end of fall-through.
    [InlineData(new byte[] { 0x05, 0x00, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 }, ImageFormat.Tga)]
    public void DetectsAdditionalMagicByteSequences(byte[] head, ImageFormat expected)
    {
        Assert.Equal(expected, ImageFormatDetector.Detect(head));
    }

    [Fact]
    public void DetectsDicomBySignatureAtOffset128()
    {
        // 132-byte preamble: zero bytes 0..127, "DICM" at 128..131.
        var bytes = new byte[132];
        bytes[128] = (byte)'D';
        bytes[129] = (byte)'I';
        bytes[130] = (byte)'C';
        bytes[131] = (byte)'M';
        Assert.Equal(ImageFormat.Dicom, ImageFormatDetector.Detect(bytes));
    }

    [Fact]
    public void DetectsEmfBySignatureAtOffset40()
    {
        // 44-byte EMR_HEADER: first dword = 0x00000001 (EMR_HEADER record type),
        // ASCII " EMF" sits 40 bytes in.
        var bytes = new byte[44];
        bytes[0] = 0x01;
        bytes[40] = (byte)' '; bytes[41] = (byte)'E';
        bytes[42] = (byte)'M'; bytes[43] = (byte)'F';
        Assert.Equal(ImageFormat.Emf, ImageFormatDetector.Detect(bytes));
    }

    [Fact]
    public void DetectsPvrV2ByLegacySignatureAtOffset44()
    {
        // PVR v2 places "PVR!" at byte 0x2C of a 52-byte header.
        var bytes = new byte[0x30];
        bytes[0x2C] = (byte)'P';
        bytes[0x2D] = (byte)'V';
        bytes[0x2E] = (byte)'R';
        bytes[0x2F] = (byte)'!';
        Assert.Equal(ImageFormat.Pvr, ImageFormatDetector.Detect(bytes));
    }

    [Theory]
    [InlineData(new byte[] { })]
    [InlineData(new byte[] { 0xFF })]
    [InlineData(new byte[] { 0x00, 0x00, 0x00 })]
    public void Detect_ReturnsUnknownForUnrecognisedShortInput(byte[] head)
    {
        Assert.Equal(ImageFormat.Unknown, ImageFormatDetector.Detect(head));
    }

    [Fact]
    public void RecommendedHeaderBytes_Is64()
    {
        Assert.Equal(64, ImageFormatDetector.RecommendedHeaderBytes);
    }

    [Fact]
    public async Task DetectAsync_ReadsFromStreamAndDetects()
    {
        var pngHeader = new byte[] { 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A };
        using var ms = new MemoryStream(pngHeader);
        var fmt = await ImageFormatDetector.DetectAsync(ms);
        Assert.Equal(ImageFormat.Png, fmt);
    }

    [Fact]
    public async Task DetectAsync_ThrowsForNullStream()
    {
        await Assert.ThrowsAsync<ArgumentNullException>(
            async () => await ImageFormatDetector.DetectAsync(null!));
    }

    // ----- Extension mapping: extra paths the production code handles -----

    [Theory]
    [InlineData(@"C:\photos\sunrise.png", ImageFormat.Png)]
    [InlineData("photo.tar.gz.jxl", ImageFormat.Jxl)] // last-dot wins
    [InlineData("png", ImageFormat.Png)]              // dot-less → treated as bare extension
    [InlineData(".tiff", ImageFormat.Tiff)]
    [InlineData(".TIF", ImageFormat.Tiff)]            // case-insensitive
    [InlineData(".dng", ImageFormat.Dng)]
    [InlineData(".webp", ImageFormat.WebP)]
    [InlineData(".weba", ImageFormat.WebA)]
    [InlineData(".mng", ImageFormat.Mng)]
    [InlineData(".apng", ImageFormat.Apng)]
    [InlineData(".jp2", ImageFormat.Jp2)]
    [InlineData(".j2k", ImageFormat.J2k)]
    [InlineData(".dicom", ImageFormat.Dicom)]
    [InlineData(".x3f", ImageFormat.X3f)]
    public void ExtensionMapping_AcceptsPathsAndAdditionalFormats(string input, ImageFormat expected)
    {
        Assert.Equal(expected, ImageFormatExtensions.FromExtension(input));
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    public void ExtensionMapping_NullOrEmpty_Unknown(string? input)
    {
        Assert.Equal(ImageFormat.Unknown, ImageFormatExtensions.FromExtension(input));
    }
}

