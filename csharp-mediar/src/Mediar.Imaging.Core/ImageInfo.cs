using System.Collections.Frozen;

namespace Mediar.Imaging;

/// <summary>
/// Lightweight, immutable view of an image header. Always available even
/// when the codec for actual pixel decode is not implemented.
/// </summary>
public readonly record struct ImageInfo
{
    /// <summary>Width in pixels (best known; 0 when unknown).</summary>
    public int Width { get; init; }

    /// <summary>Height in pixels (best known; 0 when unknown).</summary>
    public int Height { get; init; }

    /// <summary>Bits per pixel as stored on disk.</summary>
    public int BitsPerPixel { get; init; }

    /// <summary>Number of color channels in the source.</summary>
    public int ChannelCount { get; init; }

    /// <summary>Decoded pixel format. <see cref="PixelFormat.Unknown"/> if not decodable.</summary>
    public PixelFormat PixelFormat { get; init; }

    /// <summary>Concrete image format (PNG, JPEG, …).</summary>
    public ImageFormat Format { get; init; }

    /// <summary>True if the source declares an alpha channel.</summary>
    public bool HasAlpha { get; init; }

    /// <summary>True if the source is HDR / floating-point.</summary>
    public bool IsHdr { get; init; }

    /// <summary>True if the source supports animation and contains more than one frame.</summary>
    public bool IsAnimated { get; init; }

    /// <summary>Frame count for animated/multi-page images (1 for stills, 0 if unknown).</summary>
    public int FrameCount { get; init; }

    /// <summary>Horizontal DPI / PPI; 0 if unknown.</summary>
    public double HorizontalDpi { get; init; }

    /// <summary>Vertical DPI / PPI; 0 if unknown.</summary>
    public double VerticalDpi { get; init; }

    /// <summary>Color profile / space hint (e.g. "sRGB", "Adobe RGB", "ProPhoto", "CMYK").</summary>
    public string? ColorSpace { get; init; }

    /// <summary>Raw color profile bytes if the file embeds an ICC profile.</summary>
    public ReadOnlyMemory<byte> IccProfile { get; init; }
}

/// <summary>
/// Top-level enum for every image format Mediar knows about, even ones
/// for which only detection is supported. Use
/// <see cref="ImageFormatDetector"/> to classify a byte stream.
/// </summary>
public enum ImageFormat
{
    /// <summary>Unknown / unsupported.</summary>
    Unknown = 0,

    // ---------- common raster formats ----------
    Bmp, Dib, Ico, Cur, Icns, Xpm,
    Png, Apng, Pnj, Mng, Flif,
    Jpeg, Jfif, Mpo, Thm, JpgLarge,
    Gif, Agif,
    Tiff, Pnm, Pbm, Pgm, Ppm,
    Tga,
    Pcx, Dcx,
    Dds,
    Hdr,
    WebP, WebA,
    Psd, Psb,

    // ---------- modern still containers ----------
    Heic, Heif, Avif, Bpg,
    Jxl, Jxr,
    Jp2, J2k, J2c, Jpc, Jpf, Jpm, Jpx,

    // ---------- camera RAW ----------
    Arw, Bay, Cr2, Cr3, Dcr, Dng, Nef, Orf, Pef, Raf, Raw, Rpf, Rw2, Spp, Art, Mix,

    // ---------- medical / scientific ----------
    Dicom, Djvu, Ecw, Svs, Bif,

    // ---------- vector / metafile ----------
    Svgz, Emf, Emz, Wmf, Wmz, Apm, Pict,
    Ai, Cdr, Cdx, Cmx,
    Odg, Otg, Fodg,
    Fig, Ink, Skp,
    Qtif,

    // ---------- editor proprietary ----------
    Afphoto, Afx, Agp, Clip, Cpc, Csl, Gbr, Pat, Pdn, Psp, PspImage, Pvt, Xpr,

    // ---------- aliases (treated as PNG / GIF / JPEG variants on disk) ----------
    Exif, Art2, Sdt, Webp2,
}

/// <summary>
/// Maps file extensions to <see cref="ImageFormat"/> values. Case-insensitive
/// lookup; values mirror the user-facing extension list verbatim.
/// </summary>
public static class ImageFormatExtensions
{
    private static readonly FrozenDictionary<string, ImageFormat> s_byExtension =
        new Dictionary<string, ImageFormat>(StringComparer.OrdinalIgnoreCase)
        {
            // raster
            [".bmp"] = ImageFormat.Bmp, [".dib"] = ImageFormat.Dib,
            [".ico"] = ImageFormat.Ico, [".cur"] = ImageFormat.Cur,
            [".icns"] = ImageFormat.Icns, [".xpm"] = ImageFormat.Xpm,
            [".png"] = ImageFormat.Png, [".apng"] = ImageFormat.Apng,
            [".pnj"] = ImageFormat.Pnj, [".mng"] = ImageFormat.Mng,
            [".flif"] = ImageFormat.Flif,
            [".jpg"] = ImageFormat.Jpeg, [".jpeg"] = ImageFormat.Jpeg,
            [".jfif"] = ImageFormat.Jfif, [".jpg_large"] = ImageFormat.JpgLarge,
            [".mpo"] = ImageFormat.Mpo, [".thm"] = ImageFormat.Thm,
            [".gif"] = ImageFormat.Gif, [".agif"] = ImageFormat.Agif,
            [".tif"] = ImageFormat.Tiff, [".tiff"] = ImageFormat.Tiff,
            [".pnm"] = ImageFormat.Pnm, [".pbm"] = ImageFormat.Pbm,
            [".pgm"] = ImageFormat.Pgm, [".ppm"] = ImageFormat.Ppm,
            [".tga"] = ImageFormat.Tga,
            [".pcx"] = ImageFormat.Pcx, [".dcx"] = ImageFormat.Dcx,
            [".dds"] = ImageFormat.Dds, [".hdr"] = ImageFormat.Hdr,
            [".webp"] = ImageFormat.WebP, [".weba"] = ImageFormat.WebA,
            [".psd"] = ImageFormat.Psd, [".psb"] = ImageFormat.Psb,
            // modern
            [".heic"] = ImageFormat.Heic, [".heif"] = ImageFormat.Heif,
            [".avif"] = ImageFormat.Avif, [".bpg"] = ImageFormat.Bpg,
            [".jxl"] = ImageFormat.Jxl, [".jxr"] = ImageFormat.Jxr,
            [".jp2"] = ImageFormat.Jp2, [".j2k"] = ImageFormat.J2k,
            [".j2c"] = ImageFormat.J2c, [".jpc"] = ImageFormat.Jpc,
            [".jpf"] = ImageFormat.Jpf, [".jpm"] = ImageFormat.Jpm,
            [".jpx"] = ImageFormat.Jpx,
            // RAW
            [".arw"] = ImageFormat.Arw, [".bay"] = ImageFormat.Bay,
            [".cr2"] = ImageFormat.Cr2, [".cr3"] = ImageFormat.Cr3,
            [".dcr"] = ImageFormat.Dcr, [".dng"] = ImageFormat.Dng,
            [".nef"] = ImageFormat.Nef, [".orf"] = ImageFormat.Orf, [".pef"] = ImageFormat.Pef,
            [".raf"] = ImageFormat.Raf, [".raw"] = ImageFormat.Raw,
            [".rpf"] = ImageFormat.Rpf, [".rw2"] = ImageFormat.Rw2, [".spp"] = ImageFormat.Spp,
            [".art"] = ImageFormat.Art, [".mix"] = ImageFormat.Mix,
            // medical / scientific
            [".dcm"] = ImageFormat.Dicom, [".dicom"] = ImageFormat.Dicom,
            [".djvu"] = ImageFormat.Djvu, [".ecw"] = ImageFormat.Ecw,
            [".svs"] = ImageFormat.Svs, [".bif"] = ImageFormat.Bif,
            // vector / metafile
            [".svgz"] = ImageFormat.Svgz,
            [".emf"] = ImageFormat.Emf, [".emz"] = ImageFormat.Emz,
            [".wmf"] = ImageFormat.Wmf, [".wmz"] = ImageFormat.Wmz,
            [".apm"] = ImageFormat.Apm, [".pict"] = ImageFormat.Pict, [".pct"] = ImageFormat.Pict,
            [".ai"] = ImageFormat.Ai, [".cdr"] = ImageFormat.Cdr,
            [".cdx"] = ImageFormat.Cdx, [".cmx"] = ImageFormat.Cmx,
            [".odg"] = ImageFormat.Odg, [".otg"] = ImageFormat.Otg, [".fodg"] = ImageFormat.Fodg,
            [".fig"] = ImageFormat.Fig, [".ink"] = ImageFormat.Ink, [".skp"] = ImageFormat.Skp,
            [".qtif"] = ImageFormat.Qtif,
            // proprietary
            [".afphoto"] = ImageFormat.Afphoto, [".afx"] = ImageFormat.Afx,
            [".agp"] = ImageFormat.Agp, [".clip"] = ImageFormat.Clip,
            [".cpc"] = ImageFormat.Cpc, [".csl"] = ImageFormat.Csl,
            [".gbr"] = ImageFormat.Gbr, [".pat"] = ImageFormat.Pat,
            [".pdn"] = ImageFormat.Pdn, [".psp"] = ImageFormat.Psp,
            [".pspimage"] = ImageFormat.PspImage, [".pvt"] = ImageFormat.Pvt,
            [".xpr"] = ImageFormat.Xpr,
            // aliases
            [".exif"] = ImageFormat.Exif,
        }
        .ToFrozenDictionary(StringComparer.OrdinalIgnoreCase);

    /// <summary>Returns the canonical <see cref="ImageFormat"/> for a file extension.</summary>
    public static ImageFormat FromExtension(string? extensionOrPath)
    {
        if (string.IsNullOrEmpty(extensionOrPath))
        {
            return ImageFormat.Unknown;
        }
        var ext = extensionOrPath.AsSpan();
        var dot = ext.LastIndexOf('.');
        var key = dot >= 0 ? ext[dot..].ToString() : ("." + extensionOrPath);
        return s_byExtension.TryGetValue(key, out var fmt) ? fmt : ImageFormat.Unknown;
    }
}
