using System.Collections.Frozen;

namespace Mediar.Imaging;

/// <summary>
/// Container for tag-style image metadata: EXIF, IPTC, XMP plus a
/// simplified "headline" view of the most-requested values (title, author,
/// description, GPS, camera).
/// </summary>
public sealed class ImageMetadata
{
    /// <summary>Title / object name.</summary>
    public string? Title { get; init; }

    /// <summary>Creator / author / artist.</summary>
    public string? Author { get; init; }

    /// <summary>Free-form description.</summary>
    public string? Description { get; init; }

    /// <summary>Copyright notice.</summary>
    public string? Copyright { get; init; }

    /// <summary>Software / encoder string.</summary>
    public string? Software { get; init; }

    /// <summary>Capture / creation timestamp.</summary>
    public DateTimeOffset? CapturedAt { get; init; }

    /// <summary>Original capture-time string, exactly as stored (rarely round-trips a real DateTime).</summary>
    public string? CapturedAtRaw { get; init; }

    /// <summary>Camera make (EXIF Make).</summary>
    public string? CameraMake { get; init; }

    /// <summary>Camera model (EXIF Model).</summary>
    public string? CameraModel { get; init; }

    /// <summary>Lens model (EXIF LensModel).</summary>
    public string? LensModel { get; init; }

    /// <summary>Image orientation (EXIF Orientation, 1..8).</summary>
    public int? Orientation { get; init; }

    /// <summary>GPS latitude in decimal degrees, north positive.</summary>
    public double? GpsLatitude { get; init; }

    /// <summary>GPS longitude in decimal degrees, east positive.</summary>
    public double? GpsLongitude { get; init; }

    /// <summary>GPS altitude in meters above sea level (negative = below).</summary>
    public double? GpsAltitudeMeters { get; init; }

    /// <summary>Exposure time in seconds.</summary>
    public double? ExposureTimeSeconds { get; init; }

    /// <summary>F-number (aperture).</summary>
    public double? FNumber { get; init; }

    /// <summary>ISO speed rating.</summary>
    public int? IsoSpeed { get; init; }

    /// <summary>Focal length in millimeters.</summary>
    public double? FocalLengthMm { get; init; }

    /// <summary>All raw EXIF / IPTC / XMP / format-specific tags, keyed by IFD-prefixed name.</summary>
    public FrozenDictionary<string, string> Tags { get; init; } = FrozenDictionary<string, string>.Empty;

    /// <summary>Empty metadata sentinel.</summary>
    public static ImageMetadata Empty { get; } = new();
}
