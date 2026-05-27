using System.Globalization;

namespace Mediar.Codecs.SvgRaster;

/// <summary>
/// SVG length parser. CSS reference DPI is 96 - all absolute units below
/// are derived from that. Percentage values resolve against the supplied
/// viewport length when one is provided; otherwise they remain 0.
/// </summary>
public static class SvgLength
{
    /// <summary>Resolve an SVG length string into user-space units.</summary>
    public static float Parse(string? text, float viewport = 0f, float defaultIfMissing = 0f)
    {
        if (string.IsNullOrWhiteSpace(text)) return defaultIfMissing;
        var s = text.AsSpan().Trim();
        float factor = 1f;
        bool percent = false;
        if (s.EndsWith("px", StringComparison.OrdinalIgnoreCase)) { factor = 1f; s = s[..^2]; }
        else if (s.EndsWith("pt", StringComparison.OrdinalIgnoreCase)) { factor = 96f / 72f; s = s[..^2]; }
        else if (s.EndsWith("pc", StringComparison.OrdinalIgnoreCase)) { factor = 16f; s = s[..^2]; }
        else if (s.EndsWith("mm", StringComparison.OrdinalIgnoreCase)) { factor = 96f / 25.4f; s = s[..^2]; }
        else if (s.EndsWith("cm", StringComparison.OrdinalIgnoreCase)) { factor = 96f / 2.54f; s = s[..^2]; }
        else if (s.EndsWith("in", StringComparison.OrdinalIgnoreCase)) { factor = 96f; s = s[..^2]; }
        else if (s.EndsWith("em", StringComparison.OrdinalIgnoreCase)) { factor = 16f; s = s[..^2]; }
        else if (s.EndsWith("ex", StringComparison.OrdinalIgnoreCase)) { factor = 8f; s = s[..^2]; }
        else if (s.EndsWith('%')) { percent = true; s = s[..^1]; }

        if (!float.TryParse(s.Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out float v))
            return defaultIfMissing;
        return percent ? v * viewport / 100f : v * factor;
    }
}
