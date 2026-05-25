using System.Text;

namespace Mediar.Subtitles.WebVtt;

/// <summary>WebVTT (<c>.vtt</c>) reader.</summary>
public static class WebVttReader
{
    /// <summary>Open a WebVTT file and stream its cues.</summary>
    public static IEnumerable<WebVttCue> ReadFile(string path)
    {
        using var reader = new StreamReader(path, Encoding.UTF8, detectEncodingFromByteOrderMarks: true);
        foreach (var cue in Read(reader)) yield return cue;
    }

    /// <summary>Parse a WebVTT string.</summary>
    public static IEnumerable<WebVttCue> ReadString(string content)
    {
        using var reader = new StringReader(content);
        foreach (var cue in Read(reader)) yield return cue;
    }

    /// <summary>Stream cues from <paramref name="reader"/>.</summary>
    public static IEnumerable<WebVttCue> Read(TextReader reader)
    {
        ArgumentNullException.ThrowIfNull(reader);

        // Required header line "WEBVTT" possibly followed by extra text.
        string? header = reader.ReadLine();
        if (header is null) yield break;
        if (header.Length > 0 && header[0] == '\uFEFF') header = header[1..];
        if (!header.StartsWith("WEBVTT", StringComparison.Ordinal))
        {
            throw new InvalidDataException("Missing WEBVTT signature.");
        }

        // Skip the header block.
        while (true)
        {
            string? line = reader.ReadLine();
            if (line is null) yield break;
            if (line.Length == 0) break;
        }

        var text = new StringBuilder();
        while (true)
        {
            string? first = SkipBlankLines(reader);
            if (first is null) yield break;

            // NOTE / STYLE / REGION blocks: skip until empty line.
            if (first.StartsWith("NOTE", StringComparison.Ordinal) ||
                first.StartsWith("STYLE", StringComparison.Ordinal) ||
                first.StartsWith("REGION", StringComparison.Ordinal))
            {
                while (true)
                {
                    string? l = reader.ReadLine();
                    if (l is null || l.Length == 0) break;
                }
                continue;
            }

            string identifier = string.Empty;
            string timecodeLine;
            if (first.Contains("-->", StringComparison.Ordinal))
            {
                timecodeLine = first;
            }
            else
            {
                identifier = first;
                string? t = reader.ReadLine();
                if (t is null) yield break;
                timecodeLine = t;
            }

            if (!TryParseTimecodeLine(timecodeLine, out var start, out var end, out string settings))
            {
                continue;
            }

            text.Clear();
            while (true)
            {
                string? line = reader.ReadLine();
                if (line is null || line.Length == 0) break;
                if (text.Length > 0) text.Append('\n');
                text.Append(line);
            }

            yield return new WebVttCue(identifier, start, end, settings, text.ToString());
        }
    }

    private static string? SkipBlankLines(TextReader reader)
    {
        while (true)
        {
            string? l = reader.ReadLine();
            if (l is null) return null;
            if (l.Length == 0) continue;
            return l;
        }
    }

    private static bool TryParseTimecodeLine(string line, out TimeSpan start, out TimeSpan end, out string settings)
    {
        start = end = TimeSpan.Zero;
        settings = string.Empty;

        int sep = line.IndexOf("-->", StringComparison.Ordinal);
        if (sep < 0) return false;

        var left = line.AsSpan(0, sep).Trim();
        var right = line.AsSpan(sep + 3).TrimStart();
        int sp = right.IndexOf(' ');
        ReadOnlySpan<char> endSpan;
        if (sp >= 0)
        {
            endSpan = right[..sp];
            settings = right[(sp + 1)..].ToString();
        }
        else
        {
            endSpan = right;
        }

        return TryParseTimecode(left, out start) && TryParseTimecode(endSpan, out end);
    }

    /// <summary>Parse a WebVTT timecode (<c>[HH:]MM:SS.mmm</c>).</summary>
    public static bool TryParseTimecode(ReadOnlySpan<char> s, out TimeSpan value)
    {
        value = default;
        if (s.Length < 7) return false;

        // Count colons: 1 means MM:SS.mmm, 2 means HH:MM:SS.mmm.
        int firstColon = s.IndexOf(':');
        if (firstColon < 0) return false;
        var rest = s[(firstColon + 1)..];
        int secondColon = rest.IndexOf(':');

        int hh = 0, mm, ss, ms = 0;
        ReadOnlySpan<char> secPart;
        if (secondColon >= 0)
        {
            hh = ParseInt(s[..firstColon]);
            mm = ParseInt(rest[..secondColon]);
            secPart = rest[(secondColon + 1)..];
        }
        else
        {
            mm = ParseInt(s[..firstColon]);
            secPart = rest;
        }

        int dot = secPart.IndexOfAny('.', ',');
        if (dot < 0)
        {
            ss = ParseInt(secPart);
        }
        else
        {
            ss = ParseInt(secPart[..dot]);
            ms = ParseInt(secPart[(dot + 1)..]);
        }
        if (hh < 0 || mm < 0 || ss < 0 || ms < 0) return false;

        value = new TimeSpan(0, hh, mm, ss, ms);
        return true;
    }

    private static int ParseInt(ReadOnlySpan<char> s)
    {
        if (s.Length == 0) return -1;
        int v = 0;
        foreach (var ch in s)
        {
            if (ch < '0' || ch > '9') return -1;
            v = v * 10 + (ch - '0');
        }
        return v;
    }
}
