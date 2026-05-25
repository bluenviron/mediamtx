using System.Globalization;
using System.Text;

namespace Mediar.Subtitles.Srt;

/// <summary>
/// SubRip (<c>.srt</c>) writer. Always emits CRLF line endings and the canonical
/// <c>HH:MM:SS,mmm</c> timecode format.
/// </summary>
public static class SrtWriter
{
    /// <summary>Write <paramref name="cues"/> to <paramref name="path"/> as UTF-8.</summary>
    public static void WriteFile(string path, IEnumerable<SrtCue> cues)
    {
        ArgumentNullException.ThrowIfNull(cues);
        using var writer = new StreamWriter(path, append: false, Encoding.UTF8);
        Write(writer, cues);
    }

    /// <summary>Serialize <paramref name="cues"/> to a string.</summary>
    public static string WriteString(IEnumerable<SrtCue> cues)
    {
        ArgumentNullException.ThrowIfNull(cues);
        using var sw = new StringWriter(CultureInfo.InvariantCulture);
        Write(sw, cues);
        return sw.ToString();
    }

    /// <summary>Write cues to <paramref name="writer"/>.</summary>
    public static void Write(TextWriter writer, IEnumerable<SrtCue> cues)
    {
        ArgumentNullException.ThrowIfNull(writer);
        ArgumentNullException.ThrowIfNull(cues);

        bool first = true;
        int autoIndex = 0;
        foreach (var cue in cues)
        {
            if (!first) writer.Write("\r\n");
            first = false;
            int idx = cue.Index > 0 ? cue.Index : ++autoIndex;
            writer.Write(idx);
            writer.Write("\r\n");
            writer.Write(FormatTimecode(cue.Start));
            writer.Write(" --> ");
            writer.Write(FormatTimecode(cue.End));
            writer.Write("\r\n");
            string text = cue.Text.Replace("\r\n", "\n").Replace('\r', '\n');
            foreach (var line in text.Split('\n'))
            {
                writer.Write(line);
                writer.Write("\r\n");
            }
        }
    }

    /// <summary>Format <paramref name="value"/> as an SRT timecode (<c>HH:MM:SS,mmm</c>).</summary>
    public static string FormatTimecode(TimeSpan value)
    {
        if (value < TimeSpan.Zero) value = TimeSpan.Zero;
        long ms = (long)value.TotalMilliseconds;
        int hh = (int)(ms / 3_600_000);
        ms -= hh * 3_600_000L;
        int mm = (int)(ms / 60_000);
        ms -= mm * 60_000L;
        int ss = (int)(ms / 1000);
        int mmm = (int)(ms - ss * 1000);
        return string.Create(12, (hh, mm, ss, mmm), static (span, state) =>
        {
            Write2(span[..2], state.hh);
            span[2] = ':';
            Write2(span[3..5], state.mm);
            span[5] = ':';
            Write2(span[6..8], state.ss);
            span[8] = ',';
            Write3(span[9..12], state.mmm);
        });
    }

    private static void Write2(Span<char> dst, int value)
    {
        if (value < 0) value = 0;
        if (value > 99) value = 99;
        dst[0] = (char)('0' + value / 10);
        dst[1] = (char)('0' + value % 10);
    }

    private static void Write3(Span<char> dst, int value)
    {
        if (value < 0) value = 0;
        if (value > 999) value = 999;
        dst[0] = (char)('0' + value / 100);
        dst[1] = (char)('0' + (value / 10) % 10);
        dst[2] = (char)('0' + value % 10);
    }
}
