using Mediar.Imaging.TiffRaw;

namespace Mediar.Imaging.Dcr;

/// <summary>
/// Reader for Kodak DCR camera-RAW files. DCR is a standard
/// TIFF-based camera-RAW dialect (II/MM byte-order mark + magic 0x002A)
/// identified at parse time by the EXIF <c>Make</c> tag.
/// </summary>
/// <remarks>
/// <para>
/// All parsing, IFD walking, SubIFD recursion and pixel-decode
/// delegation is performed by <see cref="TiffRawReader"/>. This type is
/// a thin factory that supplies the DCR-specific
/// <see cref="TiffRawConfig"/>.
/// </para>
/// <para>
/// Kodak-compressed RAW (TIFF compression tag 65000) uses a
/// proprietary packing scheme that Mediar does not yet ship -
/// sub-images using that compression are reported as
/// <c>CanDecodePixels = false</c>. Uncompressed (tag 1) and standard
/// JPEG-in-TIFF (tag 7) decode through the existing TIFF stack.
/// </para>
/// </remarks>
public static class DcrReader
{
    /// <summary>DCR <see cref="TiffRawConfig"/>.</summary>
    public static readonly TiffRawConfig Config = new()
    {
        Format = ImageFormat.Dcr,
        FormatLabel = "DCR",
        BrandName = "Kodak",
        ProprietaryCompressionTag = 65000,
        IsMatchingMake = IsKodakMake,

    };

    /// <summary>Open a DCR file from a stream.</summary>
    public static TiffRawReader Open(Stream stream, bool ownsStream = false)
        => TiffRawReader.OpenStandard(stream, Config, ownsStream);

    /// <summary>Open a DCR file from a filesystem path.</summary>
    public static TiffRawReader Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    private static bool IsKodakMake(string m)
        => m.StartsWith("EASTMAN KODAK", StringComparison.Ordinal) || m.StartsWith("KODAK", StringComparison.Ordinal) || m.StartsWith("Kodak", StringComparison.Ordinal);
}