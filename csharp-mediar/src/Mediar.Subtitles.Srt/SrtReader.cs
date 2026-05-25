using System.Globalization;
using System.Text;

namespace Mediar.Subtitles.Srt;

/// <summary>
/// SubRip (<c>.srt</c>) reader. Iterates cues without buffering the full file
/// when given a <see cref="TextReader"/>. UTF-8 BOM is consumed transparently.
/// </summary>
public static class SrtReader
{
    /// <summary>Open <paramref name="path"/> and read every cue.</summary>
    public static IEnumerable<SrtCue> ReadFile(string path)
    {
        using var reader = new StreamReader(path, Encoding.UTF8, detectEncodingFromByteOrderMarks: true);
        foreach (var cue in Read(reader)) yield return cue;
    }

    /// <summary>Parse a string containing SRT content.</summary>
    public static IEnumerable<SrtCue> ReadString(string content)
    {
        using var reader = new StringReader(content);
        foreach (var cue in Read(reader)) yield return cue;
    }

    /// <summary>Stream cues from <paramref name="reader"/>.</summary>
    public static IEnumerable<SrtCue> Read(TextReader reader)
    {
        ArgumentNullException.ThrowIfNull(reader);

        var text = new StringBuilder();
        int implicitIndex = 0;
        while (true)
        {
            string? indexLine = ReadNonEmptyLine(reader);
            if (indexLine is null) yield break;

            int cueIndex;
            string? timecodeLine;
            if (!int.TryParse(indexLine, NumberStyles.Integer, CultureInfo.InvariantCulture, out cueIndex))
            {
                // Some SRT files omit the index entirely; treat this line as the timecode.
                cueIndex = ++implicitIndex;
                timecodeLine = indexLine;
            }
            else
            {
                implicitIndex = cueIndex;
                timecodeLine = reader.ReadLine();
                if (timecodeLine is null) yield break;
            }

            if (!TryParseTimecodeLine(timecodeLine, out var start, out var end)) yield break;

            text.Clear();
            while (true)
            {
                string? line = reader.ReadLine();
                if (line is null || line.Length == 0) break;
                if (text.Length > 0) text.Append('\n');
                text.Append(line);
            }

            yield return new SrtCue(cueIndex, start, end, text.ToString());
        }
    }

    private static string? ReadNonEmptyLine(TextReader reader)
    {
        while (true)
        {
            string? line = reader.ReadLine();
            if (line is null) return null;
            if (line.Length == 0) continue;
            // Skip optional leading UTF-8 BOM characters that some editors leave behind.
            int start = 0;
            while (start < line.Length && line[start] == '\uFEFF') start++;
            if (start > 0) line = line[start..];
            if (line.Length == 0) continue;
            return line;
        }
    }

    private static bool TryParseTimecodeLine(string line, out TimeSpan start, out TimeSpan end)
    {
        start = end = TimeSpan.Zero;
        int sep = line.IndexOf("-->", StringComparison.Ordinal);
        if (sep < 0) return false;
        var left = line.AsSpan(0, sep).Trim();
        var right = line.AsSpan(sep + 3).Trim();

        // Right side may carry optional position attributes after the timecode.
        int sp = right.IndexOf(' ');
        if (sp > 0) right = right[..sp];

        return TryParseTimecode(left, out start) && TryParseTimecode(right, out end);
    }

    /// <summary>Parse an SRT timecode <c>HH:MM:SS,mmm</c> (also accepts <c>HH:MM:SS.mmm</c>).</summary>
    public static bool TryParseTimecode(ReadOnlySpan<char> s, out TimeSpan value)
    {
        value = default;
        if (s.Length < 8) return false;

        int colon1 = s.IndexOf(':');
        if (colon1 < 0) return false;
        int colon2 = s[(colon1 + 1)..].IndexOf(':');
        if (colon2 < 0) return false;
        colon2 += colon1 + 1;

        int hh = ParseInt(s[..colon1]);
        int mm = ParseInt(s.Slice(colon1 + 1, colon2 - colon1 - 1));
        var rest = s[(colon2 + 1)..];

        int sepIdx = rest.IndexOfAny(',', '.');
        int ss; int ms = 0;
        if (sepIdx >= 0)
        {
            ss = ParseInt(rest[..sepIdx]);
            ms = ParseInt(rest[(sepIdx + 1)..]);
        }
        else
        {
            ss = ParseInt(rest);
        }
        if (hh < 0 || mm < 0 || ss < 0 || ms < 0) return false;

        value = new TimeSpan(0, hh, mm, ss, ms);
        return true;
    }

    private static int ParseInt(ReadOnlySpan<char> s)
    {
        if (s.Length == 0) return -1;
        int value = 0;
        foreach (var ch in s)
        {
            if (ch < '0' || ch > '9') return -1;
            value = value * 10 + (ch - '0');
        }
        return value;
    }
}
