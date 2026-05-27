using System.Globalization;
using Mediar.Vector;

namespace Mediar.Codecs.SvgRaster;

/// <summary>
/// Parses an SVG / CSS color value into an <see cref="RgbaColor"/>.
/// Supports:
/// <list type="bullet">
///   <item><description>The 147 CSS Level 3 named colors (incl. <c>transparent</c>).</description></item>
///   <item><description><c>#RGB</c>, <c>#RGBA</c>, <c>#RRGGBB</c>, <c>#RRGGBBAA</c>.</description></item>
///   <item><description><c>rgb(r, g, b)</c> / <c>rgba(r, g, b, a)</c> with integer
///     or percentage channels and 0..1 alpha.</description></item>
///   <item><description><c>none</c>, <c>transparent</c>, <c>currentColor</c>.</description></item>
/// </list>
/// </summary>
public static class SvgColorParser
{
    /// <summary>
    /// Marker color used to express <c>currentColor</c>. Callers must
    /// substitute the cascaded <c>color</c> property when they encounter
    /// this sentinel.
    /// </summary>
    public static readonly RgbaColor CurrentColorSentinel = new(-1f, -1f, -1f, -1f);

    /// <summary>
    /// Try to parse a CSS color literal.
    /// Returns <c>true</c> on success, <c>false</c> on <c>none</c> /
    /// unrecognised input. Returns <see cref="CurrentColorSentinel"/> for
    /// <c>currentColor</c>.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<char> text, out RgbaColor color)
    {
        color = default;
        text = text.Trim();
        if (text.IsEmpty) return false;
        if (text.Equals("none", StringComparison.OrdinalIgnoreCase)) return false;
        if (text.Equals("transparent", StringComparison.OrdinalIgnoreCase))
        {
            color = RgbaColor.Transparent;
            return true;
        }
        if (text.Equals("currentColor", StringComparison.OrdinalIgnoreCase))
        {
            color = CurrentColorSentinel;
            return true;
        }
        if (text[0] == '#') return TryParseHex(text[1..], out color);
        if (text.StartsWith("rgb(", StringComparison.OrdinalIgnoreCase))
            return TryParseRgb(text[4..^1], out color, hasAlpha: false);
        if (text.StartsWith("rgba(", StringComparison.OrdinalIgnoreCase))
            return TryParseRgb(text[5..^1], out color, hasAlpha: true);
        return NamedColors.TryGet(text, out color);
    }

    private static bool TryParseHex(ReadOnlySpan<char> hex, out RgbaColor color)
    {
        color = default;
        if (hex.Length is 3 or 4)
        {
            // Short form: each hex digit duplicated.
            byte r = ExpandNibble(hex[0]);
            byte g = ExpandNibble(hex[1]);
            byte b = ExpandNibble(hex[2]);
            byte a = hex.Length == 4 ? ExpandNibble(hex[3]) : (byte)255;
            color = RgbaColor.FromBytes(r, g, b, a);
            return true;
        }
        if (hex.Length is 6 or 8)
        {
            if (!byte.TryParse(hex.Slice(0, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out byte r)) return false;
            if (!byte.TryParse(hex.Slice(2, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out byte g)) return false;
            if (!byte.TryParse(hex.Slice(4, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out byte b)) return false;
            byte a = 255;
            if (hex.Length == 8 && !byte.TryParse(hex.Slice(6, 2), NumberStyles.HexNumber, CultureInfo.InvariantCulture, out a)) return false;
            color = RgbaColor.FromBytes(r, g, b, a);
            return true;
        }
        return false;

        static byte ExpandNibble(char c)
        {
            int v = HexValue(c);
            return v < 0 ? (byte)0 : (byte)((v << 4) | v);
        }

        static int HexValue(char c) => c switch
        {
            >= '0' and <= '9' => c - '0',
            >= 'a' and <= 'f' => c - 'a' + 10,
            >= 'A' and <= 'F' => c - 'A' + 10,
            _ => -1,
        };
    }

    private static bool TryParseRgb(ReadOnlySpan<char> body, out RgbaColor color, bool hasAlpha)
    {
        color = default;
        // Split on commas (legacy) or whitespace + optional slash (modern CSS).
        var parts = new List<string>();
        int start = 0;
        for (int i = 0; i <= body.Length; i++)
        {
            if (i == body.Length || body[i] == ',' || body[i] == '/' || body[i] == ' ' || body[i] == '\t')
            {
                if (i > start) parts.Add(new string(body[start..i]).Trim());
                start = i + 1;
            }
        }
        parts.RemoveAll(p => p.Length == 0);
        if (parts.Count < 3) return false;
        if (!ParseChannel(parts[0], out byte r)) return false;
        if (!ParseChannel(parts[1], out byte g)) return false;
        if (!ParseChannel(parts[2], out byte b)) return false;
        byte a = 255;
        if (hasAlpha)
        {
            if (parts.Count < 4) return false;
            if (!ParseAlpha(parts[3], out a)) return false;
        }
        else if (parts.Count >= 4)
        {
            if (!ParseAlpha(parts[3], out a)) return false;
        }
        color = RgbaColor.FromBytes(r, g, b, a);
        return true;

        static bool ParseChannel(string s, out byte v)
        {
            s = s.Trim();
            v = 0;
            if (s.EndsWith('%'))
            {
                if (!float.TryParse(s.AsSpan(0, s.Length - 1), NumberStyles.Float, CultureInfo.InvariantCulture, out float pct)) return false;
                pct = Math.Clamp(pct, 0f, 100f);
                v = (byte)(pct * 255f / 100f + 0.5f);
                return true;
            }
            if (!float.TryParse(s, NumberStyles.Float, CultureInfo.InvariantCulture, out float f)) return false;
            v = (byte)Math.Clamp((int)MathF.Round(f), 0, 255);
            return true;
        }

        static bool ParseAlpha(string s, out byte v)
        {
            s = s.Trim();
            v = 0;
            if (s.EndsWith('%'))
            {
                if (!float.TryParse(s.AsSpan(0, s.Length - 1), NumberStyles.Float, CultureInfo.InvariantCulture, out float pct)) return false;
                v = (byte)(Math.Clamp(pct, 0f, 100f) * 255f / 100f + 0.5f);
                return true;
            }
            if (!float.TryParse(s, NumberStyles.Float, CultureInfo.InvariantCulture, out float f)) return false;
            v = (byte)(Math.Clamp(f, 0f, 1f) * 255f + 0.5f);
            return true;
        }
    }
}
