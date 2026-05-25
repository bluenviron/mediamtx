using System.Globalization;

namespace Mediar.Subtitles.Ass;

/// <summary>
/// A single Advanced SubStation Alpha event ("Dialogue:" line).
/// Holds the raw field values exactly as they appear in the source
/// file so round-tripping is lossless.
/// </summary>
public sealed record AssEvent
{
    /// <summary>Event kind (typically <c>Dialogue</c>; also <c>Comment</c>).</summary>
    public string Kind { get; init; } = "Dialogue";

    /// <summary>Render layer ordering value.</summary>
    public int Layer { get; init; }

    /// <summary>Cue start time.</summary>
    public TimeSpan Start { get; init; }

    /// <summary>Cue end time.</summary>
    public TimeSpan End { get; init; }

    /// <summary>Style name (resolved from the [V4+ Styles] section).</summary>
    public string Style { get; init; } = "Default";

    /// <summary>Actor / speaker name.</summary>
    public string Name { get; init; } = string.Empty;

    /// <summary>Left margin override in pixels (0 = use style default).</summary>
    public int MarginL { get; init; }

    /// <summary>Right margin override in pixels.</summary>
    public int MarginR { get; init; }

    /// <summary>Vertical margin override in pixels.</summary>
    public int MarginV { get; init; }

    /// <summary>Effect string (e.g. <c>Karaoke</c>).</summary>
    public string Effect { get; init; } = string.Empty;

    /// <summary>Subtitle text, preserving inline override tags.</summary>
    public string Text { get; init; } = string.Empty;

    /// <inheritdoc/>
    public override string ToString() =>
        $"[{Kind} L{Layer}] {AssTime.Format(Start)} → {AssTime.Format(End)} ({Style}): {Text}";
}

internal static class AssTime
{
    public static bool TryParse(ReadOnlySpan<char> s, out TimeSpan value)
    {
        // ASS format: H:MM:SS.CS (hours unbounded, centiseconds)
        value = default;
        int c1 = s.IndexOf(':');
        if (c1 < 0) return false;
        int c2 = s[(c1 + 1)..].IndexOf(':') + c1 + 1;
        if (c2 <= c1) return false;
        int dot = s[(c2 + 1)..].IndexOf('.') + c2 + 1;
        if (dot <= c2) return false;
        if (!int.TryParse(s[..c1], NumberStyles.Integer, CultureInfo.InvariantCulture, out int h)) return false;
        if (!int.TryParse(s[(c1 + 1)..c2], NumberStyles.Integer, CultureInfo.InvariantCulture, out int m)) return false;
        if (!int.TryParse(s[(c2 + 1)..dot], NumberStyles.Integer, CultureInfo.InvariantCulture, out int sec)) return false;
        if (!int.TryParse(s[(dot + 1)..], NumberStyles.Integer, CultureInfo.InvariantCulture, out int cs)) return false;
        value = new TimeSpan(0, h, m, sec, cs * 10);
        return true;
    }

    public static string Format(TimeSpan t)
    {
        if (t < TimeSpan.Zero) t = TimeSpan.Zero;
        int h = (int)t.TotalHours;
        int m = t.Minutes;
        int s = t.Seconds;
        int cs = t.Milliseconds / 10;
        return $"{h}:{m:D2}:{s:D2}.{cs:D2}";
    }
}
