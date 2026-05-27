using Mediar.Imaging.TiffRaw;

namespace Mediar.Imaging.Mef;

/// <summary>
/// Reader for Mamiya MEF camera-RAW files. MEF is a standard
/// TIFF-based camera-RAW dialect (II/MM byte-order mark + magic 0x002A)
/// identified at parse time by the EXIF <c>Make</c> tag.
/// </summary>
/// <remarks>
/// <para>
/// All parsing, IFD walking, SubIFD recursion and pixel-decode
/// delegation is performed by <see cref="TiffRawReader"/>. This type is
/// a thin factory that supplies the MEF-specific
/// <see cref="TiffRawConfig"/>.
/// </para>
/// <para>
/// Mamiya-compressed RAW (TIFF compression tag 65000) uses a
/// proprietary packing scheme that Mediar does not yet ship -
/// sub-images using that compression are reported as
/// <c>CanDecodePixels = false</c>. Uncompressed (tag 1) and standard
/// JPEG-in-TIFF (tag 7) decode through the existing TIFF stack.
/// </para>
/// </remarks>
public static class MefReader
{
    /// <summary>MEF <see cref="TiffRawConfig"/>.</summary>
    public static readonly TiffRawConfig Config = new()
    {
        Format = ImageFormat.Mef,
        FormatLabel = "MEF",
        BrandName = "Mamiya",
        ProprietaryCompressionTag = 65000,
        IsMatchingMake = IsMamiyaMake,

    };

    /// <summary>Open a MEF file from a stream.</summary>
    public static TiffRawReader Open(Stream stream, bool ownsStream = false)
        => TiffRawReader.OpenStandard(stream, Config, ownsStream);

    /// <summary>Open a MEF file from a filesystem path.</summary>
    public static TiffRawReader Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    private static bool IsMamiyaMake(string m)
        => m.StartsWith("Mamiya", StringComparison.Ordinal) || m.StartsWith("MAMIYA", StringComparison.Ordinal) || m.StartsWith("Phase One", StringComparison.Ordinal);
}