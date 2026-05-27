using Mediar.Imaging.TiffRaw;

namespace Mediar.Imaging.Iiq;

/// <summary>
/// Reader for Phase One IIQ camera-RAW files. IIQ is a standard TIFF-based
/// camera-RAW dialect (II/MM byte-order mark + magic 0x002A) identified at
/// parse time by the EXIF <c>Make</c> tag prefix matching "Phase One".
/// </summary>
/// <remarks>
/// <para>
/// All parsing, IFD walking, SubIFD recursion and pixel-decode delegation
/// is performed by <see cref="TiffRawReader"/>. This type is a thin
/// factory that supplies the IIQ-specific <see cref="TiffRawConfig"/>.
/// </para>
/// <para>
/// Phase One's proprietary IIQ-L (lossless) and IIQ-S (small) compression
/// schemes are encoded under TIFF compression tag 34892. Sub-images using
/// that compression are reported as <c>CanDecodePixels = false</c>.
/// Uncompressed (tag 1), standard JPEG-in-TIFF (tag 7) and deflate (tag
/// 8 / 32946) decode through the existing TIFF stack.
/// </para>
/// </remarks>
public static class IiqReader
{
    /// <summary>IIQ <see cref="TiffRawConfig"/>.</summary>
    public static readonly TiffRawConfig Config = new()
    {
        Format = ImageFormat.Iiq,
        FormatLabel = "IIQ",
        BrandName = "Phase One",
        ProprietaryCompressionTag = 34892,
        IsMatchingMake = IsPhaseOneMake,
    };

    /// <summary>Open an IIQ file from a stream.</summary>
    public static TiffRawReader Open(Stream stream, bool ownsStream = false)
        => TiffRawReader.OpenStandard(stream, Config, ownsStream);

    /// <summary>Open an IIQ file from a filesystem path.</summary>
    public static TiffRawReader Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    private static bool IsPhaseOneMake(string m)
        => m.StartsWith("Phase One", StringComparison.Ordinal)
        || m.StartsWith("PhaseOne", StringComparison.Ordinal)
        || m.StartsWith("PHASE ONE", StringComparison.Ordinal);
}
