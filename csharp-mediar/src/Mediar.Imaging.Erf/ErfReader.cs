using Mediar.Imaging.TiffRaw;

namespace Mediar.Imaging.Erf;

/// <summary>
/// Reader for Epson ERF camera-RAW files. ERF is a standard
/// TIFF-based camera-RAW dialect (II/MM byte-order mark + magic 0x002A)
/// identified at parse time by the EXIF <c>Make</c> tag.
/// </summary>
/// <remarks>
/// <para>
/// All parsing, IFD walking, SubIFD recursion and pixel-decode
/// delegation is performed by <see cref="TiffRawReader"/>. This type is
/// a thin factory that supplies the ERF-specific
/// <see cref="TiffRawConfig"/>.
/// </para>
/// <para>
/// Epson-compressed RAW (TIFF compression tag 65535) uses a
/// proprietary packing scheme that Mediar does not yet ship -
/// sub-images using that compression are reported as
/// <c>CanDecodePixels = false</c>. Uncompressed (tag 1) and standard
/// JPEG-in-TIFF (tag 7) decode through the existing TIFF stack.
/// </para>
/// </remarks>
public static class ErfReader
{
    /// <summary>ERF <see cref="TiffRawConfig"/>.</summary>
    public static readonly TiffRawConfig Config = new()
    {
        Format = ImageFormat.Erf,
        FormatLabel = "ERF",
        BrandName = "Epson",
        ProprietaryCompressionTag = 65535,
        IsMatchingMake = IsEpsonMake,

    };

    /// <summary>Open a ERF file from a stream.</summary>
    public static TiffRawReader Open(Stream stream, bool ownsStream = false)
        => TiffRawReader.OpenStandard(stream, Config, ownsStream);

    /// <summary>Open a ERF file from a filesystem path.</summary>
    public static TiffRawReader Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    private static bool IsEpsonMake(string m)
        => m.StartsWith("EPSON", StringComparison.Ordinal) || m.StartsWith("Epson", StringComparison.Ordinal) || m.StartsWith("SEIKO EPSON", StringComparison.Ordinal);
}