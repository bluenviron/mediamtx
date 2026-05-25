using System.Text;

namespace Mediar.Subtitles.WebVtt;

/// <summary>WebVTT writer.</summary>
public static class WebVttWriter
{
    /// <summary>Write WebVTT to <paramref name="path"/> as UTF-8.</summary>
    public static void WriteFile(string path, IEnumerable<WebVttCue> cues)
    {
        using var writer = new StreamWriter(path, append: false, Encoding.UTF8);
        Write(writer, cues);
    }

    /// <summary>Serialize cues to a string.</summary>
    public static string WriteString(IEnumerable<WebVttCue> cues)
    {
        using var sw = new StringWriter();
        Write(sw, cues);
        return sw.ToString();
    }

    /// <summary>Write cues to <paramref name="writer"/>.</summary>
    public static void Write(TextWriter writer, IEnumerable<WebVttCue> cues)
    {
        ArgumentNullException.ThrowIfNull(writer);
        ArgumentNullException.ThrowIfNull(cues);

        writer.Write("WEBVTT\n\n");
        foreach (var cue in cues)
        {
            if (!string.IsNullOrEmpty(cue.Identifier))
            {
                writer.Write(cue.Identifier);
                writer.Write('\n');
            }
            writer.Write(FormatTimecode(cue.Start));
            writer.Write(" --> ");
            writer.Write(FormatTimecode(cue.End));
            if (!string.IsNullOrEmpty(cue.Settings))
            {
                writer.Write(' ');
                writer.Write(cue.Settings);
            }
            writer.Write('\n');
            string text = cue.Text.Replace("\r\n", "\n").Replace('\r', '\n');
            foreach (var line in text.Split('\n'))
            {
                writer.Write(line);
                writer.Write('\n');
            }
            writer.Write('\n');
        }
    }

    /// <summary>Format a TimeSpan as a WebVTT timecode.</summary>
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
        return $"{hh:00}:{mm:00}:{ss:00}.{mmm:000}";
    }
}
