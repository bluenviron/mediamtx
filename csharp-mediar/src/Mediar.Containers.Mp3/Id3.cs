using System.Buffers;
using System.Text;
using Mediar.IO;

namespace Mediar.Containers.Mp3;

/// <summary>
/// ID3v2.2 / 2.3 / 2.4 tag parser. Reads the supported text and comment
/// frames and pushes them into a <see cref="MediaMetadataBuilder"/>.
/// </summary>
internal static class Id3v2
{
    public static void Parse(IRandomAccessSource source, byte version, byte flags, int size, MediaMetadataBuilder meta)
    {
        if (size <= 0 || size > 100_000_000) return;

        byte[] buf = ArrayPool<byte>.Shared.Rent(size);
        try
        {
            int n = source.Read(10, buf.AsSpan(0, size));
            if (n != size) return;

            ReadOnlySpan<byte> body = buf.AsSpan(0, size);
            bool unsynchronised = (flags & 0x80) != 0;
            bool hasExtHeader = (flags & 0x40) != 0;

            if (unsynchronised)
            {
                // Remove $FF 00 padding bytes before parsing frames.
                byte[] un = ArrayPool<byte>.Shared.Rent(size);
                try
                {
                    int w = 0;
                    for (int i = 0; i < body.Length; i++)
                    {
                        un[w++] = body[i];
                        if (body[i] == 0xFF && i + 1 < body.Length && body[i + 1] == 0x00) i++;
                    }
                    ParseFrames(un.AsSpan(0, w), version, hasExtHeader, meta);
                }
                finally
                {
                    ArrayPool<byte>.Shared.Return(un);
                }
            }
            else
            {
                ParseFrames(body, version, hasExtHeader, meta);
            }
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(buf);
        }
    }

    private static void ParseFrames(ReadOnlySpan<byte> body, byte version, bool hasExtHeader, MediaMetadataBuilder meta)
    {
        int frameHeaderLen = version == 2 ? 6 : 10;
        int pos = 0;

        if (hasExtHeader && pos + 4 <= body.Length)
        {
            int extSize = version == 4
                ? DecodeSynchsafe(body[pos..(pos + 4)])
                : ReadBeUInt32(body[pos..(pos + 4)]);
            pos += version == 4 ? extSize : extSize + 4;
        }

        while (pos + frameHeaderLen <= body.Length)
        {
            if (body[pos] == 0) break; // padding starts here

            string id;
            int frameSize;
            if (version == 2)
            {
                id = Encoding.ASCII.GetString(body.Slice(pos, 3));
                frameSize = (body[pos + 3] << 16) | (body[pos + 4] << 8) | body[pos + 5];
                pos += 6;
            }
            else
            {
                id = Encoding.ASCII.GetString(body.Slice(pos, 4));
                var sizeSpan = body.Slice(pos + 4, 4);
                frameSize = version == 4 ? DecodeSynchsafe(sizeSpan) : ReadBeUInt32(sizeSpan);
                pos += 10;
            }

            if (frameSize < 0 || pos + frameSize > body.Length) break;
            var data = body.Slice(pos, frameSize);
            pos += frameSize;

            HandleFrame(id, data, meta);
        }
    }

    private static void HandleFrame(string id, ReadOnlySpan<byte> data, MediaMetadataBuilder meta)
    {
        // Text-information frames: first byte = encoding, remainder = NUL-terminated text(s).
        if (id.Length > 0 && id[0] == 'T' && id is not ("TXXX" or "TXX"))
        {
            string? text = DecodeText(data);
            if (text is null) return;
            string mapped = MapTextFrame(id);
            if (mapped.Length == 0) return;
            meta.Set(mapped, text);
            return;
        }

        if (id is "COMM" or "COM")
        {
            // [encoding][lang(3)][short desc(null-term)][actual text]
            if (data.Length < 4) return;
            byte enc = data[0];
            int p = 4; // skip language
            int descEnd = FindStringTerminator(data, p, enc);
            if (descEnd < 0) return;
            int textStart = descEnd + GetTerminatorLength(enc);
            if (textStart > data.Length) return;
            string? text = DecodeString(data[textStart..], enc);
            if (text is not null) meta.Set("COMMENT", text);
            return;
        }

        if (id is "USLT" or "ULT")
        {
            if (data.Length < 4) return;
            byte enc = data[0];
            int p = 4;
            int descEnd = FindStringTerminator(data, p, enc);
            if (descEnd < 0) return;
            int textStart = descEnd + GetTerminatorLength(enc);
            if (textStart > data.Length) return;
            string? text = DecodeString(data[textStart..], enc);
            if (text is not null) meta.Set("LYRICS", text);
            return;
        }

        if (id is "TXXX" or "TXX")
        {
            // [encoding][description \0][value]
            if (data.Length < 2) return;
            byte enc = data[0];
            int p = 1;
            int descEnd = FindStringTerminator(data, p, enc);
            if (descEnd < 0) return;
            string? desc = DecodeString(data[p..descEnd], enc);
            int valueStart = descEnd + GetTerminatorLength(enc);
            if (valueStart > data.Length) return;
            string? value = DecodeString(data[valueStart..], enc);
            if (desc is null || value is null) return;
            meta.Set(desc, value);
            return;
        }

        // URL link frames: plain ISO-8859-1 URL with no encoding prefix.
        // WXXX (user-defined URL) is parsed separately like TXXX.
        if (id.Length > 0 && id[0] == 'W' && id is not ("WXXX" or "WXX"))
        {
            string? url = DecodeUrl(data);
            if (url is null || url.Length == 0) return;
            string mapped = MapUrlFrame(id);
            if (mapped.Length == 0) return;
            meta.Set(mapped, url);
            return;
        }

        if (id is "WXXX" or "WXX")
        {
            if (data.Length < 2) return;
            byte enc = data[0];
            int p = 1;
            int descEnd = FindStringTerminator(data, p, enc);
            if (descEnd < 0) return;
            int valueStart = descEnd + GetTerminatorLength(enc);
            if (valueStart > data.Length) return;
            string? url = DecodeUrl(data[valueStart..]);
            if (url is null || url.Length == 0) return;
            meta.Set("WEBSITE", url);
        }
    }

    private static string MapUrlFrame(string id) => id switch
    {
        "WCOP" or "WCP" => "LICENSE",
        "WOAR" or "WAR" => "WEBSITE",
        "WPUB" or "WPB" => "WEBSITE",
        "WORS" => "WEBSITE",
        "WCOM" or "WCM" => "WEBSITE",
        _ => "",
    };

    private static string? DecodeUrl(ReadOnlySpan<byte> data)
    {
        // URL frames are always ISO-8859-1 with possible trailing nul.
        while (data.Length > 0 && data[^1] == 0) data = data[..^1];
        return data.IsEmpty ? null : Encoding.Latin1.GetString(data);
    }

    private static string MapTextFrame(string id) => id switch
    {
        "TIT2" or "TT2" => "TITLE",
        "TPE1" or "TP1" => "ARTIST",
        "TPE2" or "TP2" => "ALBUMARTIST",
        "TALB" or "TAL" => "ALBUM",
        "TYER" or "TYE" => "DATE",
        "TDRC" => "DATE",
        "TCON" or "TCO" => "GENRE",
        "TRCK" or "TRK" => "TRACKNUMBER",
        "TPOS" or "TPA" => "DISCNUMBER",
        "TCOM" or "TCM" => "COMPOSER",
        "TPUB" or "TPB" => "PUBLISHER",
        "TCOP" or "TCR" => "COPYRIGHT",
        "TSSE" or "TSS" => "ENCODER",
        "TENC" or "TEN" => "ENCODED_BY",
        "TLAN" or "TLA" => "LANGUAGE",
        "TSRC" or "TRC" => "ISRC",
        // ID3v2.3 / v2.4 text frames mapping to extended Vorbis canonical keys.
        "TEXT" or "TXT" => "LYRICIST",
        "TPE3" or "TP3" => "CONDUCTOR",
        "TPE4" or "TP4" => "REMIXER",
        "TBPM" or "TBP" => "BPM",
        "TKEY" or "TKE" => "MUSICALKEY",
        "TMOO" => "MOOD",
        "TCMP" => "COMPILATION",
        "TIT3" or "TT3" => "SUBTITLE",
        "TIT1" or "TT1" => "WORK",
        "TSST" => "DISCSUBTITLE",
        _ => "",
    };

    private static string? DecodeText(ReadOnlySpan<byte> data)
    {
        if (data.IsEmpty) return null;
        byte enc = data[0];
        return DecodeString(data[1..], enc);
    }

    private static string? DecodeString(ReadOnlySpan<byte> data, byte encoding)
    {
        if (data.IsEmpty) return string.Empty;
        // Strip trailing NULs of the appropriate width.
        switch (encoding)
        {
            case 0: // ISO-8859-1
                while (data.Length > 0 && data[^1] == 0) data = data[..^1];
                return Encoding.Latin1.GetString(data);
            case 1: // UTF-16 with BOM
            {
                // Trim trailing 0x00 0x00.
                int n = data.Length & ~1;
                while (n >= 2 && data[n - 1] == 0 && data[n - 2] == 0) n -= 2;
                if (n == 0) return string.Empty;
                if (n >= 2 && data[0] == 0xFF && data[1] == 0xFE)
                    return Encoding.Unicode.GetString(data[2..n]);
                if (n >= 2 && data[0] == 0xFE && data[1] == 0xFF)
                    return Encoding.BigEndianUnicode.GetString(data[2..n]);
                return Encoding.Unicode.GetString(data[..n]);
            }
            case 2: // UTF-16BE without BOM
            {
                int n = data.Length & ~1;
                while (n >= 2 && data[n - 1] == 0 && data[n - 2] == 0) n -= 2;
                return Encoding.BigEndianUnicode.GetString(data[..n]);
            }
            case 3: // UTF-8
                while (data.Length > 0 && data[^1] == 0) data = data[..^1];
                return Encoding.UTF8.GetString(data);
            default:
                return null;
        }
    }

    private static int FindStringTerminator(ReadOnlySpan<byte> data, int start, byte encoding)
    {
        if (encoding is 1 or 2)
        {
            for (int i = start; i + 1 < data.Length; i += 2)
            {
                if (data[i] == 0 && data[i + 1] == 0) return i;
            }
            return -1;
        }
        for (int i = start; i < data.Length; i++)
        {
            if (data[i] == 0) return i;
        }
        return -1;
    }

    private static int GetTerminatorLength(byte encoding) => encoding is 1 or 2 ? 2 : 1;

    private static int DecodeSynchsafe(ReadOnlySpan<byte> b) =>
        ((b[0] & 0x7F) << 21) | ((b[1] & 0x7F) << 14) | ((b[2] & 0x7F) << 7) | (b[3] & 0x7F);

    private static int ReadBeUInt32(ReadOnlySpan<byte> b) =>
        (b[0] << 24) | (b[1] << 16) | (b[2] << 8) | b[3];
}

/// <summary>
/// ID3v1 trailer parser. The trailer is a fixed 128-byte record at the very
/// end of the file: "TAG" + 30B title + 30B artist + 30B album + 4B year +
/// 30B comment + 1B genre (with the ID3v1.1 variant carrying the track
/// number in byte 28 of the comment slot when byte 27 is zero).
/// </summary>
internal static class Id3v1
{
    public static void Parse(IRandomAccessSource source, long offset, MediaMetadataBuilder meta)
    {
        Span<byte> buf = stackalloc byte[128];
        if (source.Read(offset, buf) != 128) return;
        if (buf[0] != 'T' || buf[1] != 'A' || buf[2] != 'G') return;

        meta.Set("TITLE", TrimAscii(buf.Slice(3, 30)));
        meta.Set("ARTIST", TrimAscii(buf.Slice(33, 30)));
        meta.Set("ALBUM", TrimAscii(buf.Slice(63, 30)));
        meta.Set("DATE", TrimAscii(buf.Slice(93, 4)));

        if (buf[125] == 0 && buf[126] != 0)
        {
            // ID3v1.1: track number in byte 126.
            meta.Set("TRACKNUMBER", buf[126].ToString(System.Globalization.CultureInfo.InvariantCulture));
            meta.Set("COMMENT", TrimAscii(buf.Slice(97, 28)));
        }
        else
        {
            meta.Set("COMMENT", TrimAscii(buf.Slice(97, 30)));
        }

        int genreId = buf[127];
        if (genreId < Genres.Length) meta.Set("GENRE", Genres[genreId]);
    }

    private static string TrimAscii(ReadOnlySpan<byte> bytes)
    {
        int end = bytes.IndexOf((byte)0);
        if (end < 0) end = bytes.Length;
        while (end > 0 && (bytes[end - 1] == (byte)' ' || bytes[end - 1] == 0)) end--;
        if (end == 0) return string.Empty;
        return Encoding.Latin1.GetString(bytes[..end]);
    }

    // Standard ID3v1 genre table (subset; index 0..79 are the original Eric Kemp set
    // and 80..147 the WinAMP extension). We keep the first 80 since the extension
    // is rarely seen and adding all of them adds noise.
    private static readonly string[] Genres =
    [
        "Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge", "Hip-Hop",
        "Jazz", "Metal", "New Age", "Oldies", "Other", "Pop", "R&B", "Rap",
        "Reggae", "Rock", "Techno", "Industrial", "Alternative", "Ska", "Death Metal", "Pranks",
        "Soundtrack", "Euro-Techno", "Ambient", "Trip-Hop", "Vocal", "Jazz+Funk", "Fusion", "Trance",
        "Classical", "Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel", "Noise",
        "AlternRock", "Bass", "Soul", "Punk", "Space", "Meditative", "Instrumental Pop", "Instrumental Rock",
        "Ethnic", "Gothic", "Darkwave", "Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance", "Dream",
        "Southern Rock", "Comedy", "Cult", "Gangsta", "Top 40", "Christian Rap", "Pop/Funk", "Jungle",
        "Native American", "Cabaret", "New Wave", "Psychadelic", "Rave", "Showtunes", "Trailer", "Lo-Fi",
        "Tribal", "Acid Punk", "Acid Jazz", "Polka", "Retro", "Musical", "Rock & Roll", "Hard Rock",
    ];
}
