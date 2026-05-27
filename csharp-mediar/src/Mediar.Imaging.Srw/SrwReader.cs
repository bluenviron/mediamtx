using Mediar.Imaging.TiffRaw;

namespace Mediar.Imaging.Srw;

/// <summary>
/// Reader for Samsung NX RAW (SRW) files. SRW is a standard TIFF-based
/// camera-RAW dialect (II/MM byte-order mark + magic 0x002A) identified
/// at parse time by the EXIF <c>Make</c> tag beginning with
/// "SAMSUNG TECHWIN", "SAMSUNG ELECTRONICS CO.,LTD.", "SAMSUNG" or
/// "Samsung". Used by Samsung's NX-series mirrorless cameras and certain
/// Galaxy device modes.
/// </summary>
/// <remarks>
/// <para>
/// All parsing, IFD walking, SubIFD recursion and pixel-decode
/// delegation is performed by <see cref="TiffRawReader"/>. This type is
/// a thin factory that supplies the SRW-specific
/// <see cref="TiffRawConfig"/>.
/// </para>
/// <para>
/// Samsung-compressed RAW (TIFF compression tag 32770) uses a
/// proprietary 12/14-bit delta-coded packing scheme that Mediar does
/// not yet ship - sub-images using that compression are reported as
/// <c>CanDecodePixels = false</c>. Uncompressed (tag 1) and standard
/// JPEG-in-TIFF (tag 7) decode through the existing TIFF stack.
/// </para>
/// </remarks>
public static class SrwReader
{
    /// <summary>SRW <see cref="TiffRawConfig"/>.</summary>
    public static readonly TiffRawConfig Config = new()
    {
        Format = ImageFormat.Srw,
        FormatLabel = "SRW",
        BrandName = "Samsung",
        ProprietaryCompressionTag = 32770,
        IsMatchingMake = IsSamsungMake,
    };

    /// <summary>Open an SRW file from a stream.</summary>
    public static TiffRawReader Open(Stream stream, bool ownsStream = false)
        => TiffRawReader.OpenStandard(stream, Config, ownsStream);

    /// <summary>Open an SRW file from a filesystem path.</summary>
    public static TiffRawReader Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    private static bool IsSamsungMake(string make)
        => make.StartsWith("SAMSUNG", StringComparison.Ordinal)
        || make.StartsWith("Samsung", StringComparison.Ordinal);
}