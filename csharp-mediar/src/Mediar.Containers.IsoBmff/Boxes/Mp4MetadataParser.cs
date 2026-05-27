using System.Buffers.Binary;
using System.Text;
using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Parses MP4 / QuickTime metadata atoms:
/// <list type="bullet">
///   <item><c>moov.udta.meta.ilst</c> — iTunes-style key/value (©nam, ©ART, ©alb, …)</item>
///   <item><c>moov.udta.©xyz</c> — 3GPP geographic position (ISO 6709)</item>
///   <item><c>moov.udta.loci</c> — 3GPP TS 26.244 location information box</item>
///   <item>Raw <c>udta</c> children for QuickTime-style 4-letter tag atoms</item>
/// </list>
/// Recognised values are pushed to the supplied
/// <see cref="MediaMetadataBuilder"/>.
/// </summary>
internal static class Mp4MetadataParser
{
    public static void ParseUdta(ReadOnlyMemory<byte> udta, MediaMetadataBuilder meta)
    {
        foreach (var (type, payload) in MovieParser.IterateChildren(udta))
        {
            if (type.Value == BoxTypes.Meta.Value)
            {
                // 'meta' box has a 4-byte FullBox version/flags header before its children.
                ParseMeta(payload.Length >= 4 ? payload[4..] : payload, meta);
            }
            else if (type.Value == BoxTypes.IlXyz.Value)
            {
                ParseQtStringAtom(payload, out var s);
                if (s is not null && GeoLocation.TryParseIso6709(s, out var loc))
                {
                    meta.SetLocation(loc);
                }
            }
            else if (type.Value == BoxTypes.Loci.Value)
            {
                ParseLoci(payload.Span, meta);
            }
            else if (IsCopyrightFourCc(type.Value))
            {
                ParseQtStringAtom(payload, out var s);
                if (s is not null)
                {
                    string key = MapCopyrightAtom(type.Value);
                    if (key.Length > 0) meta.Set(key, s);
                }
            }
        }
    }

    public static void ParseMeta(ReadOnlyMemory<byte> meta, MediaMetadataBuilder builder)
    {
        foreach (var (type, payload) in MovieParser.IterateChildren(meta))
        {
            if (type.Value == BoxTypes.Ilst.Value)
            {
                ParseIlst(payload, builder);
            }
        }
    }

    private static void ParseIlst(ReadOnlyMemory<byte> ilst, MediaMetadataBuilder meta)
    {
        foreach (var (atomType, atomPayload) in MovieParser.IterateChildren(ilst))
        {
            if (atomType.Value == BoxTypes.IlFreeForm.Value)
            {
                ParseFreeFormAtom(atomPayload, meta);
                continue;
            }

            string? key = MapItunesAtom(atomType);
            if (key is null) continue;

            // Each iTunes atom contains one or more children; we only care
            // about the 'data' child which holds the actual value.
            foreach (var (childType, childPayload) in MovieParser.IterateChildren(atomPayload))
            {
                if (childType.Value != BoxTypes.Data.Value) continue;
                if (childPayload.Length < 8) continue;

                var span = childPayload.Span;
                uint typeFlags = BinaryPrimitives.ReadUInt32BigEndian(span);
                // bytes 4..7 = locale; payload starts at offset 8.
                uint dataType = typeFlags & 0x00FFFFFF;
                var value = span[8..];

                if (key == "TRACKNUMBER" && value.Length >= 6)
                {
                    int n = BinaryPrimitives.ReadUInt16BigEndian(value[2..4]);
                    int t = BinaryPrimitives.ReadUInt16BigEndian(value[4..6]);
                    if (n > 0) meta.Set("TRACKNUMBER", t > 0 ? $"{n}/{t}" : n.ToString(System.Globalization.CultureInfo.InvariantCulture));
                }
                else if (key == "DISCNUMBER" && value.Length >= 6)
                {
                    int n = BinaryPrimitives.ReadUInt16BigEndian(value[2..4]);
                    int t = BinaryPrimitives.ReadUInt16BigEndian(value[4..6]);
                    if (n > 0) meta.Set("DISCNUMBER", t > 0 ? $"{n}/{t}" : n.ToString(System.Globalization.CultureInfo.InvariantCulture));
                }
                else if (dataType == 21 && value.Length > 0)
                {
                    // BE signed integer (1, 2, 4, or 8 bytes).
                    long n = ReadBeSignedInt(value);
                    meta.Set(key, n.ToString(System.Globalization.CultureInfo.InvariantCulture));
                }
                else if (dataType == 22 && value.Length > 0)
                {
                    // BE unsigned integer.
                    ulong n = ReadBeUnsignedInt(value);
                    meta.Set(key, n.ToString(System.Globalization.CultureInfo.InvariantCulture));
                }
                else if (dataType == 1)
                {
                    string text = Encoding.UTF8.GetString(value);
                    meta.Set(key, text);
                }
                else if (dataType == 0 && value.Length > 0)
                {
                    string text = Encoding.UTF8.GetString(value);
                    meta.Set(key, text);
                }
                break;
            }
        }
    }

    /// <summary>
    /// Parse an iTunes "----" freeform atom. Layout:
    /// <code>
    /// '----' container
    ///   'mean' FullBox: 4 bytes ver/flags, UTF-8 mean string (e.g. "com.apple.iTunes")
    ///   'name' FullBox: 4 bytes ver/flags, UTF-8 key name (e.g. "MUSICALKEY", "BARCODE")
    ///   'data'         : 4 bytes typeFlags, 4 bytes locale, value
    /// </code>
    /// Used by MusicBrainz Picard, Mp3tag, Roon and similar to write
    /// extended audio tags (BARCODE, CATALOGNUMBER, LICENSE, MOOD,
    /// REPLAYGAIN_*, MUSICBRAINZ_*, sort variants, etc.) that have no
    /// dedicated iTunes 4CC. The key name is normalised to upper-case
    /// before being routed through <see cref="MediaMetadataBuilder.Set"/>
    /// so it lands on the same canonical-key vocabulary as Vorbis and
    /// ID3v2-derived tags.
    /// </summary>
    private static void ParseFreeFormAtom(ReadOnlyMemory<byte> payload, MediaMetadataBuilder meta)
    {
        string? mean = null;
        string? name = null;
        ReadOnlyMemory<byte>? dataChild = null;
        foreach (var (childType, childPayload) in MovieParser.IterateChildren(payload))
        {
            if (childType.Value == BoxTypes.Mean.Value)
            {
                if (childPayload.Length > 4)
                    mean = Encoding.UTF8.GetString(childPayload.Span[4..]);
            }
            else if (childType.Value == BoxTypes.Name.Value)
            {
                if (childPayload.Length > 4)
                    name = Encoding.UTF8.GetString(childPayload.Span[4..]);
            }
            else if (childType.Value == BoxTypes.Data.Value)
            {
                dataChild = childPayload;
            }
        }

        if (string.IsNullOrWhiteSpace(name) || dataChild is null) return;
        // Only accept Apple/iTunes/QuickTime namespaces. Other vendors may
        // legitimately use the same wire format but ship conflicting keys
        // (e.g. Sony's "----:com.sony.xxx" arrays), so we limit the scope.
        if (mean is not null && mean.Length > 0
            && !string.Equals(mean, "com.apple.iTunes", StringComparison.OrdinalIgnoreCase)
            && !string.Equals(mean, "com.apple.QuickTime", StringComparison.OrdinalIgnoreCase))
        {
            return;
        }

        var dataSpan = dataChild.Value.Span;
        if (dataSpan.Length < 8) return;
        uint typeFlags = BinaryPrimitives.ReadUInt32BigEndian(dataSpan);
        uint dataType = typeFlags & 0x00FFFFFF;
        var value = dataSpan[8..];

        string canonicalKey = NormaliseFreeFormKey(name);
        if (canonicalKey.Length == 0) return;

        if (dataType == 21 && value.Length > 0)
        {
            long n = ReadBeSignedInt(value);
            meta.Set(canonicalKey, n.ToString(System.Globalization.CultureInfo.InvariantCulture));
        }
        else if (dataType == 22 && value.Length > 0)
        {
            ulong n = ReadBeUnsignedInt(value);
            meta.Set(canonicalKey, n.ToString(System.Globalization.CultureInfo.InvariantCulture));
        }
        else if (dataType == 1 || dataType == 0)
        {
            if (value.Length == 0) return;
            string text = Encoding.UTF8.GetString(value);
            meta.Set(canonicalKey, text);
        }
    }

    /// <summary>
    /// Map a freeform iTunes name to the canonical Mediar key
    /// vocabulary. Aliases that Picard / Mp3tag / Beets / Roon use
    /// interchangeably are folded onto the primary key. Unknown names
    /// pass through upper-cased so callers can still read them via
    /// <see cref="MediaMetadata.Tags"/>.
    /// </summary>
    private static string NormaliseFreeFormKey(string name)
    {
        string upper = name.ToUpperInvariant();
        return upper switch
        {
            "INITIALKEY" => "MUSICALKEY",
            "REPLAYGAIN_TRACK_GAIN" => "REPLAYGAIN_TRACK_GAIN",
            "REPLAYGAIN_TRACK_PEAK" => "REPLAYGAIN_TRACK_PEAK",
            "REPLAYGAIN_ALBUM_GAIN" => "REPLAYGAIN_ALBUM_GAIN",
            "REPLAYGAIN_ALBUM_PEAK" => "REPLAYGAIN_ALBUM_PEAK",
            "MUSICBRAINZ TRACK ID" => "MUSICBRAINZ_TRACKID",
            "MUSICBRAINZ ALBUM ID" => "MUSICBRAINZ_ALBUMID",
            "MUSICBRAINZ ARTIST ID" => "MUSICBRAINZ_ARTISTID",
            "MUSICBRAINZ ALBUM ARTIST ID" => "MUSICBRAINZ_ALBUMARTISTID",
            "MUSICBRAINZ RELEASE TRACK ID" => "MUSICBRAINZ_RELEASETRACKID",
            "MUSICBRAINZ RELEASE GROUP ID" => "MUSICBRAINZ_RELEASEGROUPID",
            "ACOUSTID ID" => "ACOUSTID_ID",
            "ACOUSTID FINGERPRINT" => "ACOUSTID_FINGERPRINT",
            _ => upper,
        };
    }

    private static long ReadBeSignedInt(ReadOnlySpan<byte> v) => v.Length switch
    {
        1 => (sbyte)v[0],
        2 => BinaryPrimitives.ReadInt16BigEndian(v),
        4 => BinaryPrimitives.ReadInt32BigEndian(v),
        8 => BinaryPrimitives.ReadInt64BigEndian(v),
        _ => (sbyte)v[0],
    };

    private static ulong ReadBeUnsignedInt(ReadOnlySpan<byte> v) => v.Length switch
    {
        1 => v[0],
        2 => BinaryPrimitives.ReadUInt16BigEndian(v),
        4 => BinaryPrimitives.ReadUInt32BigEndian(v),
        8 => BinaryPrimitives.ReadUInt64BigEndian(v),
        _ => v[0],
    };

    private static void ParseLoci(ReadOnlySpan<byte> payload, MediaMetadataBuilder meta)
    {
        // 3GPP TS 26.244 §8.1 LocationInformationBox:
        //   FullBox(loci, 0)
        //   string  language[3]   // bit-packed 5-bit chars
        //   string  name          // null-terminated
        //   uint8   role
        //   uint32  longitude     // 16.16 fixed
        //   uint32  latitude
        //   uint32  altitude
        //   string  astronomical_body
        //   string  additional_notes
        if (payload.Length < 4 + 2 + 1 + 1 + 4 + 4 + 4) return;
        int p = 4; // version + flags
        p += 2;    // packed language
        // Skip null-terminated name.
        while (p < payload.Length && payload[p] != 0) p++;
        if (p >= payload.Length) return;
        p++; // null
        if (p + 1 + 12 > payload.Length) return;
        p++; // role
        double lon = FixedToDouble(BinaryPrimitives.ReadUInt32BigEndian(payload[p..])); p += 4;
        double lat = FixedToDouble(BinaryPrimitives.ReadUInt32BigEndian(payload[p..])); p += 4;
        double alt = FixedToDouble(BinaryPrimitives.ReadUInt32BigEndian(payload[p..])); p += 4;
        meta.SetLocation(new GeoLocation(lat, lon, alt));
    }

    private static double FixedToDouble(uint v)
    {
        // 16.16 signed fixed-point.
        int signed = (int)v;
        return signed / 65536.0;
    }

    /// <summary>Read the typical QuickTime <c>udta</c> child layout: <c>[uint16 length][uint16 language][text]</c>.</summary>
    private static void ParseQtStringAtom(ReadOnlyMemory<byte> payload, out string? text)
    {
        text = null;
        var span = payload.Span;
        if (span.Length < 4) return;
        int len = BinaryPrimitives.ReadUInt16BigEndian(span);
        if (len <= 0 || 4 + len > span.Length) return;
        text = Encoding.UTF8.GetString(span.Slice(4, len));
    }

    private static bool IsCopyrightFourCc(uint value) => (value >> 24) == 0xA9;

    private static string MapCopyrightAtom(uint value)
    {
        char a = (char)(byte)(value >> 16);
        char b = (char)(byte)(value >> 8);
        char c = (char)(byte)value;
        return (a, b, c) switch
        {
            ('n', 'a', 'm') => "TITLE",
            ('A', 'R', 'T') => "ARTIST",
            ('a', 'l', 'b') => "ALBUM",
            ('d', 'a', 'y') => "DATE",
            ('g', 'e', 'n') => "GENRE",
            ('c', 'm', 't') => "COMMENT",
            ('w', 'r', 't') => "COMPOSER",
            ('t', 'o', 'o') => "ENCODER",
            ('l', 'y', 'r') => "LYRICS",
            ('w', 'r', 'k') => "WORK",
            ('g', 'r', 'p') => "WORK",
            ('s', 't', '3') => "SUBTITLE",
            ('c', 'o', 'n') => "CONDUCTOR",
            ('d', 'i', 'r') => "DIRECTOR",
            ('m', 'v', 'n') => "MOVEMENTNAME",
            ('k', 'e', 'y') => "MUSICALKEY",
            ('p', 'u', 'b') => "PUBLISHER",
            _ => "",
        };
    }

    private static string? MapItunesAtom(FourCc t)
    {
        if (t.Value == BoxTypes.IlNam.Value) return "TITLE";
        if (t.Value == BoxTypes.IlArt.Value) return "ARTIST";
        if (t.Value == BoxTypes.IlAlb.Value) return "ALBUM";
        if (t.Value == BoxTypes.IlDay.Value) return "DATE";
        if (t.Value == BoxTypes.IlGen.Value) return "GENRE";
        if (t.Value == BoxTypes.IlCmt.Value) return "COMMENT";
        if (t.Value == BoxTypes.IlWrt.Value) return "COMPOSER";
        if (t.Value == BoxTypes.IlToo.Value) return "ENCODER";
        if (t.Value == BoxTypes.IlLyr.Value) return "LYRICS";
        if (t.Value == BoxTypes.IlGrp.Value) return "ALBUMARTIST";
        if (t.Value == BoxTypes.IlTrk.Value) return "TRACKNUMBER";
        if (t.Value == BoxTypes.IlDsk.Value) return "DISCNUMBER";
        if (t.Value == BoxTypes.IlCpy.Value) return "COPYRIGHT";
        if (t.Value == BoxTypes.IlXyz.Value) return "LOCATION";
        // Extended iTunes atoms.
        if (t.Value == BoxTypes.IlWrk.Value) return "WORK";
        if (t.Value == BoxTypes.IlGroup.Value) return "WORK";
        if (t.Value == BoxTypes.IlSt3.Value) return "SUBTITLE";
        if (t.Value == BoxTypes.IlCon.Value) return "CONDUCTOR";
        if (t.Value == BoxTypes.IlDir.Value) return "DIRECTOR";
        if (t.Value == BoxTypes.IlMvN.Value) return "MOVEMENTNAME";
        if (t.Value == BoxTypes.IlKey.Value) return "MUSICALKEY";
        if (t.Value == BoxTypes.IlPub.Value) return "PUBLISHER";
        if (t.Value == BoxTypes.IlTmpo.Value) return "BPM";
        if (t.Value == BoxTypes.IlCpil.Value) return "COMPILATION";
        if (t.Value == BoxTypes.IlDesc.Value) return "DESCRIPTION";
        if (t.Value == BoxTypes.IlLdes.Value) return "DESCRIPTION";
        return null;
    }
}
