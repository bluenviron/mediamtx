using Mediar.Imaging.TiffRaw;

namespace Mediar.Imaging.ThreeFr;

/// <summary>
/// Reader for Hasselblad 3FR camera-RAW files. 3FR is a standard
/// TIFF-based camera-RAW dialect (II/MM byte-order mark + magic 0x002A)
/// identified at parse time by the EXIF <c>Make</c> tag.
/// </summary>
/// <remarks>
/// <para>
/// All parsing, IFD walking, SubIFD recursion and pixel-decode
/// delegation is performed by <see cref="TiffRawReader"/>. This type is
/// a thin factory that supplies the 3FR-specific
/// <see cref="TiffRawConfig"/>.
/// </para>
/// <para>
/// Hasselblad-compressed RAW (TIFF compression tag 32767) uses a
/// proprietary packing scheme that Mediar does not yet ship -
/// sub-images using that compression are reported as
/// <c>CanDecodePixels = false</c>. Uncompressed (tag 1) and standard
/// JPEG-in-TIFF (tag 7) decode through the existing TIFF stack.
/// </para>
/// </remarks>
public static class ThreeFrReader
{
    /// <summary>3FR <see cref="TiffRawConfig"/>.</summary>
    public static readonly TiffRawConfig Config = new()
    {
        Format = ImageFormat.ThreeFr,
        FormatLabel = "3FR",
        BrandName = "Hasselblad",
        ProprietaryCompressionTag = 32767,
        IsMatchingMake = IsHasselbladMake,
        IsVendorUndecodable = (comp, photo, _) => photo == 32803 && comp != 1,
    };

    /// <summary>Open a 3FR file from a stream.</summary>
    public static TiffRawReader Open(Stream stream, bool ownsStream = false)
        => TiffRawReader.OpenStandard(stream, Config, ownsStream);

    /// <summary>Open a 3FR file from a filesystem path.</summary>
    public static TiffRawReader Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    private static bool IsHasselbladMake(string m)
        => m.StartsWith("Hasselblad", StringComparison.Ordinal) || m.StartsWith("HASSELBLAD", StringComparison.Ordinal);
}