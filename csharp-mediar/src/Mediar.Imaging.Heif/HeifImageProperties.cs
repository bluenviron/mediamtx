using System.Buffers.Binary;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view of the HEIF <c>clli</c> property (Content Light Level Information)
/// per CEA-861.3 / SMPTE ST 2086, carrying the static HDR10 luminance metadata
/// for a single image item.
/// </summary>
public sealed record HeifContentLightLevel
{
    /// <summary>Max Content Light Level (CLL) in candela per m^2.</summary>
    public required ushort MaxContentLightLevel { get; init; }

    /// <summary>Max Picture Average Light Level (MaxFALL) in candela per m^2.</summary>
    public required ushort MaxPicAverageLightLevel { get; init; }

    /// <summary>Decodes a raw 4-byte <c>clli</c> payload.</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifContentLightLevel info)
    {
        info = null!;
        if (data.Length < 4) return false;
        info = new HeifContentLightLevel
        {
            MaxContentLightLevel = BinaryPrimitives.ReadUInt16BigEndian(data),
            MaxPicAverageLightLevel = BinaryPrimitives.ReadUInt16BigEndian(data[2..]),
        };
        return true;
    }
}

/// <summary>
/// Typed view of the HEIF <c>mdcv</c> property (Mastering Display Colour
/// Volume) per SMPTE ST 2086, carrying the static HDR10 colour-volume
/// metadata for a single image item. Chromaticity coordinates are in
/// 0.00002 increments (i.e. divide raw value by 50000 to get CIE xy).
/// Luminance values are in 0.0001 cd/m^2 units (divide by 10000).
/// </summary>
public sealed record HeifMasteringDisplayColourVolume
{
    /// <summary>Red display primary chromaticity (CIE xy * 50000).</summary>
    public required (ushort X, ushort Y) DisplayPrimaryR { get; init; }

    /// <summary>Green display primary chromaticity (CIE xy * 50000).</summary>
    public required (ushort X, ushort Y) DisplayPrimaryG { get; init; }

    /// <summary>Blue display primary chromaticity (CIE xy * 50000).</summary>
    public required (ushort X, ushort Y) DisplayPrimaryB { get; init; }

    /// <summary>White-point chromaticity (CIE xy * 50000).</summary>
    public required (ushort X, ushort Y) WhitePoint { get; init; }

    /// <summary>Max display mastering luminance in 0.0001 cd/m^2 units.</summary>
    public required uint MaxDisplayMasteringLuminance { get; init; }

    /// <summary>Min display mastering luminance in 0.0001 cd/m^2 units.</summary>
    public required uint MinDisplayMasteringLuminance { get; init; }

    /// <summary>Decodes a raw 24-byte <c>mdcv</c> payload.</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifMasteringDisplayColourVolume info)
    {
        info = null!;
        if (data.Length < 24) return false;
        info = new HeifMasteringDisplayColourVolume
        {
            DisplayPrimaryR = (
                BinaryPrimitives.ReadUInt16BigEndian(data[0..]),
                BinaryPrimitives.ReadUInt16BigEndian(data[2..])),
            DisplayPrimaryG = (
                BinaryPrimitives.ReadUInt16BigEndian(data[4..]),
                BinaryPrimitives.ReadUInt16BigEndian(data[6..])),
            DisplayPrimaryB = (
                BinaryPrimitives.ReadUInt16BigEndian(data[8..]),
                BinaryPrimitives.ReadUInt16BigEndian(data[10..])),
            WhitePoint = (
                BinaryPrimitives.ReadUInt16BigEndian(data[12..]),
                BinaryPrimitives.ReadUInt16BigEndian(data[14..])),
            MaxDisplayMasteringLuminance = BinaryPrimitives.ReadUInt32BigEndian(data[16..]),
            MinDisplayMasteringLuminance = BinaryPrimitives.ReadUInt32BigEndian(data[20..]),
        };
        return true;
    }
}

/// <summary>
/// Typed view of the HEIF <c>clap</c> property (Clean Aperture) per
/// ISO/IEC 14496-12 §12.1.4.3. Each field is a rational (numerator /
/// denominator). The clean-aperture rectangle is centred at the image
/// centre plus the offset, with the given width and height.
/// </summary>
public sealed record HeifCleanAperture
{
    /// <summary>Clean aperture width as a rational (numerator / denominator).</summary>
    public required (int Numerator, uint Denominator) Width { get; init; }

    /// <summary>Clean aperture height as a rational.</summary>
    public required (int Numerator, uint Denominator) Height { get; init; }

    /// <summary>Horizontal offset of the clean-aperture centre from the image centre, as a signed rational.</summary>
    public required (int Numerator, uint Denominator) HorizontalOffset { get; init; }

    /// <summary>Vertical offset of the clean-aperture centre from the image centre, as a signed rational.</summary>
    public required (int Numerator, uint Denominator) VerticalOffset { get; init; }

    /// <summary>Decodes a raw 32-byte <c>clap</c> payload.</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifCleanAperture info)
    {
        info = null!;
        if (data.Length < 32) return false;
        info = new HeifCleanAperture
        {
            Width = (
                (int)BinaryPrimitives.ReadUInt32BigEndian(data[0..]),
                BinaryPrimitives.ReadUInt32BigEndian(data[4..])),
            Height = (
                (int)BinaryPrimitives.ReadUInt32BigEndian(data[8..]),
                BinaryPrimitives.ReadUInt32BigEndian(data[12..])),
            HorizontalOffset = (
                (int)BinaryPrimitives.ReadUInt32BigEndian(data[16..]),
                BinaryPrimitives.ReadUInt32BigEndian(data[20..])),
            VerticalOffset = (
                (int)BinaryPrimitives.ReadUInt32BigEndian(data[24..]),
                BinaryPrimitives.ReadUInt32BigEndian(data[28..])),
        };
        return true;
    }
}
