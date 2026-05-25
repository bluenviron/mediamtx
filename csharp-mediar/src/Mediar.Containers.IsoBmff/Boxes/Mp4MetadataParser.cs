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
        return null;
    }
}
