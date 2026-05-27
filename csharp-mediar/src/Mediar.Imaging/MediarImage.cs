namespace Mediar.Imaging;

/// <summary>
/// High-level entry point for Mediar's image-reading capabilities.
/// <see cref="Open(string)"/> sniffs the file (by extension and by magic
/// bytes) and dispatches to the correct concrete reader implementation.
/// </summary>
public static class MediarImage
{
    /// <summary>
    /// Open an image from a path. The concrete reader returned implements
    /// <see cref="IImageReader"/> so callers don't need to know which
    /// codec backed it.
    /// </summary>
    /// <param name="path">A path to an existing image file.</param>
    /// <returns>A disposable reader exposing <see cref="ImageInfo"/>,
    /// <see cref="ImageMetadata"/> and a stream of
    /// <see cref="ImageFrame"/>s.</returns>
    public static IImageReader Open(string path)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        var fmt = ImageFormatExtensions.FromExtension(path);

        // Open + read first 32 bytes for magic-byte refinement.
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            Span<byte> head = stackalloc byte[Math.Min(32, (int)Math.Min(fs.Length, 32))];
            int read = 0;
            while (read < head.Length)
            {
                int n = fs.Read(head[read..]);
                if (n <= 0) break;
                read += n;
            }
            fs.Position = 0;
            var detected = ImageFormatDetector.Detect(head[..read]);
            if (detected != ImageFormat.Unknown) fmt = detected;
        }
        catch
        {
            fs.Dispose();
            throw;
        }

        return fmt switch
        {
            ImageFormat.Bmp or ImageFormat.Dib =>
                Mediar.Imaging.Bmp.BmpReader.Open(fs, ownsStream: true),
            ImageFormat.Ico or ImageFormat.Cur =>
                Mediar.Imaging.Bmp.IcoReader.Open(fs, ownsStream: true),
            ImageFormat.Png or ImageFormat.Apng or ImageFormat.Pnj =>
                Mediar.Imaging.Png.PngReader.Open(fs, ownsStream: true),
            ImageFormat.Jpeg or ImageFormat.Jfif or ImageFormat.Mpo
              or ImageFormat.Thm or ImageFormat.JpgLarge =>
                Mediar.Imaging.Jpeg.JpegReader.Open(fs, fmt, ownsStream: true),
            ImageFormat.Gif or ImageFormat.Agif =>
                Mediar.Imaging.Gif.GifReader.Open(fs, ownsStream: true),
            ImageFormat.Tiff =>
                Mediar.Imaging.Tiff.TiffReader.Open(fs, ownsStream: true),
            ImageFormat.Tga =>
                Mediar.Imaging.Tga.TgaReader.Open(fs, ownsStream: true),
            ImageFormat.Pcx or ImageFormat.Dcx =>
                Mediar.Imaging.Pcx.PcxReader.Open(fs, ownsStream: true),
            ImageFormat.Hdr =>
                Mediar.Imaging.Hdr.HdrReader.Open(fs, ownsStream: true),
            ImageFormat.Pnm or ImageFormat.Pbm or ImageFormat.Pgm or ImageFormat.Ppm =>
                Mediar.Imaging.Pnm.PnmReader.Open(fs, fmt, ownsStream: true),
            ImageFormat.Xpm =>
                Mediar.Imaging.Xpm.XpmReader.Open(fs, ownsStream: true),
            ImageFormat.Icns =>
                Mediar.Imaging.Icns.IcnsReader.Open(fs, ownsStream: true),
            ImageFormat.Dds =>
                Mediar.Imaging.Dds.DdsReader.Open(fs, ownsStream: true),
            ImageFormat.Dicom =>
                Mediar.Imaging.Dicom.DicomReader.Open(fs, ownsStream: true),
            ImageFormat.Mng =>
                Mediar.Imaging.Mng.MngReader.Open(fs, ownsStream: true),
            ImageFormat.Svs =>
                Mediar.Imaging.Svs.SvsReader.Open(fs, ownsStream: true),
            ImageFormat.WebP =>
                Mediar.Imaging.WebP.WebPReader.Open(fs, ownsStream: true),
            ImageFormat.Psd or ImageFormat.Psb =>
                Mediar.Imaging.Psd.PsdReader.Open(fs, ownsStream: true),
            ImageFormat.Heic or ImageFormat.Heif or ImageFormat.Avif or ImageFormat.Cr3 =>
                Mediar.Imaging.Heif.HeifReader.Open(fs, fmt, ownsStream: true),
            ImageFormat.Jp2 or ImageFormat.J2k or ImageFormat.J2c or ImageFormat.Jpc
              or ImageFormat.Jpf or ImageFormat.Jpm or ImageFormat.Jpx =>
                Mediar.Imaging.Jpeg2000.Jpeg2000Reader.Open(fs, fmt, ownsStream: true),
            ImageFormat.Jxr =>
                Mediar.Imaging.Jxr.JxrReader.Open(fs, ownsStream: true),
            ImageFormat.Jxl =>
                Mediar.Imaging.Jxl.JxlReader.Open(fs, ownsStream: true),
            ImageFormat.Bpg =>
                Mediar.Imaging.Bpg.BpgReader.Open(fs, ownsStream: true),
            ImageFormat.Flif =>
                Mediar.Imaging.Flif.FlifReader.Open(fs, ownsStream: true),
            ImageFormat.Svgz =>
                Mediar.Imaging.Vector.SvgReader.Open(fs, fmt, ownsStream: true),
            ImageFormat.Emf or ImageFormat.Wmf or ImageFormat.Apm or ImageFormat.Emz or ImageFormat.Wmz =>
                Mediar.Imaging.Metafiles.MetafileReader.Open(fs, fmt, ownsStream: true),
            _ => Mediar.Imaging.Probe.ProbeReader.Open(fs, fmt, ownsStream: true),
        };
    }
}
