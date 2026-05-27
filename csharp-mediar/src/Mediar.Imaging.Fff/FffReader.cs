using Mediar.Imaging.TiffRaw;

namespace Mediar.Imaging.Fff;

/// <summary>
/// Reader for Hasselblad FFF (Flexible File Format) camera-RAW files. FFF
/// is a standard TIFF-based camera-RAW / processed-RAW dialect (II/MM
/// byte-order mark + magic 0x002A) produced by the Hasselblad Phocus
/// workflow. It is identified at parse time by the EXIF <c>Make</c> tag
/// prefix matching "Hasselblad".
/// </summary>
/// <remarks>
/// <para>
/// All parsing, IFD walking, SubIFD recursion and pixel-decode delegation
/// is performed by <see cref="TiffRawReader"/>. This type is a thin
/// factory that supplies the FFF-specific <see cref="TiffRawConfig"/>.
/// </para>
/// <para>
/// FFF shares the underlying CFA-Bayer compression scheme with 3FR on
/// legacy H1D / H2D bodies (TIFF compression tag 32767) and also surfaces
/// the same "compressed + CFA photometric" pairing as undecodable.
/// Uncompressed FFF (tag 1) and processed FFF stored as standard
/// JPEG-in-TIFF (tag 7) decode through the existing TIFF stack.
/// </para>
/// </remarks>
public static class FffReader
{
    /// <summary>FFF <see cref="TiffRawConfig"/>.</summary>
    public static readonly TiffRawConfig Config = new()
    {
        Format = ImageFormat.Fff,
        FormatLabel = "FFF",
        BrandName = "Hasselblad",
        ProprietaryCompressionTag = 32767,
        IsMatchingMake = IsHasselbladMake,
        IsVendorUndecodable = (comp, photo, _) => photo == 32803 && comp != 1,
    };

    /// <summary>Open an FFF file from a stream.</summary>
    public static TiffRawReader Open(Stream stream, bool ownsStream = false)
        => TiffRawReader.OpenStandard(stream, Config, ownsStream);

    /// <summary>Open an FFF file from a filesystem path.</summary>
    public static TiffRawReader Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    private static bool IsHasselbladMake(string m)
        => m.StartsWith("Hasselblad", StringComparison.Ordinal)
        || m.StartsWith("HASSELBLAD", StringComparison.Ordinal);
}
