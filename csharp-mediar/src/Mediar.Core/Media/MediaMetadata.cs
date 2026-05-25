using System.Collections.Frozen;
using System.Diagnostics.CodeAnalysis;
using System.Globalization;

namespace Mediar;

/// <summary>
/// A geographic location attached to a media file. Latitude / longitude are
/// in WGS-84 decimal degrees. <see cref="Altitude"/> is in metres above the
/// WGS-84 ellipsoid; <see langword="null"/> when the source does not provide
/// an altitude (e.g. ISO 6709 strings without the third coordinate).
/// </summary>
public readonly record struct GeoLocation(double Latitude, double Longitude, double? Altitude = null)
{
    /// <summary>
    /// Try to parse an ISO 6709 short-form string (used by MP4 <c>©xyz</c> /
    /// 3GPP <c>loci</c> and Matroska <c>LOCATION</c> tags), e.g.
    /// <c>+47.5234-122.3456+0042/</c>.
    /// </summary>
    public static bool TryParseIso6709(ReadOnlySpan<char> input, out GeoLocation value)
    {
        value = default;
        if (input.IsEmpty) return false;
        // Strip optional trailing solidus.
        if (input[^1] == '/') input = input[..^1];

        // Walk the string from left to right; each coordinate begins with '+' or '-'.
        int start = 0;
        Span<Range> ranges = stackalloc Range[3];
        int found = 0;
        for (int i = 1; i < input.Length && found < 3; i++)
        {
            if (input[i] == '+' || input[i] == '-')
            {
                ranges[found++] = new Range(start, i);
                start = i;
            }
        }
        if (found < 1) return false;
        ranges[found++] = new Range(start, input.Length);
        if (found < 2) return false;

        if (!double.TryParse(input[ranges[0]], NumberStyles.Float, CultureInfo.InvariantCulture, out var lat) ||
            !double.TryParse(input[ranges[1]], NumberStyles.Float, CultureInfo.InvariantCulture, out var lon))
        {
            return false;
        }

        double? alt = null;
        if (found == 3 &&
            double.TryParse(input[ranges[2]], NumberStyles.Float, CultureInfo.InvariantCulture, out var altVal))
        {
            alt = altVal;
        }
        value = new GeoLocation(lat, lon, alt);
        return true;
    }
}

/// <summary>
/// File-level metadata extracted from a container. Container demuxers
/// populate as many strongly-typed fields as possible plus an open
/// <see cref="Tags"/> dictionary for anything else. Keys in
/// <see cref="Tags"/> are normalised to upper case so callers can look up
/// fields without worrying about the container's native casing.
/// </summary>
public sealed class MediaMetadata
{
    /// <summary>Singleton empty instance returned by demuxers that find no tags.</summary>
    public static MediaMetadata Empty { get; } = new();

    /// <summary>Track / song / movie title.</summary>
    public string? Title { get; init; }
    /// <summary>Primary artist (performer).</summary>
    public string? Artist { get; init; }
    /// <summary>Album-level artist (compilations: "Various Artists").</summary>
    public string? AlbumArtist { get; init; }
    /// <summary>Album title.</summary>
    public string? Album { get; init; }
    /// <summary>Composer name.</summary>
    public string? Composer { get; init; }
    /// <summary>Genre, free-form text.</summary>
    public string? Genre { get; init; }
    /// <summary>Recording or release date. Format follows the source (ISO 8601 preferred).</summary>
    public string? Date { get; init; }
    /// <summary>1-based track number on its disc.</summary>
    public int? TrackNumber { get; init; }
    /// <summary>Total track count on the disc.</summary>
    public int? TrackCount { get; init; }
    /// <summary>1-based disc number within the album.</summary>
    public int? DiscNumber { get; init; }
    /// <summary>Total disc count for the album.</summary>
    public int? DiscCount { get; init; }
    /// <summary>Free-form comment.</summary>
    public string? Comment { get; init; }
    /// <summary>Long-form description.</summary>
    public string? Description { get; init; }
    /// <summary>Encoder / muxer software identifier.</summary>
    public string? Encoder { get; init; }
    /// <summary>Person or entity that encoded the file.</summary>
    public string? EncodedBy { get; init; }
    /// <summary>Copyright notice.</summary>
    public string? Copyright { get; init; }
    /// <summary>Publisher / record label.</summary>
    public string? Publisher { get; init; }
    /// <summary>BCP-47 language tag.</summary>
    public string? Language { get; init; }
    /// <summary>International Standard Recording Code.</summary>
    public string? Isrc { get; init; }
    /// <summary>Lyrics, when embedded.</summary>
    public string? Lyrics { get; init; }
    /// <summary>Vendor string emitted by the encoder (e.g. Xiph vendor identifier).</summary>
    public string? Vendor { get; init; }
    /// <summary>Geographic location stored in the container.</summary>
    public GeoLocation? Location { get; init; }

    /// <summary>
    /// All extracted raw tag entries keyed by an uppercase canonical name
    /// (typically the Vorbis-comment-style identifier, e.g. <c>TITLE</c>,
    /// <c>ARTIST</c>, <c>LOCATION</c>). Strong properties above are also
    /// present in this dictionary for uniform iteration. Empty when the
    /// container carried no tags.
    /// </summary>
    public IReadOnlyDictionary<string, string> Tags { get; init; } = FrozenDictionary<string, string>.Empty;

    /// <summary>True when this instance carries no information at all.</summary>
    [MemberNotNullWhen(false, nameof(Title), nameof(Artist), nameof(Album), nameof(Date), nameof(Comment))]
    public bool IsEmpty =>
        Tags.Count == 0 &&
        Title is null && Artist is null && AlbumArtist is null && Album is null &&
        Composer is null && Genre is null && Date is null &&
        TrackNumber is null && TrackCount is null && DiscNumber is null && DiscCount is null &&
        Comment is null && Description is null && Encoder is null && EncodedBy is null &&
        Copyright is null && Publisher is null && Language is null && Isrc is null &&
        Lyrics is null && Vendor is null && Location is null;
}

/// <summary>
/// Mutable accumulator for building a <see cref="MediaMetadata"/>. Container
/// demuxers walk their tag chunks once, push every <c>(key, value)</c> pair
/// through <see cref="Set(string, string?)"/>, and finally call
/// <see cref="Build"/>. Recognised keys are mapped to strongly-typed
/// properties; everything else is preserved in <see cref="MediaMetadata.Tags"/>.
/// </summary>
public sealed class MediaMetadataBuilder
{
    private readonly Dictionary<string, string> _tags = new(StringComparer.OrdinalIgnoreCase);

    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Title { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Artist { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? AlbumArtist { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Album { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Composer { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Genre { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Date { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public int? TrackNumber { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public int? TrackCount { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public int? DiscNumber { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public int? DiscCount { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Comment { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Description { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Encoder { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? EncodedBy { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Copyright { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Publisher { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Language { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Isrc { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Lyrics { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public string? Vendor { get; set; }
    /// <summary>Strong-property fields populated from canonical keys.</summary>
    public GeoLocation? Location { get; set; }

    /// <summary>True when no tags have been accumulated.</summary>
    public bool IsEmpty => _tags.Count == 0 && Location is null && Vendor is null;

    /// <summary>
    /// Record a single tag. <paramref name="key"/> is canonicalised to
    /// uppercase; <see langword="null"/> or whitespace values are ignored.
    /// Recognised canonical keys also populate the strong-typed properties.
    /// </summary>
    public void Set(string key, string? value)
    {
        if (string.IsNullOrEmpty(key) || string.IsNullOrEmpty(value)) return;
        string canonical = key.ToUpperInvariant();
        _tags[canonical] = value;

        switch (canonical)
        {
            case "TITLE": Title ??= value; break;
            case "ARTIST":
            case "PERFORMER":
                Artist ??= value; break;
            case "ALBUMARTIST":
            case "ALBUM_ARTIST":
            case "ALBUM ARTIST":
                AlbumArtist ??= value; break;
            case "ALBUM": Album ??= value; break;
            case "COMPOSER": Composer ??= value; break;
            case "GENRE": Genre ??= value; break;
            case "DATE":
            case "YEAR":
            case "DATE_RELEASED":
            case "RELEASE_DATE":
            case "ORIGINALDATE":
                Date ??= value; break;
            case "TRACKNUMBER":
            case "PART_NUMBER":
            case "TRACK":
                ParseTrackOrDisc(value, out var trackNum, out var trackCnt);
                TrackNumber ??= trackNum;
                TrackCount ??= trackCnt;
                break;
            case "TRACKTOTAL":
            case "TOTALTRACKS":
                if (int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out var totT))
                    TrackCount ??= totT;
                break;
            case "DISCNUMBER":
            case "DISC":
                ParseTrackOrDisc(value, out var discNum, out var discCnt);
                DiscNumber ??= discNum;
                DiscCount ??= discCnt;
                break;
            case "DISCTOTAL":
            case "TOTALDISCS":
                if (int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out var totD))
                    DiscCount ??= totD;
                break;
            case "COMMENT":
            case "COMMENTS":
                Comment ??= value; break;
            case "DESCRIPTION":
                Description ??= value; break;
            case "ENCODER":
            case "ENCODING":
            case "ENCODING_TOOL":
                Encoder ??= value; break;
            case "ENCODED_BY":
            case "ENCODEDBY":
                EncodedBy ??= value; break;
            case "COPYRIGHT":
                Copyright ??= value; break;
            case "PUBLISHER":
            case "ORGANIZATION":
            case "LABEL":
                Publisher ??= value; break;
            case "LANGUAGE":
                Language ??= value; break;
            case "ISRC":
                Isrc ??= value; break;
            case "LYRICS":
            case "LYRIC":
                Lyrics ??= value; break;
            case "VENDOR":
                Vendor ??= value; break;
            case "LOCATION":
            case "GEO_LOCATION":
            case "GEOLOCATION":
                if (Location is null && GeoLocation.TryParseIso6709(value, out var loc))
                    Location = loc;
                break;
        }
    }

    /// <summary>Set the vendor identifier (e.g. Xiph vendor string from VorbisComment).</summary>
    public void SetVendor(string vendor)
    {
        if (string.IsNullOrEmpty(vendor)) return;
        Vendor = vendor;
        _tags["VENDOR"] = vendor;
    }

    /// <summary>Set the geographic location directly when the container does not encode it as a tag.</summary>
    public void SetLocation(GeoLocation location)
    {
        Location = location;
        _tags["LOCATION"] = FormatIso6709(location);
    }

    /// <summary>Build the immutable <see cref="MediaMetadata"/> snapshot.</summary>
    public MediaMetadata Build()
    {
        if (IsEmpty) return MediaMetadata.Empty;
        return new MediaMetadata
        {
            Title = Title,
            Artist = Artist,
            AlbumArtist = AlbumArtist,
            Album = Album,
            Composer = Composer,
            Genre = Genre,
            Date = Date,
            TrackNumber = TrackNumber,
            TrackCount = TrackCount,
            DiscNumber = DiscNumber,
            DiscCount = DiscCount,
            Comment = Comment,
            Description = Description,
            Encoder = Encoder,
            EncodedBy = EncodedBy,
            Copyright = Copyright,
            Publisher = Publisher,
            Language = Language,
            Isrc = Isrc,
            Lyrics = Lyrics,
            Vendor = Vendor,
            Location = Location,
            Tags = _tags.ToFrozenDictionary(StringComparer.OrdinalIgnoreCase),
        };
    }

    private static void ParseTrackOrDisc(string value, out int? number, out int? total)
    {
        number = null;
        total = null;
        int slash = value.IndexOf('/');
        var numPart = slash < 0 ? value.AsSpan() : value.AsSpan(0, slash);
        if (int.TryParse(numPart, NumberStyles.Integer, CultureInfo.InvariantCulture, out var n))
            number = n;
        if (slash >= 0 && int.TryParse(value.AsSpan(slash + 1), NumberStyles.Integer, CultureInfo.InvariantCulture, out var t))
            total = t;
    }

    private static string FormatIso6709(GeoLocation loc)
    {
        string lat = (loc.Latitude >= 0 ? "+" : "") + loc.Latitude.ToString("F4", CultureInfo.InvariantCulture);
        string lon = (loc.Longitude >= 0 ? "+" : "") + loc.Longitude.ToString("F4", CultureInfo.InvariantCulture);
        string alt = loc.Altitude is { } a
            ? (a >= 0 ? "+" : "") + a.ToString("F2", CultureInfo.InvariantCulture)
            : "";
        return $"{lat}{lon}{alt}/";
    }
}
