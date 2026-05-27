using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Metadata;

/// <summary>
/// EXIF tag parser. Operates over a TIFF-style IFD payload (the bytes
/// following the <c>"Exif\0\0"</c> identifier in JPEG APP1, or the entire
/// TIFF file when reading TIFF/RAW directly). Resolves both standard and
/// GPS sub-IFDs.
/// </summary>
public static class ExifParser
{
    /// <summary>
    /// Parse the supplied TIFF payload and return a populated
    /// <see cref="ImageMetadata"/>.
    /// </summary>
    public static ImageMetadata Parse(ReadOnlySpan<byte> payload)
    {
        if (payload.Length < 8) return ImageMetadata.Empty;

        bool littleEndian;
        if (payload[0] == 'I' && payload[1] == 'I') littleEndian = true;
        else if (payload[0] == 'M' && payload[1] == 'M') littleEndian = false;
        else return ImageMetadata.Empty;

        ushort magic = ReadU16(payload, 2, littleEndian);
        if (magic != 42 && magic != 0x2B) return ImageMetadata.Empty;
        uint firstIfdOffset = ReadU32(payload, 4, littleEndian);

        var tags = new Dictionary<string, string>(StringComparer.Ordinal);

        ReadIfd(payload, firstIfdOffset, littleEndian, tags, prefix: "IFD0:");

        // GPS sub-IFD ?
        if (tags.TryGetValue("IFD0:GPSInfoIFDPointer", out var gpsPtrStr) &&
            uint.TryParse(gpsPtrStr, out var gpsPtr))
        {
            ReadIfd(payload, gpsPtr, littleEndian, tags, prefix: "GPS:");
        }
        if (tags.TryGetValue("IFD0:ExifIFDPointer", out var exifPtrStr) &&
            uint.TryParse(exifPtrStr, out var exifPtr))
        {
            ReadIfd(payload, exifPtr, littleEndian, tags, prefix: "Exif:");
        }

        return BuildMetadata(tags);
    }

    private static void ReadIfd(
        ReadOnlySpan<byte> payload, uint offset, bool le,
        Dictionary<string, string> sink, string prefix)
    {
        if (offset == 0 || offset + 2 > payload.Length) return;
        ushort count = ReadU16(payload, (int)offset, le);
        int p = (int)offset + 2;
        for (int i = 0; i < count; i++)
        {
            if (p + 12 > payload.Length) return;
            ushort tag = ReadU16(payload, p, le);
            ushort type = ReadU16(payload, p + 2, le);
            uint nComponents = ReadU32(payload, p + 4, le);
            uint valOffset = ReadU32(payload, p + 8, le);
            string name = ResolveTagName(prefix, tag);
            string val = DecodeValue(payload, type, nComponents, valOffset, le);
            if (!string.IsNullOrEmpty(val)) sink[prefix + name] = val;
            p += 12;
        }
    }

    private static string DecodeValue(
        ReadOnlySpan<byte> payload, ushort type, uint nComponents, uint valOffset, bool le)
    {
        int sizePerComponent = type switch
        {
            1 or 2 or 6 or 7 => 1,
            3 or 8 => 2,
            4 or 9 or 11 => 4,
            5 or 10 or 12 => 8,
            _ => 0,
        };
        if (sizePerComponent == 0) return "";
        long byteSize = (long)nComponents * sizePerComponent;
        ReadOnlySpan<byte> data;
        if (byteSize <= 4)
        {
            Span<byte> inline = stackalloc byte[4];
            if (le) BinaryPrimitives.WriteUInt32LittleEndian(inline, valOffset);
            else BinaryPrimitives.WriteUInt32BigEndian(inline, valOffset);
            data = inline[..(int)Math.Min((int)byteSize, 4)].ToArray();
        }
        else
        {
            if (valOffset + byteSize > payload.Length) return "";
            data = payload.Slice((int)valOffset, (int)byteSize);
        }

        return type switch
        {
            // ASCII
            2 => CleanAscii(data),
            // BYTE / UNDEFINED
            1 or 7 => BytesToString(data),
            // SHORT
            3 => ShortsToString(data, le),
            // LONG
            4 => LongsToString(data, le),
            // RATIONAL
            5 => RationalsToString(data, le, signed: false),
            // SLONG
            9 => SignedLongsToString(data, le),
            // SRATIONAL
            10 => RationalsToString(data, le, signed: true),
            _ => "",
        };
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16(ReadOnlySpan<byte> b, int off, bool le)
        => le ? BinaryPrimitives.ReadUInt16LittleEndian(b.Slice(off, 2))
              : BinaryPrimitives.ReadUInt16BigEndian(b.Slice(off, 2));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(ReadOnlySpan<byte> b, int off, bool le)
        => le ? BinaryPrimitives.ReadUInt32LittleEndian(b.Slice(off, 4))
              : BinaryPrimitives.ReadUInt32BigEndian(b.Slice(off, 4));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int ReadI32(ReadOnlySpan<byte> b, int off, bool le)
        => le ? BinaryPrimitives.ReadInt32LittleEndian(b.Slice(off, 4))
              : BinaryPrimitives.ReadInt32BigEndian(b.Slice(off, 4));

    private static string CleanAscii(ReadOnlySpan<byte> b)
    {
        int end = b.IndexOf((byte)0);
        if (end < 0) end = b.Length;
        return Encoding.ASCII.GetString(b[..end]).Trim();
    }

    private static string BytesToString(ReadOnlySpan<byte> b)
    {
        if (b.Length == 0) return "";
        var sb = new StringBuilder();
        for (int i = 0; i < b.Length; i++)
        {
            if (i > 0) sb.Append(' ');
            sb.Append(b[i]);
        }
        return sb.ToString();
    }

    private static string ShortsToString(ReadOnlySpan<byte> b, bool le)
    {
        var sb = new StringBuilder();
        for (int i = 0; i + 2 <= b.Length; i += 2)
        {
            if (i > 0) sb.Append(' ');
            sb.Append(ReadU16(b, i, le));
        }
        return sb.ToString();
    }

    private static string LongsToString(ReadOnlySpan<byte> b, bool le)
    {
        var sb = new StringBuilder();
        for (int i = 0; i + 4 <= b.Length; i += 4)
        {
            if (i > 0) sb.Append(' ');
            sb.Append(ReadU32(b, i, le));
        }
        return sb.ToString();
    }

    private static string SignedLongsToString(ReadOnlySpan<byte> b, bool le)
    {
        var sb = new StringBuilder();
        for (int i = 0; i + 4 <= b.Length; i += 4)
        {
            if (i > 0) sb.Append(' ');
            sb.Append(ReadI32(b, i, le));
        }
        return sb.ToString();
    }

    private static string RationalsToString(ReadOnlySpan<byte> b, bool le, bool signed)
    {
        var sb = new StringBuilder();
        for (int i = 0; i + 8 <= b.Length; i += 8)
        {
            if (i > 0) sb.Append(' ');
            if (signed)
            {
                sb.Append(ReadI32(b, i, le)).Append('/').Append(ReadI32(b, i + 4, le));
            }
            else
            {
                sb.Append(ReadU32(b, i, le)).Append('/').Append(ReadU32(b, i + 4, le));
            }
        }
        return sb.ToString();
    }

    private static string ResolveTagName(string prefix, ushort tag) => prefix switch
    {
        "IFD0:" or "Exif:" => StandardName(tag),
        "GPS:" => GpsName(tag),
        _ => $"Tag{tag:X4}",
    };

    private static string StandardName(ushort tag) => tag switch
    {
        0x0100 => "ImageWidth",
        0x0101 => "ImageHeight",
        0x010E => "ImageDescription",
        0x010F => "Make",
        0x0110 => "Model",
        0x0112 => "Orientation",
        0x011A => "XResolution",
        0x011B => "YResolution",
        0x0128 => "ResolutionUnit",
        0x0131 => "Software",
        0x0132 => "DateTime",
        0x013B => "Artist",
        0x8298 => "Copyright",
        0x8769 => "ExifIFDPointer",
        0x8825 => "GPSInfoIFDPointer",
        0x829A => "ExposureTime",
        0x829D => "FNumber",
        0x8827 => "ISOSpeedRatings",
        0x9003 => "DateTimeOriginal",
        0x9004 => "DateTimeDigitized",
        0x920A => "FocalLength",
        0xA432 => "LensSpecification",
        0xA433 => "LensMake",
        0xA434 => "LensModel",
        _ => $"Tag{tag:X4}",
    };

    private static string GpsName(ushort tag) => tag switch
    {
        0x0000 => "GPSVersionID",
        0x0001 => "GPSLatitudeRef",
        0x0002 => "GPSLatitude",
        0x0003 => "GPSLongitudeRef",
        0x0004 => "GPSLongitude",
        0x0005 => "GPSAltitudeRef",
        0x0006 => "GPSAltitude",
        0x0007 => "GPSTimeStamp",
        0x001D => "GPSDateStamp",
        _ => $"GpsTag{tag:X4}",
    };

    private static ImageMetadata BuildMetadata(Dictionary<string, string> tags)
    {
        return new ImageMetadata
        {
            Title = tags.GetValueOrDefault("IFD0:ImageDescription"),
            Author = tags.GetValueOrDefault("IFD0:Artist"),
            Copyright = tags.GetValueOrDefault("IFD0:Copyright"),
            Software = tags.GetValueOrDefault("IFD0:Software"),
            CameraMake = tags.GetValueOrDefault("IFD0:Make"),
            CameraModel = tags.GetValueOrDefault("IFD0:Model"),
            LensModel = tags.GetValueOrDefault("Exif:LensModel"),
            Orientation = TryInt(tags.GetValueOrDefault("IFD0:Orientation")),
            CapturedAtRaw = tags.GetValueOrDefault("Exif:DateTimeOriginal")
                          ?? tags.GetValueOrDefault("IFD0:DateTime"),
            CapturedAt = ParseExifTime(tags.GetValueOrDefault("Exif:DateTimeOriginal")
                                    ?? tags.GetValueOrDefault("IFD0:DateTime")),
            ExposureTimeSeconds = ParseRational(tags.GetValueOrDefault("Exif:ExposureTime")),
            FNumber = ParseRational(tags.GetValueOrDefault("Exif:FNumber")),
            FocalLengthMm = ParseRational(tags.GetValueOrDefault("Exif:FocalLength")),
            IsoSpeed = TryInt(tags.GetValueOrDefault("Exif:ISOSpeedRatings")),
            GpsLatitude = ParseGpsDegree(tags, "GPS:GPSLatitude", "GPS:GPSLatitudeRef"),
            GpsLongitude = ParseGpsDegree(tags, "GPS:GPSLongitude", "GPS:GPSLongitudeRef"),
            GpsAltitudeMeters = ParseGpsAltitude(tags),
            Tags = tags.ToFrozenDictionary(),
        };
    }

    private static int? TryInt(string? s)
        => int.TryParse(s, out var i) ? i : null;

    private static double? ParseRational(string? s)
    {
        if (string.IsNullOrEmpty(s)) return null;
        int slash = s.IndexOf('/');
        if (slash < 0 ||
            !double.TryParse(s.AsSpan(0, slash), System.Globalization.CultureInfo.InvariantCulture, out var num) ||
            !double.TryParse(s.AsSpan(slash + 1), System.Globalization.CultureInfo.InvariantCulture, out var den) ||
            den == 0)
        {
            return null;
        }
        return num / den;
    }

    private static DateTimeOffset? ParseExifTime(string? s)
    {
        if (string.IsNullOrEmpty(s)) return null;
        // EXIF format: "YYYY:MM:DD HH:MM:SS"
        if (DateTime.TryParseExact(s, "yyyy:MM:dd HH:mm:ss",
                System.Globalization.CultureInfo.InvariantCulture,
                System.Globalization.DateTimeStyles.AssumeLocal, out var dt))
        {
            return new DateTimeOffset(dt);
        }
        return null;
    }

    private static double? ParseGpsDegree(
        Dictionary<string, string> tags, string valKey, string refKey)
    {
        if (!tags.TryGetValue(valKey, out var triplet)) return null;
        var parts = triplet.Split(' ');
        if (parts.Length < 3) return null;
        double deg = ParseRational(parts[0]) ?? 0;
        double min = ParseRational(parts[1]) ?? 0;
        double sec = ParseRational(parts[2]) ?? 0;
        double v = deg + (min / 60.0) + (sec / 3600.0);
        if (tags.TryGetValue(refKey, out var dir) && (dir == "S" || dir == "W")) v = -v;
        return v;
    }

    private static double? ParseGpsAltitude(Dictionary<string, string> tags)
    {
        if (!tags.TryGetValue("GPS:GPSAltitude", out var v)) return null;
        double alt = ParseRational(v) ?? 0;
        if (tags.TryGetValue("GPS:GPSAltitudeRef", out var r) && r.Trim() == "1") alt = -alt;
        return alt;
    }
}
